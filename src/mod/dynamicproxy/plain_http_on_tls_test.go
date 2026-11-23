package dynamicproxy

import (
	"errors"
	"html"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestPlainHTTPOnTLS(t *testing.T) {
	const domain = "app.example.test"
	const path = "/a%2Fb?first=x%20y&second=z"
	for _, mode := range []string{"", "off", "redirect", "link"} {
		name := mode
		if name == "" {
			name = "default"
		}
		t.Run(name, func(t *testing.T) {
			router, err := NewDynamicProxy(RouterOption{PlainHTTPOnTLS: mode})
			if err != nil {
				t.Fatal(err)
			}
			router.ProxyEndpoints.Store(domain, &ProxyEndpoint{RootOrMatchingDomain: domain})
			tlsFixture := httptest.NewUnstartedServer(http.NotFoundHandler())
			tlsFixture.EnableHTTP2 = true
			tlsFixture.StartTLS()
			defer tlsFixture.Close()

			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close()
			_, port, err := net.SplitHostPort(listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			server := &http.Server{
				TLSConfig: tlsFixture.TLS.Clone(),
				Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					io.WriteString(w, "TLS response")
				}),
			}
			defer server.Close()
			stopped := make(chan error, 1)
			go func() { stopped <- router.serveTLSListener(server, listener) }()

			client := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			}}
			req, err := http.NewRequest(http.MethodGet, "http://"+listener.Addr().String()+path, nil)
			if err != nil {
				t.Fatal(err)
			}
			req.Host = net.JoinHostPort(domain, port)
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(res.Body)
			res.Body.Close()
			if err != nil {
				t.Fatal(err)
			}
			location := res.Header.Get("Location")
			want := "https://" + req.Host + path
			switch mode {
			case "", "off":
				if res.StatusCode != http.StatusBadRequest || location != "" {
					t.Fatalf("Strict TLS returned %d, Location %q", res.StatusCode, location)
				}
			case "redirect":
				if res.StatusCode != http.StatusTemporaryRedirect || location != want {
					t.Fatalf("Redirect returned %d, Location %q", res.StatusCode, location)
				}
			case "link":
				if res.StatusCode != http.StatusOK || location != "" ||
					!strings.Contains(string(body), `href="`+html.EscapeString(want)+`"`) {
					t.Fatalf("Link mode returned %d, Location %q, body %q", res.StatusCode, location, body)
				}
			}

			tlsClient := tlsFixture.Client()
			tlsClient.Timeout = 3 * time.Second
			res, err = tlsClient.Get("https://" + listener.Addr().String())
			if err != nil {
				t.Fatal(err)
			}
			body, err = io.ReadAll(res.Body)
			res.Body.Close()
			if err != nil || res.StatusCode != http.StatusOK || res.ProtoMajor != 2 || string(body) != "TLS response" {
				t.Fatalf("Unexpected HTTPS response: %s %d, body %q, error %v", res.Proto, res.StatusCode, body, err)
			}

			server.Close()
			select {
			case err := <-stopped:
				if !errors.Is(err, http.ErrServerClosed) {
					t.Fatal(err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("Listener did not stop")
			}
		})
	}
}

func TestPlainHTTPOnTLSRejectsUnknownHost(t *testing.T) {
	router, err := NewDynamicProxy(RouterOption{PlainHTTPOnTLS: "redirect"})
	if err != nil {
		t.Fatal(err)
	}
	res := httptest.NewRecorder()
	router.handlePlainHTTPOnTLS(res, httptest.NewRequest(http.MethodGet, "http://unknown.example.test/", nil), "8443")
	if res.Code != http.StatusNotFound || res.Header().Get("Location") != "" {
		t.Fatalf("Unknown host returned %d, Location %q", res.Code, res.Header().Get("Location"))
	}
}

type temporaryErrorListener struct {
	net.Listener
	attempts int
}

func (listener *temporaryErrorListener) Accept() (net.Conn, error) {
	listener.attempts++
	if listener.attempts < 4 {
		return nil, &net.DNSError{IsTemporary: true}
	}
	return nil, net.ErrClosed
}

func TestAcceptBackoffListener(t *testing.T) {
	listener := &temporaryErrorListener{}
	start := time.Now()
	_, err := (acceptBackoffListener{Listener: listener}).Accept()
	if !errors.Is(err, net.ErrClosed) || listener.attempts != 4 {
		t.Fatalf("Accept returned %v after %d attempts", err, listener.attempts)
	}
	if elapsed := time.Since(start); elapsed < 35*time.Millisecond {
		t.Fatalf("Accept retried without the expected backoff, elapsed %s", elapsed)
	}
}
