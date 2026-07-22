package dpcore

import (
	"mime"
	"net/http"
	"strings"
	"time"
)

// Auto sniff of flush interval from header
func (p *ReverseProxy) getFlushInterval(req *http.Request, res *http.Response) time.Duration {
	contentType := res.Header.Get("Content-Type")
	if actualContentType, _, _ := mime.ParseMediaType(contentType); actualContentType == "text/event-stream" {
		return -1
	}

	//An unknown response length means the upstream is streaming, e.g. chunked
	//transfer encoding. See issue #1231.
	if res.ContentLength == -1 || p.isBidirectionalStream(req, res) {
		return -1
	}

	// Fixed issue #235: Added auto detection for ollama / llm output stream
	connectionHeader := req.Header["Connection"]
	if len(connectionHeader) > 0 && strings.Contains(strings.Join(connectionHeader, ","), "keep-alive") {
		return -1
	}

	//Cannot sniff anything. Use default value
	return p.FlushInterval

}

// Check for bidirectional stream, copy from Caddy :D
func (p *ReverseProxy) isBidirectionalStream(req *http.Request, res *http.Response) bool {
	// We have to check the encoding here; only flush headers with identity encoding.
	// Non-identity encoding might combine with "encode" directive, and in that case,
	// if body size larger than enc.MinLength, upper level encode handle might have
	// Content-Encoding header to write.
	// (see https://github.com/caddyserver/caddy/issues/3606 for use case)
	ae := req.Header.Get("Accept-Encoding")

	return req.ProtoMajor == 2 &&
		res.ProtoMajor == 2 &&
		res.ContentLength == -1 &&
		(ae == "identity" || ae == "")
}
