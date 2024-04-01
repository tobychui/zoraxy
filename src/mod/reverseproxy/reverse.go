package reverseproxy

import (
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

var onExitFlushLoop func()

const (
	defaultTimeout = time.Minute * 5
)

// ReverseProxy is an HTTP Handler that takes an incoming request and
// sends it to another server, proxying the response back to the
// client, support http, also support https tunnel using http.hijacker
type ReverseProxy struct {
	// Set the timeout of the proxy server, default is 5 minutes
	Timeout time.Duration

	// Director must be a function which modifies
	// the request into a new request to be sent
	// using Transport. Its response is then copied
	// back to the original client unmodified.
	// Director must not access the provided Request
	// after returning.
	Director func(*http.Request)

	// The transport used to perform proxy requests.
	// default is http.DefaultTransport.
	Transport http.RoundTripper

	// FlushInterval specifies the flush interval
	// to flush to the client while copying the
	// response body. If zero, no periodic flushing is done.
	FlushInterval time.Duration

	// ErrorLog specifies an optional logger for errors
	// that occur when attempting to proxy the request.
	// If nil, logging goes to os.Stderr via the log package's
	// standard logger.
	ErrorLog *log.Logger

	// ModifyResponse is an optional function that
	// modifies the Response from the backend.
	// If it returns an error, the proxy returns a StatusBadGateway error.
	ModifyResponse func(*http.Response) error

	Verbal bool
}

type requestCanceler interface {
	CancelRequest(req *http.Request)
}

// NewReverseProxy returns a new ReverseProxy that routes
// URLs to the scheme, host, and base path provided in target. If the
// target's path is "/base" and the incoming request was for "/dir",
// the target request will be for /base/dir. if the target's query is a=10
// and the incoming request's query is b=100, the target's request's query
// will be a=10&b=100.
// NewReverseProxy does not rewrite the Host header.
// To rewrite Host headers, use ReverseProxy directly with a custom
// Director policy.
func NewReverseProxy(target *url.URL) *ReverseProxy {
	targetQuery := target.RawQuery
	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path = singleJoiningSlash(target.Path, req.URL.Path)

		// If Host is empty, the Request.Write method uses
		// the value of URL.Host.
		// force use URL.Host
		req.Host = req.URL.Host
		if targetQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = targetQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
		}
	}

	return &ReverseProxy{Director: director, Verbal: false}
}

func singleJoiningSlash(a, b string) string {
	aslash := strings.HasSuffix(a, "/")
	bslash := strings.HasPrefix(b, "/")
	switch {
	case aslash && bslash:
		return a + b[1:]
	case !aslash && !bslash:
		return a + "/" + b
	}
	return a + b
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

// Hop-by-hop headers. These are removed when sent to the backend.
// http://www.w3.org/Protocols/rfc2616/rfc2616-sec13.html
var hopHeaders = []string{
	//"Connection",
	"Proxy-Connection", // non-standard but still sent by libcurl and rejected by e.g. google
	"Keep-Alive",
	"Proxy-Authenticate",
	"Proxy-Authorization",
	"Te",      // canonicalized version of "TE"
	"Trailer", // not Trailers per URL above; http://www.rfc-editor.org/errata_search.php?eid=4522
	"Transfer-Encoding",
	//"Upgrade",
}

func (p *ReverseProxy) copyResponse(dst io.Writer, src io.Reader) {
	if p.FlushInterval != 0 {
		if wf, ok := dst.(writeFlusher); ok {
			mlw := &maxLatencyWriter{
				dst:     wf,
				latency: p.FlushInterval,
				done:    make(chan bool),
			}

			go mlw.flushLoop()
			defer mlw.stop()
			dst = mlw
		}
	}

	io.Copy(dst, src)
}

type writeFlusher interface {
	io.Writer
	http.Flusher
}

type maxLatencyWriter struct {
	dst     writeFlusher
	latency time.Duration
	mu      sync.Mutex
	done    chan bool
}

func (m *maxLatencyWriter) Write(b []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dst.Write(b)
}

func (m *maxLatencyWriter) flushLoop() {
	t := time.NewTicker(m.latency)
	defer t.Stop()
	for {
		select {
		case <-m.done:
			if onExitFlushLoop != nil {
				onExitFlushLoop()
			}
			return
		case <-t.C:
			m.mu.Lock()
			m.dst.Flush()
			m.mu.Unlock()
		}
	}
}

func (m *maxLatencyWriter) stop() {
	m.done <- true
}

func (p *ReverseProxy) logf(format string, args ...interface{}) {
	if p.ErrorLog != nil {
		p.ErrorLog.Printf(format, args...)
	} else {
		log.Printf(format, args...)
	}
}

func removeHeaders(header http.Header) {
	// Remove hop-by-hop headers listed in the "Connection" header.
	if c := header.Get("Connection"); c != "" {
		for _, f := range strings.Split(c, ",") {
			if f = strings.TrimSpace(f); f != "" {
				header.Del(f)
			}
		}
	}

	// Remove hop-by-hop headers
	for _, h := range hopHeaders {
		if header.Get(h) != "" {
			header.Del(h)
		}
	}

	if header.Get("Zr-Origin-Upgrade") != "" {
		header.Set("Upgrade", header.Get("Zr-Origin-Upgrade"))
		header.Del("Zr-Origin-Upgrade")
	}
}

func addXForwardedForHeader(req *http.Request) {
	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		// If we aren't the first proxy retain prior
		// X-Forwarded-For information as a comma+space
		// separated list and fold multiple headers into one.
		if prior, ok := req.Header["X-Forwarded-For"]; ok {
			clientIP = strings.Join(prior, ", ") + ", " + clientIP
		}
		req.Header.Set("X-Forwarded-For", clientIP)
	}
}

func (p *ReverseProxy) ProxyHTTP(rw http.ResponseWriter, req *http.Request) error {
	transport := p.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}

	outreq := new(http.Request)
	// Shallow copies of maps, like header
	*outreq = *req

	if cn, ok := rw.(http.CloseNotifier); ok {
		if requestCanceler, ok := transport.(requestCanceler); ok {
			// After the Handler has returned, there is no guarantee
			// that the channel receives a value, so to make sure
			reqDone := make(chan struct{})
			defer close(reqDone)
			clientGone := cn.CloseNotify()

			go func() {
				select {
				case <-clientGone:
					requestCanceler.CancelRequest(outreq)
				case <-reqDone:
				}
			}()
		}
	}

	p.Director(outreq)
	outreq.Close = false

	// We may modify the header (shallow copied above), so we only copy it.
	outreq.Header = make(http.Header)
	copyHeader(outreq.Header, req.Header)

	// Remove hop-by-hop headers listed in the "Connection" header, Remove hop-by-hop headers.
	removeHeaders(outreq.Header)

	// Add X-Forwarded-For Header.
	addXForwardedForHeader(outreq)

	res, err := transport.RoundTrip(outreq)
	if err != nil {
		if p.Verbal {
			p.logf("http: proxy error: %v", err)
		}

		//rw.WriteHeader(http.StatusBadGateway)
		return err
	}

	// Remove hop-by-hop headers listed in the "Connection" header of the response, Remove hop-by-hop headers.
	removeHeaders(res.Header)

	if p.ModifyResponse != nil {
		if err := p.ModifyResponse(res); err != nil {
			if p.Verbal {
				p.logf("http: proxy error: %v", err)
			}
			//rw.WriteHeader(http.StatusBadGateway)
			return err
		}
	}

	// Copy header from response to client.
	copyHeader(rw.Header(), res.Header)

	// The "Trailer" header isn't included in the Transport's response, Build it up from Trailer.
	if len(res.Trailer) > 0 {
		trailerKeys := make([]string, 0, len(res.Trailer))
		for k := range res.Trailer {
			trailerKeys = append(trailerKeys, k)
		}
		rw.Header().Add("Trailer", strings.Join(trailerKeys, ", "))
	}

	rw.WriteHeader(res.StatusCode)
	if len(res.Trailer) > 0 {
		// Force chunking if we saw a response trailer.
		// This prevents net/http from calculating the length for short
		// bodies and adding a Content-Length.
		if fl, ok := rw.(http.Flusher); ok {
			fl.Flush()
		}
	}

	p.copyResponse(rw, res.Body)
	// close now, instead of defer, to populate res.Trailer
	res.Body.Close()
	copyHeader(rw.Header(), res.Trailer)

	return nil
}

func (p *ReverseProxy) ProxyHTTPS(rw http.ResponseWriter, req *http.Request) error {
	hij, ok := rw.(http.Hijacker)
	if !ok {
		p.logf("http server does not support hijacker")
		return errors.New("http server does not support hijacker")
	}

	clientConn, _, err := hij.Hijack()
	if err != nil {
		if p.Verbal {
			p.logf("http: proxy error: %v", err)
		}
		return err
	}

	proxyConn, err := net.Dial("tcp", req.URL.Host)
	if err != nil {
		if p.Verbal {
			p.logf("http: proxy error: %v", err)
		}
		return err
	}

	// The returned net.Conn may have read or write deadlines
	// already set, depending on the configuration of the
	// Server, to set or clear those deadlines as needed
	// we set timeout to 5 minutes
	deadline := time.Now()
	if p.Timeout == 0 {
		deadline = deadline.Add(time.Minute * 5)
	} else {
		deadline = deadline.Add(p.Timeout)
	}

	err = clientConn.SetDeadline(deadline)
	if err != nil {
		if p.Verbal {
			p.logf("http: proxy error: %v", err)
		}
		return err
	}

	err = proxyConn.SetDeadline(deadline)
	if err != nil {
		if p.Verbal {
			p.logf("http: proxy error: %v", err)
		}
		return err
	}

	_, err = clientConn.Write([]byte("HTTP/1.0 200 OK\r\n\r\n"))
	if err != nil {
		if p.Verbal {
			p.logf("http: proxy error: %v", err)
		}
		return err
	}

	go func() {
		io.Copy(clientConn, proxyConn)
		clientConn.Close()
		proxyConn.Close()
	}()

	io.Copy(proxyConn, clientConn)
	proxyConn.Close()
	clientConn.Close()

	return nil
}

func (p *ReverseProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) error {
	if req.Method == "CONNECT" {
		err := p.ProxyHTTPS(rw, req)
		return err
	} else {
		err := p.ProxyHTTP(rw, req)
		return err
	}
}
