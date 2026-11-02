package dpcore_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
)

// TestChunkedResponseReachesClientBeforeUpstreamFinishes reproduces the shape of
// issue #1231: a POST carrying a request body whose response is streamed chunked
// with no Content-Length. The response must reach the client as the upstream
// produces it, rather than being held back until the upstream is done.
func TestChunkedResponseReachesClientBeforeUpstreamFinishes(t *testing.T) {
	const (
		chunkCount = 3
		chunkDelay = 500 * time.Millisecond
		chunkBody  = "chunk"
	)

	//Upstream streams small chunks slowly and never sets a Content-Length
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/zip")
		w.WriteHeader(http.StatusOK)
		for i := 0; i < chunkCount; i++ {
			w.Write([]byte(chunkBody))
			w.(http.Flusher).Flush()
			time.Sleep(chunkDelay)
		}
	}))
	defer upstream.Close()

	upstreamURL, err := url.Parse(upstream.URL)
	if err != nil {
		t.Fatal(err)
	}

	//A flush interval far longer than the test would ever wait. If the streamed
	//response is not detected, the chunks are batched behind this instead.
	proxy := dpcore.NewDynamicProxyCore(upstreamURL, "", &dpcore.DpcoreOptions{
		FlushInterval: 10 * time.Second,
	})

	front := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxy.ServeHTTP(w, r, &dpcore.ResponseRewriteRuleSet{
			ProxyDomain:  upstreamURL.Host,
			OriginalHost: r.Host,
		})
	}))
	defer front.Close()

	req, err := http.NewRequest("POST", front.URL+"/api/download/archive", strings.NewReader(`{"assetIds":["a"]}`))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd") //As a browser would send

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	//Time to the first byte of the body, which is what stalls in issue #1231
	buf := make([]byte, len(chunkBody))
	if _, err := io.ReadFull(resp.Body, buf); err != nil {
		t.Fatalf("reading first chunk: %v", err)
	}
	firstChunk := time.Since(start)

	//The upstream runs for chunkCount*chunkDelay. Arriving well inside that window
	//is only possible if the chunk was passed through rather than buffered.
	if firstChunk > chunkDelay-200*time.Millisecond {
		t.Errorf("first chunk took %v, expected it to arrive promptly; the response is being buffered until the upstream finishes", firstChunk)
	}

	rest, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("reading remaining body: %v", err)
	}
	if got, want := string(buf)+string(rest), strings.Repeat(chunkBody, chunkCount); got != want {
		t.Errorf("body = %q, want %q", got, want)
	}
}
