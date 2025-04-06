package dpcore

import (
	"context"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/dynamicproxy/domainsniff"
	"imuslab.com/zoraxy/mod/dynamicproxy/permissionpolicy"
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

	//Prepender is an optional prepend text for URL rewrite
	Prepender string

	Verbal bool

	//Appended by Zoraxy project

}

type ResponseRewriteRuleSet struct {
	/* Basic Rewrite Rulesets */
	ProxyDomain       string
	OriginalHost      string
	UseTLS            bool
	NoCache           bool
	PathPrefix        string //Vdir prefix for root, / will be rewrite to this
	UpstreamHeaders   [][]string
	DownstreamHeaders [][]string

	/* Advance Usecase Options */
	HostHeaderOverwrite string //Force overwrite of request "Host" header (advanced usecase)
	NoRemoveHopByHop    bool   //Do not remove hop-by-hop headers (advanced usecase)

	/* System Information Payload */
	Version string //Version number of Zoraxy, use for X-Proxy-By
}

type requestCanceler interface {
	CancelRequest(req *http.Request)
}

type DpcoreOptions struct {
	IgnoreTLSVerification   bool          //Disable all TLS verification when request pass through this proxy router
	FlushInterval           time.Duration //Duration to flush in normal requests. Stream request or keep-alive request will always flush with interval of -1 (immediately)
	MaxConcurrentConnection int           //Maxmium concurrent requests to this server
	ResponseHeaderTimeout   int64         //Timeout for response header, set to 0 for default
}

func NewDynamicProxyCore(target *url.URL, prepender string, dpcOptions *DpcoreOptions) *ReverseProxy {
	targetQuery := target.RawQuery
	director := func(req *http.Request) {
		req.URL.Scheme = target.Scheme
		req.URL.Host = target.Host
		req.URL.Path, req.URL.RawPath = joinURLPath(target, req.URL)
		if targetQuery == "" || req.URL.RawQuery == "" {
			req.URL.RawQuery = targetQuery + req.URL.RawQuery
		} else {
			req.URL.RawQuery = targetQuery + "&" + req.URL.RawQuery
		}

	}

	thisTransporter := http.DefaultTransport

	//Hack the default transporter to handle more connections
	optimalConcurrentConnection := 256
	if dpcOptions.MaxConcurrentConnection > 0 {
		optimalConcurrentConnection = dpcOptions.MaxConcurrentConnection
	}

	thisTransporter.(*http.Transport).IdleConnTimeout = 30 * time.Second
	thisTransporter.(*http.Transport).MaxIdleConns = optimalConcurrentConnection * 2
	thisTransporter.(*http.Transport).DisableCompression = true
	thisTransporter.(*http.Transport).DisableKeepAlives = false

	if dpcOptions.ResponseHeaderTimeout > 0 {
		//Set response header timeout
		thisTransporter.(*http.Transport).ResponseHeaderTimeout = time.Duration(dpcOptions.ResponseHeaderTimeout) * time.Millisecond
	}

	if dpcOptions.IgnoreTLSVerification {
		//Ignore TLS certificate validation error
		if thisTransporter.(*http.Transport).TLSClientConfig != nil {
			thisTransporter.(*http.Transport).TLSClientConfig.InsecureSkipVerify = true
		}
	}

	return &ReverseProxy{
		Director:      director,
		Prepender:     prepender,
		FlushInterval: dpcOptions.FlushInterval,
		Verbal:        false,
		Transport:     thisTransporter,
	}
}

func joinURLPath(a, b *url.URL) (path, rawpath string) {
	apath, bpath := a.EscapedPath(), b.EscapedPath()
	aslash, bslash := strings.HasSuffix(apath, "/"), strings.HasPrefix(bpath, "/")

	switch {
	case aslash && bslash:
		return a.Path + b.Path[1:], apath + bpath[1:]
	case !aslash && !bslash:
		return a.Path + "/" + b.Path, apath + "/" + bpath
	default:
		return a.Path + b.Path, apath + bpath
	}
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
	//"Upgrade", // handled by websocket proxy in higher layer abstraction
}

// Copy response from src to dst with given flush interval, reference from httputil.ReverseProxy
func (p *ReverseProxy) copyResponse(dst http.ResponseWriter, src io.Reader, flushInterval time.Duration) error {
	var w io.Writer = dst
	if flushInterval != 0 {
		mlw := &maxLatencyWriter{
			dst:     dst,
			flush:   http.NewResponseController(dst).Flush,
			latency: flushInterval,
		}

		defer mlw.stop()
		// set up initial timer so headers get flushed even if body writes are delayed
		mlw.flushPending = true
		mlw.t = time.AfterFunc(flushInterval, mlw.delayedFlush)
		w = mlw
	}

	var buf []byte
	_, err := p.copyBuffer(w, src, buf)
	return err

}

// Copy with given buffer size. Default to 64k
func (p *ReverseProxy) copyBuffer(dst io.Writer, src io.Reader, buf []byte) (int64, error) {
	if len(buf) == 0 {
		buf = make([]byte, 64*1024)
	}

	var written int64
	for {
		nr, rerr := src.Read(buf)
		if rerr != nil && rerr != io.EOF && rerr != context.Canceled {
			p.logf("dpcore read error during body copy: %v", rerr)
		}

		if nr > 0 {
			nw, werr := dst.Write(buf[:nr])
			if nw > 0 {
				written += int64(nw)
			}

			if werr != nil {
				return written, werr
			}

			if nr != nw {
				return written, io.ErrShortWrite
			}
		}

		if rerr != nil {
			if rerr == io.EOF {
				rerr = nil
			}
			return written, rerr
		}
	}
}

func (p *ReverseProxy) logf(format string, args ...interface{}) {
	if p.ErrorLog != nil {
		p.ErrorLog.Printf(format, args...)
	} else {
		log.Printf(format, args...)
	}
}

func (p *ReverseProxy) ProxyHTTP(rw http.ResponseWriter, req *http.Request, rrr *ResponseRewriteRuleSet) (int, error) {
	transport := p.Transport

	outreq := req.Clone(req.Context())

	ctx, cancel := context.WithCancel(req.Context())
	defer cancel()
	outreq = outreq.WithContext(ctx)

	if requestCanceler, ok := transport.(requestCanceler); ok {
		go func() {
			<-ctx.Done()
			requestCanceler.CancelRequest(outreq)
		}()
	}

	p.Director(outreq)
	outreq.Close = false

	//Only skip origin rewrite iff proxy target require TLS and it is external domain name like github.com
	if rrr.HostHeaderOverwrite != "" {
		//Use user defined overwrite header value, see issue #255
		outreq.Host = rrr.HostHeaderOverwrite
	} else if !(rrr.UseTLS && isExternalDomainName(rrr.ProxyDomain)) {
		// Always use the original host, see issue #164
		outreq.Host = rrr.OriginalHost
	}

	// We may modify the header (shallow copied above), so we only copy it.
	outreq.Header = make(http.Header)
	copyHeader(outreq.Header, req.Header)

	// Remove hop-by-hop headers.
	if !rrr.NoRemoveHopByHop {
		removeHeaders(outreq.Header, rrr.NoCache)
	}

	// Add X-Forwarded-For Header.
	addXForwardedForHeader(outreq)

	// Add user defined headers (to upstream)
	injectUserDefinedHeaders(outreq.Header, rrr.UpstreamHeaders)

	// Rewrite outbound UA, must be after user headers
	rewriteUserAgent(outreq.Header, "Zoraxy/"+rrr.Version)

	//Fix proxmox transfer encoding bug if detected Proxmox Cookie
	if domainsniff.IsProxmox(req) {
		outreq.TransferEncoding = []string{"identity"}
	}

	res, err := transport.RoundTrip(outreq)
	if err != nil {
		if p.Verbal {
			p.logf("http: proxy error: %v", err)
		}
		return http.StatusBadGateway, err
	}

	// Remove hop-by-hop headers listed in the "Connection" header of the response, Remove hop-by-hop headers.
	if !rrr.NoRemoveHopByHop {
		removeHeaders(res.Header, rrr.NoCache)
	}

	//Remove the User-Agent header if exists
	if _, ok := res.Header["User-Agent"]; ok {
		//Server to client request should not contains a User-Agent header
		res.Header.Del("User-Agent")
	}

	if p.ModifyResponse != nil {
		if err := p.ModifyResponse(res); err != nil {
			if p.Verbal {
				p.logf("http: proxy error: %v", err)
			}

			//rw.WriteHeader(http.StatusBadGateway)
			return http.StatusBadGateway, err
		}
	}

	//Add debug X-Proxy-By tracker
	res.Header.Set("x-proxy-by", "zoraxy/"+rrr.Version)

	//Custom Location header rewriter functions
	if res.Header.Get("Location") != "" {
		locationRewrite := res.Header.Get("Location")
		originLocation := res.Header.Get("Location")
		res.Header.Set("zr-origin-location", originLocation)

		if strings.HasPrefix(originLocation, "http://") || strings.HasPrefix(originLocation, "https://") {
			//Full path
			//Replace the forwarded target with expected Host
			lr, err := replaceLocationHost(locationRewrite, rrr, req.TLS != nil)
			if err == nil {
				locationRewrite = lr
			}
		} else if strings.HasPrefix(originLocation, "/") && rrr.PathPrefix != "" {
			//Back to the root of this proxy object
			locationRewrite = strings.TrimSuffix(rrr.PathPrefix, "/") + originLocation
		} else {
			//Relative path. Do not modifiy location header

		}

		//Custom redirection to this rproxy relative path
		res.Header.Set("Location", locationRewrite)
	}

	// Add user defined headers (to downstream)
	injectUserDefinedHeaders(res.Header, rrr.DownstreamHeaders)

	// Copy header from response to client.
	copyHeader(rw.Header(), res.Header)

	// inject permission policy headers
	permissionpolicy.InjectPermissionPolicyHeader(rw, nil)

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

	//Get flush interval in real time and start copying the request
	flushInterval := p.getFlushInterval(req, res)
	p.copyResponse(rw, res.Body, flushInterval)

	// close now, instead of defer, to populate res.Trailer
	res.Body.Close()
	copyHeader(rw.Header(), res.Trailer)

	return res.StatusCode, nil
}

func (p *ReverseProxy) ProxyHTTPS(rw http.ResponseWriter, req *http.Request) (int, error) {
	hij, ok := rw.(http.Hijacker)
	if !ok {
		p.logf("http server does not support hijacker")
		return http.StatusNotImplemented, errors.New("http server does not support hijacker")
	}

	clientConn, _, err := hij.Hijack()
	if err != nil {
		if p.Verbal {
			p.logf("http: proxy error: %v", err)
		}
		return http.StatusInternalServerError, err
	}

	proxyConn, err := net.Dial("tcp", req.URL.Host)
	if err != nil {
		if p.Verbal {
			p.logf("http: proxy error: %v", err)
		}

		return http.StatusInternalServerError, err
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
		return http.StatusGatewayTimeout, err
	}

	err = proxyConn.SetDeadline(deadline)
	if err != nil {
		if p.Verbal {
			p.logf("http: proxy error: %v", err)
		}

		return http.StatusGatewayTimeout, err
	}

	_, err = clientConn.Write([]byte("HTTP/1.0 200 OK\r\n\r\n"))
	if err != nil {
		if p.Verbal {
			p.logf("http: proxy error: %v", err)
		}

		return http.StatusInternalServerError, err
	}

	go func() {
		io.Copy(clientConn, proxyConn)
		clientConn.Close()
		proxyConn.Close()
	}()

	io.Copy(proxyConn, clientConn)
	proxyConn.Close()
	clientConn.Close()

	return http.StatusOK, nil
}

func (p *ReverseProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request, rrr *ResponseRewriteRuleSet) (int, error) {
	if req.Method == "CONNECT" {
		return p.ProxyHTTPS(rw, req)
	} else {
		return p.ProxyHTTP(rw, req, rrr)
	}
}
