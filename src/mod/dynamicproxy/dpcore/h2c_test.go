package dpcore_test

import (
	"context"
	"crypto/tls"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
)

func h2cTestFrontend(t *testing.T, upstream string, opts *dpcore.DpcoreOptions) *httptest.Server {
	t.Helper()
	target, err := url.Parse(upstream)
	if err != nil {
		t.Fatal(err)
	}
	proxy := dpcore.NewDynamicProxyCore(target, "", opts)
	t.Cleanup(proxy.Transport.(*http.Transport).CloseIdleConnections)
	front := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		code, err := proxy.ServeHTTP(w, r, &dpcore.ResponseRewriteRuleSet{ProxyDomain: target.Host, OriginalHost: r.Host})
		if err != nil {
			http.Error(w, err.Error(), code)
		}
	}))
	front.EnableHTTP2 = true
	front.StartTLS()
	t.Cleanup(front.Close)
	return front
}

// Exercise actual gRPC framing, metadata, trailers, server streaming and cancellation
// through an encrypted frontend and a plaintext HTTP/2 backend.
func TestH2CGRPC(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	checkMetadata := func(ctx context.Context) error {
		md, _ := metadata.FromIncomingContext(ctx)
		if got := md.Get("x-api-key"); len(got) != 1 || got[0] != "test-project-key" {
			return status.Error(codes.Unauthenticated, "missing project key")
		}
		return nil
	}
	streamStopped := make(chan struct{}, 1)
	backend := grpc.NewServer(
		grpc.UnaryInterceptor(func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
			if err := checkMetadata(ctx); err != nil {
				return nil, err
			}
			return handler(ctx, req)
		}),
		grpc.StreamInterceptor(func(srv any, stream grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
			if err := checkMetadata(stream.Context()); err != nil {
				return err
			}
			err := handler(srv, stream)
			streamStopped <- struct{}{}
			return err
		}),
	)
	healthServer := health.NewServer()
	healthServer.SetServingStatus("project", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(backend, healthServer)
	go func() { _ = backend.Serve(listener) }()
	t.Cleanup(backend.Stop)
	front := h2cTestFrontend(t, "http://"+listener.Addr().String(), &dpcore.DpcoreOptions{UseH2C: true, FlushInterval: 10 * time.Second})
	conn, err := grpc.NewClient(strings.TrimPrefix(front.URL, "https://"), grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{InsecureSkipVerify: true})))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	client := healthpb.NewHealthClient(conn)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ctx = metadata.AppendToOutgoingContext(ctx, "x-api-key", "test-project-key")
	response, err := client.Check(ctx, &healthpb.HealthCheckRequest{Service: "project"})
	if err != nil || response.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("unary export: response=%v, error=%v", response, err)
	}
	if _, err := client.Check(ctx, &healthpb.HealthCheckRequest{Service: "missing"}); status.Code(err) != codes.NotFound {
		t.Fatalf("gRPC error trailer lost: %v", err)
	}
	unauthCtx, unauthCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer unauthCancel()
	if _, err := client.Check(unauthCtx, &healthpb.HealthCheckRequest{Service: "project"}); status.Code(err) != codes.Unauthenticated {
		t.Fatalf("authentication failure not forwarded: %v", err)
	}
	watchCtx, stopWatch := context.WithCancel(ctx)
	defer stopWatch()
	stream, err := client.Watch(watchCtx, &healthpb.HealthCheckRequest{Service: "project"})
	if err != nil {
		t.Fatal(err)
	}
	first, err := stream.Recv()
	if err != nil || first.GetStatus() != healthpb.HealthCheckResponse_SERVING {
		t.Fatalf("first streaming response: %v, %v", first, err)
	}
	healthServer.SetServingStatus("project", healthpb.HealthCheckResponse_NOT_SERVING)
	second, err := stream.Recv()
	if err != nil || second.GetStatus() != healthpb.HealthCheckResponse_NOT_SERVING {
		t.Fatalf("next streaming response: %v, %v", second, err)
	}
	stopWatch()
	select {
	case <-streamStopped:
	case <-ctx.Done():
		t.Fatal("client cancellation did not reach the gRPC backend")
	}
}

func TestH2COptIn(t *testing.T) {
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Upstream-Protocol", r.Proto)
		w.Header().Set("X-Upstream-TE", r.Header.Get("Te"))
		w.WriteHeader(http.StatusOK)
	}))
	upstream.Config.Protocols = new(http.Protocols)
	upstream.Config.Protocols.SetHTTP1(true)
	upstream.Config.Protocols.SetUnencryptedHTTP2(true)
	upstream.Start()
	defer upstream.Close()
	for _, tc := range []struct {
		name string
		h2c  bool
		want string
	}{{"default", false, "HTTP/1.1"}, {"h2c", true, "HTTP/2.0"}} {
		t.Run(tc.name, func(t *testing.T) {
			front := h2cTestFrontend(t, upstream.URL, &dpcore.DpcoreOptions{UseH2C: tc.h2c})
			req, _ := http.NewRequest("POST", front.URL+"/export", strings.NewReader("payload"))
			req.Header.Set("Te", "trailers")
			client := front.Client()
			client.Timeout = 5 * time.Second
			res, err := client.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer res.Body.Close()
			if got := res.Header.Get("X-Upstream-Protocol"); got != tc.want {
				t.Errorf("upstream used %q, want %q", got, tc.want)
			}
			if got := res.Header.Get("X-Upstream-TE"); got != "trailers" {
				t.Errorf("TE metadata lost: %q", got)
			}
		})
	}
}

// An h2c backend may send headers before the client supplies its first message.
// Reading ahead in the request body before RoundTrip deadlocks this exchange.
func TestH2CFullDuplex(t *testing.T) {
	upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/grpc")
		w.WriteHeader(http.StatusOK)
		w.(http.Flusher).Flush()
		_, _ = io.Copy(w, r.Body)
	}))
	upstream.Config.Protocols = new(http.Protocols)
	upstream.Config.Protocols.SetUnencryptedHTTP2(true)
	upstream.Start()
	defer upstream.Close()
	front := h2cTestFrontend(t, upstream.URL, &dpcore.DpcoreOptions{UseH2C: true})
	reader, writer := io.Pipe()
	defer reader.Close()
	defer writer.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "POST", front.URL+"/stream", reader)
	res, err := front.Client().Do(req)
	if err != nil {
		t.Fatalf("backend headers blocked on request body: %v", err)
	}
	defer res.Body.Close()
	go func() {
		_, _ = io.WriteString(writer, "stream-message")
		_ = writer.Close()
	}()
	body, err := io.ReadAll(res.Body)
	if err != nil || string(body) != "stream-message" {
		t.Fatalf("duplex body = %q, error = %v", body, err)
	}
}

func TestH2CValidation(t *testing.T) {
	for _, tc := range []struct {
		name, target             string
		h2c, tls, http1, wantErr bool
	}{
		{"legacy TLS", "example.com", false, true, false, false},
		{"plaintext", "localhost:4317", true, false, false, false},
		{"TLS flag", "localhost:4317", true, true, false, true},
		{"HTTPS URL", "https://localhost:4317", true, false, false, true},
		{"forced HTTP1", "localhost:4317", true, false, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := dpcore.ValidateH2C(tc.target, tc.h2c, tc.tls, tc.http1)
			if (err != nil) != tc.wantErr {
				t.Fatalf("validation error = %v, want error = %v", err, tc.wantErr)
			}
		})
	}
}
