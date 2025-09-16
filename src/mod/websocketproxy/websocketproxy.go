// Package websocketproxy is a reverse proxy for WebSocket connections.
package websocketproxy

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"

	"github.com/gorilla/websocket"
	"imuslab.com/zoraxy/mod/dynamicproxy/rewrite"
	"imuslab.com/zoraxy/mod/info/logger"
)

var (
	// DefaultUpgrader specifies the parameters for upgrading an HTTP
	// connection to a WebSocket connection.
	DefaultUpgrader = &websocket.Upgrader{
		ReadBufferSize:  1024,
		WriteBufferSize: 1024,
	}

	// DefaultDialer is a dialer with all fields set to the default zero values.
	DefaultDialer = websocket.DefaultDialer
)

// WebsocketProxy is an HTTP Handler that takes an incoming WebSocket
// connection and proxies it to another server.
type WebsocketProxy struct {
	// Director, if non-nil, is a function that may copy additional request
	// headers from the incoming WebSocket connection into the output headers
	// which will be forwarded to another server.
	Director func(incoming *http.Request, out http.Header)

	// Backend returns the backend URL which the proxy uses to reverse proxy
	// the incoming WebSocket connection. Request is the initial incoming and
	// unmodified request.
	Backend func(*http.Request) *url.URL

	// Upgrader specifies the parameters for upgrading a incoming HTTP
	// connection to a WebSocket connection. If nil, DefaultUpgrader is used.
	Upgrader *websocket.Upgrader

	//  Dialer contains options for connecting to the backend WebSocket server.
	//  If nil, DefaultDialer is used.
	Dialer *websocket.Dialer

	Verbal bool

	Options Options
}

// Additional options for websocket proxy runtime
type Options struct {
	SkipTLSValidation  bool                         //Skip backend TLS validation
	SkipOriginCheck    bool                         //Skip origin check
	CopyAllHeaders     bool                         //Copy all headers from incoming request to backend request
	UserDefinedHeaders []*rewrite.UserDefinedHeader //User defined headers
	Logger             *logger.Logger               //Logger, can be nil
}

// ProxyHandler returns a new http.Handler interface that reverse proxies the
// request to the given target.
func ProxyHandler(target *url.URL, options Options) http.Handler {
	return NewProxy(target, options)
}

// NewProxy returns a new Websocket reverse proxy that rewrites the
// URL's to the scheme, host and base path provider in target.
func NewProxy(target *url.URL, options Options) *WebsocketProxy {
	backend := func(r *http.Request) *url.URL {
		// Shallow copy
		u := *target
		u.Fragment = r.URL.Fragment
		u.Path = r.URL.Path
		u.RawQuery = r.URL.RawQuery
		return &u
	}

	// Create a new websocket proxy
	wsprox := &WebsocketProxy{Backend: backend, Verbal: false, Options: options}
	if options.CopyAllHeaders {
		wsprox.Director = DefaultDirector
	}

	return wsprox
}

// Utilities function for log printing
func (w *WebsocketProxy) Println(messsage string, err error) {
	if w.Options.Logger != nil {
		w.Options.Logger.PrintAndLog("websocket", messsage, err)
		return
	}
	log.Println("[websocketproxy] [system:info]"+messsage, err)
}

// DefaultDirector is the default implementation of Director, which copies
// all headers from the incoming request to the outgoing request.
func DefaultDirector(r *http.Request, h http.Header) {
	//Copy all header values from request to target header
	for k, vv := range r.Header {
		for _, v := range vv {
			h.Set(k, v)
		}
	}

	// Remove hop-by-hop headers
	for _, removePendingHeader := range []string{
		"Connection",
		"Keep-Alive",
		"Proxy-Authenticate",
		"Proxy-Authorization",
		"Te",
		"Trailers",
		"Transfer-Encoding",
		"Sec-WebSocket-Extensions",
		"Sec-WebSocket-Key",
		"Sec-WebSocket-Protocol",
		"Sec-WebSocket-Version",
		"Upgrade",
	} {
		h.Del(removePendingHeader)
	}
}

// ServeHTTP implements the http.Handler that proxies WebSocket connections.
func (w *WebsocketProxy) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	if w.Backend == nil {
		w.Println("Invalid websocket backend configuration", errors.New("backend function not found"))
		http.Error(rw, "internal server error (code: 1)", http.StatusInternalServerError)
		return
	}

	backendURL := w.Backend(req)
	if backendURL == nil {
		w.Println("Invalid websocket backend configuration", errors.New("backend URL is nil"))
		http.Error(rw, "internal server error (code: 2)", http.StatusInternalServerError)
		return
	}

	dialer := w.Dialer
	if w.Dialer == nil {
		if w.Options.SkipTLSValidation {
			//Disable TLS secure check if target allow skip verification
			bypassDialer := websocket.DefaultDialer
			bypassDialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
			dialer = bypassDialer
		} else {
			//Just use the default dialer come with gorilla websocket
			dialer = DefaultDialer
		}
	}

	// Pass headers from the incoming request to the dialer to forward them to
	// the final destinations.
	requestHeader := http.Header{}
	if origin := req.Header.Get("Origin"); origin != "" {
		requestHeader.Add("Origin", origin)
	}
	for _, prot := range req.Header[http.CanonicalHeaderKey("Sec-WebSocket-Protocol")] {
		requestHeader.Add("Sec-WebSocket-Protocol", prot)
	}
	for _, cookie := range req.Header[http.CanonicalHeaderKey("Cookie")] {
		requestHeader.Add("Cookie", cookie)
	}
	if req.Host != "" {
		requestHeader.Set("Host", req.Host)
	}
	if userAgent := req.Header.Get("User-Agent"); userAgent != "" {
		requestHeader.Set("User-Agent", userAgent)
	} else {
		requestHeader.Set("User-Agent", "zoraxy-wsproxy/1.1")
	}

	// Pass X-Forwarded-For headers too, code below is a part of
	// httputil.ReverseProxy. See http://en.wikipedia.org/wiki/X-Forwarded-For
	// for more information
	// TODO: use RFC7239 http://tools.ietf.org/html/rfc7239
	if clientIP, _, err := net.SplitHostPort(req.RemoteAddr); err == nil {
		// If we aren't the first proxy retain prior
		// X-Forwarded-For information as a comma+space
		// separated list and fold multiple headers into one.
		if prior, ok := req.Header["X-Forwarded-For"]; ok {
			clientIP = strings.Join(prior, ", ") + ", " + clientIP
		}
		requestHeader.Set("X-Forwarded-For", clientIP)
	}

	// Set the originating protocol of the incoming HTTP request. The SSL might
	// be terminated on our site and because we doing proxy adding this would
	// be helpful for applications on the backend.
	requestHeader.Set("X-Forwarded-Proto", "http")
	if req.TLS != nil {
		requestHeader.Set("X-Forwarded-Proto", "https")
	}

	// Replace header variables and copy user-defined headers
	if w.Options.CopyAllHeaders {
		// Rewrite the user defined headers
		// This is reported to be not compatible with Proxmox and Home Assistant
		// but required by some other projects like MeshCentral
		// we will make this optional
		rewrittenUserDefinedHeaders := rewrite.PopulateRequestHeaderVariables(req, w.Options.UserDefinedHeaders)
		upstreamHeaders, _ := rewrite.SplitUpDownStreamHeaders(&rewrite.HeaderRewriteOptions{
			UserDefinedHeaders: rewrittenUserDefinedHeaders,
		})
		for _, headerValuePair := range upstreamHeaders {
			//Skip empty header pairs
			if len(headerValuePair) < 2 {
				continue
			}
			//Do not copy Upgrade and Connection headers, it will be handled by the upgrader
			if strings.EqualFold(headerValuePair[0], "Upgrade") || strings.EqualFold(headerValuePair[0], "Connection") {
				continue
			}
			requestHeader.Set(headerValuePair[0], headerValuePair[1])
		}

		// Enable the director to copy any additional headers it desires for
		// forwarding to the remote server.
		if w.Director != nil {
			w.Director(req, requestHeader)
		}
	}

	// Connect to the backend URL, also pass the headers we get from the requst
	// together with the Forwarded headers we prepared above.
	// TODO: support multiplexing on the same backend connection instead of
	// opening a new TCP connection time for each request. This should be
	// optional:
	// http://tools.ietf.org/html/draft-ietf-hybi-websocket-multiplexing-01
	connBackend, resp, err := dialer.Dial(backendURL.String(), requestHeader)
	if err != nil {
		w.Println("Couldn't dial to remote backend url "+backendURL.String(), err)
		if resp != nil {
			// If the WebSocket handshake fails, ErrBadHandshake is returned
			// along with a non-nil *http.Response so that callers can handle
			// redirects, authentication, etcetera.
			if err := copyResponse(rw, resp); err != nil {
				w.Println("Couldn't write response after failed remote backend handshake to "+backendURL.String(), err)
			}
		} else {
			http.Error(rw, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
		}
		return
	}
	defer connBackend.Close()

	upgrader := w.Upgrader
	if w.Upgrader == nil {
		upgrader = DefaultUpgrader
	}

	//Fixing issue #107 by bypassing request origin check
	if w.Options.SkipOriginCheck {
		upgrader.CheckOrigin = func(r *http.Request) bool {
			return true
		}
	}

	// Only pass those headers to the upgrader.
	upgradeHeader := http.Header{}
	if hdr := resp.Header.Get("Sec-Websocket-Protocol"); hdr != "" {
		upgradeHeader.Set("Sec-Websocket-Protocol", hdr)
	}
	if hdr := resp.Header.Get("Set-Cookie"); hdr != "" {
		upgradeHeader.Set("Set-Cookie", hdr)
	}

	// Now upgrade the existing incoming request to a WebSocket connection.
	// Also pass the header that we gathered from the Dial handshake.
	connPub, err := upgrader.Upgrade(rw, req, upgradeHeader)
	if err != nil {
		w.Println("Couldn't upgrade incoming request", err)
		return
	}
	defer connPub.Close()

	errClient := make(chan error, 1)
	errBackend := make(chan error, 1)
	replicateWebsocketConn := func(dst, src *websocket.Conn, errc chan error) {
		for {
			msgType, msg, err := src.ReadMessage()
			if err != nil {
				m := websocket.FormatCloseMessage(websocket.CloseNormalClosure, fmt.Sprintf("%v", err))
				if e, ok := err.(*websocket.CloseError); ok {
					if e.Code != websocket.CloseNoStatusReceived {
						m = websocket.FormatCloseMessage(e.Code, e.Text)
					}
				}
				errc <- err
				dst.WriteMessage(websocket.CloseMessage, m)
				break
			}
			err = dst.WriteMessage(msgType, msg)
			if err != nil {
				errc <- err
				break
			}
		}
	}

	go replicateWebsocketConn(connPub, connBackend, errClient)
	go replicateWebsocketConn(connBackend, connPub, errBackend)

	var message string
	select {
	case err = <-errClient:
		message = "websocketproxy: Error when copying from backend to client: %v"
	case err = <-errBackend:
		message = "websocketproxy: Error when copying from client to backend: %v"

	}
	if e, ok := err.(*websocket.CloseError); !ok || e.Code == websocket.CloseAbnormalClosure {
		if w.Verbal {
			//Only print message on verbal mode
			log.Printf(message, err)
		}

	}
}

func copyHeader(dst, src http.Header) {
	for k, vv := range src {
		for _, v := range vv {
			dst.Add(k, v)
		}
	}
}

func copyResponse(rw http.ResponseWriter, resp *http.Response) error {
	copyHeader(rw.Header(), resp.Header)
	rw.WriteHeader(resp.StatusCode)
	defer resp.Body.Close()

	_, err := io.Copy(rw, resp.Body)
	return err
}
