package dynamicproxy

import (
	"errors"
	"html/template"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/soheilhy/cmux"
)

var httpsLinkPage = template.Must(template.New("https-link").Parse(`<!doctype html>
<html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1">
<title>Continue over HTTPS</title>
<body><a href="{{.}}" rel="noreferrer">Continue over HTTPS</a></body></html>
`))

func (router *Router) serveTLSListener(server *http.Server, listener net.Listener) error {
	switch router.Option.PlainHTTPOnTLS {
	case "redirect", "link":
		return router.serveTLSWithPlainHTTP(server, listener)
	default:
		return server.ServeTLS(listener, "", "")
	}
}

// cmux retries temporary accept errors immediately. Back off as net/http does
// so resource exhaustion on the shared socket does not cause a busy loop.
type acceptBackoffListener struct {
	net.Listener
}

func (listener acceptBackoffListener) Accept() (net.Conn, error) {
	delay := 5 * time.Millisecond
	for {
		conn, err := listener.Listener.Accept()
		var temporary interface{ Temporary() bool }
		if !errors.As(err, &temporary) || !temporary.Temporary() {
			return conn, err
		}
		time.Sleep(delay)
		delay = min(2*delay, time.Second)
	}
}

func (router *Router) serveTLSWithPlainHTTP(server *http.Server, listener net.Listener) error {
	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		listener.Close()
		return err
	}
	mux := cmux.New(acceptBackoffListener{Listener: listener})
	mux.SetReadTimeout(5 * time.Second)
	tlsListener := mux.Match(cmux.TLS())
	httpListener := mux.Match(cmux.HTTP1())
	plainServer := &http.Server{
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			router.handlePlainHTTPOnTLS(w, r, port)
		}),
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		MaxHeaderBytes: 16 << 10,
	}
	plainServer.SetKeepAlivesEnabled(false)

	// The mux listeners share the root socket, so stopping the TLS server also
	// stops accepting plaintext connections on the same port.
	defer mux.Close()
	defer listener.Close()
	defer plainServer.Close()
	go mux.Serve()
	go plainServer.Serve(httpListener)
	return server.ServeTLS(tlsListener, "", "")
}

func (router *Router) handlePlainHTTPOnTLS(w http.ResponseWriter, r *http.Request, port string) {
	host := (&url.URL{Host: r.Host}).Hostname()
	if host == "" || !strings.HasPrefix(r.URL.Path, "/") {
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	if router.GetProxyEndpointFromHostname(host) == nil {
		http.NotFound(w, r)
		return
	}
	authority := net.JoinHostPort(host, port)
	if port == "443" {
		authority = strings.TrimSuffix(authority, ":443")
	}
	target := (&url.URL{
		Scheme:     "https",
		Host:       authority,
		Path:       r.URL.Path,
		RawPath:    r.URL.RawPath,
		RawQuery:   r.URL.RawQuery,
		ForceQuery: r.URL.ForceQuery,
	}).String()

	w.Header().Set("Cache-Control", "no-store")
	if router.Option.PlainHTTPOnTLS == "redirect" {
		http.Redirect(w, r, target, http.StatusTemporaryRedirect)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; base-uri 'none'; frame-ancestors 'none'")
	w.Header().Set("Referrer-Policy", "no-referrer")
	_ = httpsLinkPage.Execute(w, target)
}
