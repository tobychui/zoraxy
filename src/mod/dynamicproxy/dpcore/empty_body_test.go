package dpcore

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"testing"
	"testing/iotest"
	"time"
)

// newHTTP2ProxyTo is newHTTP2Proxy for any upstream URL, TLS included
func newHTTP2ProxyTo(t *testing.T, target *url.URL, opts *DpcoreOptions) *httptest.Server {
	t.Helper()
	core := NewDynamicProxyCore(target, "", opts)

	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		statusCode, err := core.ServeHTTP(w, r, &ResponseRewriteRuleSet{
			ProxyDomain:  target.Host,
			OriginalHost: r.Host,
			UseTLS:       target.Scheme == "https",
		})
		if err != nil {
			w.WriteHeader(statusCode)
		}
	}))
	srv.EnableHTTP2 = true
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv
}

// A client may hold its request body back until it has the response headers.
// gRPC bidirectional streams do (NetBird's Signal ConnectStream is one). Such a
// request has to reach the upstream before its first body byte exists.
func TestStreamWrittenAfterResponseHeadersIsForwarded(t *testing.T) {
	for _, contentType := range []string{"application/grpc", "application/octet-stream", ""} {
		t.Run("Content-Type="+contentType, func(t *testing.T) {
			t.Parallel()

			// Answers the headers at once, then echoes four bytes of the stream
			upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.ProtoMajor != 2 {
					t.Errorf("expected HTTP/2 between proxy and upstream, got %s", r.Proto)
				}
				w.WriteHeader(http.StatusOK)
				w.(http.Flusher).Flush()
				buf := make([]byte, 4)
				n, _ := io.ReadFull(r.Body, buf)
				w.Write(buf[:n])
			}))
			upstream.EnableHTTP2 = true
			upstream.StartTLS()
			defer upstream.Close()

			target, err := url.Parse(upstream.URL)
			if err != nil {
				t.Fatal(err)
			}
			proxy := newHTTP2ProxyTo(t, target, &DpcoreOptions{IgnoreTLSVerification: true})

			bodyReader, bodyWriter := io.Pipe()
			req, err := http.NewRequest(http.MethodPost, proxy.URL+"/svc.Stream/Connect", bodyReader)
			if err != nil {
				t.Fatal(err)
			}
			if contentType != "" {
				req.Header.Set("Content-Type", contentType)
			}

			client := proxy.Client()
			client.Timeout = 5 * time.Second

			// Do returns with the response headers, nothing has been written yet
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("no response headers for a stream that has not written yet: %v", err)
			}
			defer resp.Body.Close()
			if resp.ProtoMajor != 2 {
				t.Fatalf("expected HTTP/2 between client and proxy, got %s", resp.Proto)
			}

			bodyWriter.Write([]byte("ping"))
			bodyWriter.Close()
			echo, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatal(err)
			}
			if string(echo) != "ping" {
				t.Fatalf("echo = %q, want %q", echo, "ping")
			}
		})
	}
}

// The upstream may finish its response while the client still holds its request
// stream open, as a gRPC server does when it ends a stream on its own. The
// transport then closes the request body to stop its read of it, and the client
// has to see the end of the response.
func TestResponseCompletesWhileClientKeepsRequestStreamOpen(t *testing.T) {
	for _, sent := range []string{"", "ping"} {
		t.Run("client sent "+strconv.Quote(sent), func(t *testing.T) {
			t.Parallel()

			// Takes what the client sent so far, answers in full and returns
			upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				io.ReadFull(r.Body, make([]byte, len(sent)))
				w.Write([]byte("bye"))
			}))
			upstream.EnableHTTP2 = true
			upstream.StartTLS()
			defer upstream.Close()

			target, err := url.Parse(upstream.URL)
			if err != nil {
				t.Fatal(err)
			}
			proxy := newHTTP2ProxyTo(t, target, &DpcoreOptions{IgnoreTLSVerification: true})

			bodyReader, bodyWriter := io.Pipe()
			defer bodyWriter.Close()
			if sent != "" {
				go bodyWriter.Write([]byte(sent))
			}
			req, err := http.NewRequest(http.MethodPost, proxy.URL+"/svc.Stream/Connect", bodyReader)
			if err != nil {
				t.Fatal(err)
			}

			client := proxy.Client()
			client.Timeout = 5 * time.Second
			resp, err := client.Do(req)
			if err != nil {
				t.Fatalf("no response while the request stream is still open: %v", err)
			}
			defer resp.Body.Close()
			if resp.ProtoMajor != 2 {
				t.Fatalf("expected HTTP/2 between client and proxy, got %s", resp.Proto)
			}

			body, err := io.ReadAll(resp.Body)
			if err != nil {
				t.Fatalf("the response did not complete: %v", err)
			}
			if string(body) != "bye" {
				t.Fatalf("body = %q, want %q", body, "bye")
			}
		})
	}
}

// emptyBody is at EOF from the start but is not http.NoBody, so the HTTP/2
// client sends HEADERS without END_STREAM followed by an empty DATA frame: the
// proxy sees a body of unknown length.
type emptyBody struct{}

func (emptyBody) Read([]byte) (int, error) { return 0, io.EOF }
func (emptyBody) Close() error             { return nil }

// An HTTP/2 request without a body must reach an HTTP/1.1 upstream with
// Content-Length: 0, not as an empty chunked body (#1204).
func TestHTTP2BodylessPostIsSentWithContentLengthZero(t *testing.T) {
	cases := []struct {
		name        string
		body        io.ReadCloser //nil: END_STREAM on HEADERS, what curl and browsers send
		contentType string
	}{
		{name: "end of stream on headers"},
		{name: "empty data frame", body: emptyBody{}},
		{name: "empty data frame with Content-Type", body: emptyBody{}, contentType: "application/json"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			type framing struct {
				contentLength    int64
				transferEncoding []string
			}
			seen := make(chan framing, 1)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				seen <- framing{r.ContentLength, r.TransferEncoding}
			}))
			defer upstream.Close()

			proxy := newHTTP2Proxy(t, upstream.Listener.Addr().String(), &DpcoreOptions{})

			req, err := http.NewRequest(http.MethodPost, proxy.URL+"/api/v1/auth/refresh", nil)
			if err != nil {
				t.Fatal(err)
			}
			if tc.body != nil {
				req.Body = tc.body
			}
			if tc.contentType != "" {
				req.Header.Set("Content-Type", tc.contentType)
			}

			client := proxy.Client()
			client.Timeout = 5 * time.Second
			resp, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			resp.Body.Close()
			if resp.ProtoMajor != 2 {
				t.Fatalf("expected HTTP/2 between client and proxy, got %s", resp.Proto)
			}
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("got status %d", resp.StatusCode)
			}

			select {
			case got := <-seen:
				if got.contentLength != 0 || len(got.transferEncoding) != 0 {
					t.Fatalf("upstream got Content-Length=%d Transfer-Encoding=%v, want Content-Length=0 and no chunking", got.contentLength, got.transferEncoding)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("the request never reached the upstream")
			}
		})
	}
}

// gatedReader holds every Read until gate is closed
type gatedReader struct {
	gate <-chan struct{}
	r    io.Reader
}

func (g *gatedReader) Read(p []byte) (int, error) {
	<-g.gate
	return g.r.Read(p)
}

// Whatever the probe read takes from the body, on time or late, has to come
// back out of the body handed to the transport: nothing lost, nothing twice.
func TestMarkEmptyBodyKeepsBodyIntact(t *testing.T) {
	errBroken := errors.New("broken body")
	contents := []struct {
		name    string
		reader  func() io.Reader
		want    string
		wantErr error
	}{
		{name: "data", reader: func() io.Reader { return strings.NewReader("hello") }, want: "hello"},
		{name: "last byte with EOF", reader: func() io.Reader { return iotest.DataErrReader(strings.NewReader("h")) }, want: "h"},
		{name: "empty", reader: func() io.Reader { return strings.NewReader("") }},
		{name: "error", reader: func() io.Reader { return iotest.ErrReader(errBroken) }, wantErr: errBroken},
	}
	for _, late := range []bool{false, true} {
		for _, tc := range contents {
			name := "on time/" + tc.name
			if late {
				name = "late/" + tc.name
			}
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				gate := make(chan struct{})
				if !late {
					close(gate)
				}
				req := &http.Request{
					Method:        http.MethodPost,
					ContentLength: -1,
					Body:          io.NopCloser(&gatedReader{gate: gate, r: tc.reader()}),
				}

				returned := make(chan struct{})
				go func() {
					markEmptyBody(req)
					close(returned)
				}()
				select {
				case <-returned:
				case <-time.After(5 * time.Second):
					close(gate)
					t.Fatal("markEmptyBody is still waiting for the first body byte")
				}
				if late {
					close(gate)
				}

				// Only a body that ended before the probe gave up is known to be empty
				wantNoBody := !late && tc.want == "" && tc.wantErr == nil
				if gotNoBody := req.Body == http.NoBody; gotNoBody != wantNoBody {
					t.Fatalf("body replaced with http.NoBody = %v, want %v", gotNoBody, wantNoBody)
				}
				if wantNoBody {
					if req.ContentLength != 0 {
						t.Fatalf("ContentLength = %d, want 0", req.ContentLength)
					}
					return
				}
				if req.ContentLength != -1 {
					t.Fatalf("ContentLength = %d, want -1", req.ContentLength)
				}

				got, err := io.ReadAll(req.Body)
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("read error = %v, want %v", err, tc.wantErr)
				}
				if string(got) != tc.want {
					t.Fatalf("body = %q, want %q", got, tc.want)
				}
			})
		}
	}
}

// blockedBody stays in Read until it is closed
type blockedBody struct {
	closeOnce sync.Once
	closed    chan struct{}
	readDone  chan struct{}
}

func (b *blockedBody) Read([]byte) (int, error) {
	<-b.closed
	close(b.readDone)
	return 0, http.ErrBodyReadAfterClose
}

func (b *blockedBody) Close() error {
	b.closeOnce.Do(func() { close(b.closed) })
	return nil
}

// A probe that gave up leaves its read running. Closing the body, which the
// transport and the server both do, has to end that read: otherwise every
// stream that never writes would keep a goroutine behind.
func TestClosingBodyEndsProbeReadStillInFlight(t *testing.T) {
	t.Parallel()

	body := &blockedBody{closed: make(chan struct{}), readDone: make(chan struct{})}
	req := &http.Request{Method: http.MethodPost, ContentLength: -1, Body: body}

	returned := make(chan struct{})
	go func() {
		markEmptyBody(req)
		close(returned)
	}()
	select {
	case <-returned:
	case <-time.After(5 * time.Second):
		body.Close()
		t.Fatal("markEmptyBody is still waiting for the first body byte")
	}
	if req.Body == http.NoBody {
		t.Fatal("a body that has not ended was replaced with http.NoBody")
	}

	if err := req.Body.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-body.readDone:
	case <-time.After(5 * time.Second):
		t.Fatal("the probe read is still running after the body was closed")
	}
	if _, err := req.Body.Read(make([]byte, 1)); !errors.Is(err, http.ErrBodyReadAfterClose) {
		t.Fatalf("read after close = %v, want %v", err, http.ErrBodyReadAfterClose)
	}
}
