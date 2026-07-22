package dpcore

import (
	"net/http"
	"testing"
	"time"
)

const testDefaultFlushInterval = 100 * time.Millisecond

// TestGetFlushInterval checks that streaming responses are detected from the
// response rather than the request, see issue #1231.
func TestGetFlushInterval(t *testing.T) {
	tests := []struct {
		name     string
		req      *http.Request
		res      *http.Response
		expected time.Duration
	}{
		{
			name: "Chunked download behind a POST with a body (issue #1231)",
			req: &http.Request{
				Method:        "POST",
				ProtoMajor:    2,
				ContentLength: 512, //JSON asset id list
				Header:        http.Header{"Accept-Encoding": {"gzip, deflate, br, zstd"}},
			},
			res: &http.Response{
				ProtoMajor:    1,
				ContentLength: -1, //Chunked, no Content-Length
				Header:        http.Header{"Content-Type": {"application/zip"}},
			},
			expected: -1,
		},
		{
			name: "SSE response",
			req: &http.Request{
				Method:        "GET",
				ProtoMajor:    2,
				ContentLength: 0,
				Header:        http.Header{"Accept": {"text/event-stream"}},
			},
			res: &http.Response{
				ProtoMajor:    1,
				ContentLength: -1,
				Header:        http.Header{"Content-Type": {"text/event-stream"}},
			},
			expected: -1,
		},
		{
			name: "SSE response with charset parameter",
			req: &http.Request{
				Method:        "GET",
				ProtoMajor:    2,
				ContentLength: 0,
				Header:        http.Header{},
			},
			res: &http.Response{
				ProtoMajor:    1,
				ContentLength: 4096, //Declared length, only the MIME type marks it a stream
				Header:        http.Header{"Content-Type": {"text/event-stream; charset=utf-8"}},
			},
			expected: -1,
		},
		{
			name: "Ollama style keep-alive stream (issue #235)",
			req: &http.Request{
				Method:        "POST",
				ProtoMajor:    1,
				ContentLength: 64,
				Header:        http.Header{"Connection": {"keep-alive"}},
			},
			res: &http.Response{
				ProtoMajor:    1,
				ContentLength: 128,
				Header:        http.Header{"Content-Type": {"application/json"}},
			},
			expected: -1,
		},
		{
			name: "Bidirectional HTTP/2 stream",
			req: &http.Request{
				Method:        "POST",
				ProtoMajor:    2,
				ContentLength: -1,
				Header:        http.Header{"Accept-Encoding": {"identity"}},
			},
			res: &http.Response{
				ProtoMajor:    2,
				ContentLength: -1,
				Header:        http.Header{},
			},
			expected: -1,
		},
		{
			name: "Ordinary response with a known length",
			req: &http.Request{
				Method:        "GET",
				ProtoMajor:    2,
				ContentLength: 0,
				Header:        http.Header{"Accept-Encoding": {"gzip, deflate, br, zstd"}},
			},
			res: &http.Response{
				ProtoMajor:    1,
				ContentLength: 2048,
				Header:        http.Header{"Content-Type": {"text/html"}},
			},
			expected: testDefaultFlushInterval,
		},
		{
			name: "Request without a body must not be mistaken for a stream",
			req: &http.Request{
				Method:        "POST",
				ProtoMajor:    2,
				ContentLength: -1, //Unknown request length, the response is what matters
				Header:        http.Header{"Accept-Encoding": {"gzip"}},
			},
			res: &http.Response{
				ProtoMajor:    1,
				ContentLength: 900,
				Header:        http.Header{"Content-Type": {"application/json"}},
			},
			expected: testDefaultFlushInterval,
		},
	}

	p := &ReverseProxy{FlushInterval: testDefaultFlushInterval}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.getFlushInterval(tt.req, tt.res)
			if got != tt.expected {
				t.Errorf("getFlushInterval() = %v, want %v", got, tt.expected)
			}
		})
	}
}
