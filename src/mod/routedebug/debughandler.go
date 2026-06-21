package routedebug

import (
	"crypto/tls"
	"fmt"
	"html"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

const maxBodyPreviewBytes = 4096

// requestDebugInfo holds every piece of information extracted from an incoming
// request. It is built once and passed to whichever renderer is active so that
// both plain-text and HTML outputs share the same data-gathering logic.
type requestDebugInfo struct {
	Timestamp string

	// Request line
	Method     string
	RequestURI string // raw target as received (before URL parsing)
	ParsedURL  string // r.URL.String() after proxy rewriting
	Proto      string

	// Host header as parsed by Go's HTTP server
	Host string

	// Headers (sorted for readability)
	HeaderKeys []string
	Headers    http.Header

	// Query parameters (sorted)
	QueryKeys   []string
	QueryParams map[string][]string

	// Cookies parsed out of the Cookie header (sorted by name)
	CookieNames []string
	Cookies     map[string]string

	// Content metadata declared in the request
	ContentLength    int64
	TransferEncoding []string

	// Body
	BodyPreview   string
	BodySize      int  // actual bytes read; -1 when truncated
	BodyTruncated bool

	// Connection
	RemoteAddr string

	// TLS (only populated when r.TLS != nil)
	IsTLS         bool
	TLSVersion    string
	TLSCipher     string
	TLSServerName string
	TLSNextProto  string
}

// handleDebugRequest echoes every detail of the incoming request back to the
// caller, either as plain text or as a styled HTML page depending on PrettyPrint.
func (rd *RouteDebugger) handleDebugRequest(w http.ResponseWriter, r *http.Request) {
	info := gatherDebugInfo(r)
	if rd.option.PrettyPrint {
		rd.renderHTML(w, info)
	} else {
		rd.renderPlainText(w, info)
	}
}

// gatherDebugInfo collects all request fields into a requestDebugInfo struct.
func gatherDebugInfo(r *http.Request) requestDebugInfo {
	info := requestDebugInfo{
		Timestamp:  time.Now().UTC().Format("2006-01-02 15:04:05 UTC"),
		Method:     r.Method,
		RequestURI: r.RequestURI,
		ParsedURL:  r.URL.String(),
		Proto:      r.Proto,
		Host:       r.Host,
		RemoteAddr: r.RemoteAddr,

		ContentLength:    r.ContentLength,
		TransferEncoding: r.TransferEncoding,

		Headers:     r.Header,
		QueryParams: r.URL.Query(),
	}

	// Sorted header keys.
	for k := range r.Header {
		info.HeaderKeys = append(info.HeaderKeys, k)
	}
	sort.Strings(info.HeaderKeys)

	// Sorted query-parameter keys.
	for k := range info.QueryParams {
		info.QueryKeys = append(info.QueryKeys, k)
	}
	sort.Strings(info.QueryKeys)

	// Cookies (parsed from header, deduplicated by last value).
	info.Cookies = make(map[string]string)
	for _, c := range r.Cookies() {
		info.Cookies[c.Name] = c.Value
		info.CookieNames = append(info.CookieNames, c.Name)
	}
	// Deduplicate cookie names in case of multiple Set-Cookie with same name.
	seen := make(map[string]bool)
	unique := info.CookieNames[:0]
	for _, n := range info.CookieNames {
		if !seen[n] {
			seen[n] = true
			unique = append(unique, n)
		}
	}
	info.CookieNames = unique
	sort.Strings(info.CookieNames)

	// Body preview.
	if r.Body != nil {
		raw, err := io.ReadAll(io.LimitReader(r.Body, maxBodyPreviewBytes+1))
		if err == nil {
			if len(raw) > maxBodyPreviewBytes {
				raw = raw[:maxBodyPreviewBytes]
				info.BodyTruncated = true
				info.BodySize = -1
			} else {
				info.BodySize = len(raw)
			}
			info.BodyPreview = string(raw)
		}
	}

	// TLS details.
	if r.TLS != nil {
		info.IsTLS = true
		info.TLSVersion = tlsVersionName(r.TLS.Version)
		info.TLSCipher = tls.CipherSuiteName(r.TLS.CipherSuite)
		info.TLSServerName = r.TLS.ServerName
		info.TLSNextProto = r.TLS.NegotiatedProtocol
	}

	return info
}

// tlsVersionName converts a tls.Version* constant to a human-readable string.
func tlsVersionName(v uint16) string {
	switch v {
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("0x%04x", v)
	}
}

// ── Plain-text renderer ───────────────────────────────────────────────────────

func (rd *RouteDebugger) renderPlainText(w http.ResponseWriter, info requestDebugInfo) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")

	var sb strings.Builder
	kw := 28 // key column width

	line := func(k, v string) {
		sb.WriteString(fmt.Sprintf("  %-*s %s\n", kw, k+":", v))
	}
	section := func(title string) {
		sb.WriteString("\n")
		sb.WriteString(title + "\n")
		sb.WriteString(strings.Repeat("-", len(title)) + "\n")
	}

	sb.WriteString("Zoraxy Route Debugger\n")
	sb.WriteString(fmt.Sprintf("Generated: %s\n", info.Timestamp))

	section("REQUEST")
	line("Method", info.Method)
	line("Request-URI", info.RequestURI)
	line("Parsed URL", info.ParsedURL)
	line("Protocol", info.Proto)
	line("Host", info.Host)

	section("CLIENT")
	line("Remote Address", info.RemoteAddr)
	line("Content-Length", fmt.Sprintf("%d", info.ContentLength))
	if len(info.TransferEncoding) > 0 {
		line("Transfer-Encoding", strings.Join(info.TransferEncoding, ", "))
	} else {
		line("Transfer-Encoding", "(none)")
	}

	section("TLS")
	if info.IsTLS {
		line("Enabled", "Yes")
		line("Version", info.TLSVersion)
		line("Cipher Suite", info.TLSCipher)
		line("Server Name (SNI)", info.TLSServerName)
		line("ALPN Protocol", info.TLSNextProto)
	} else {
		line("Enabled", "No (plain HTTP)")
	}

	section("HEADERS")
	if len(info.HeaderKeys) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		for _, k := range info.HeaderKeys {
			for _, v := range info.Headers[k] {
				line(k, v)
			}
		}
	}

	section("COOKIES")
	if len(info.CookieNames) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		for _, name := range info.CookieNames {
			line(name, info.Cookies[name])
		}
	}

	section("QUERY PARAMETERS")
	if len(info.QueryKeys) == 0 {
		sb.WriteString("  (none)\n")
	} else {
		for _, k := range info.QueryKeys {
			for _, v := range info.QueryParams[k] {
				line(k, v)
			}
		}
	}

	bodyLabel := fmt.Sprintf("BODY (%d bytes)", info.BodySize)
	if info.BodyTruncated {
		bodyLabel = fmt.Sprintf("BODY (>%d bytes — preview only)", maxBodyPreviewBytes)
	}
	section(bodyLabel)
	if info.BodyPreview == "" {
		sb.WriteString("  (empty)\n")
	} else {
		for _, ln := range strings.Split(info.BodyPreview, "\n") {
			sb.WriteString("  " + ln + "\n")
		}
	}

	fmt.Fprint(w, sb.String())
}

// ── HTML renderer ─────────────────────────────────────────────────────────────

const htmlTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Route Debugger</title>
<style>
*{box-sizing:border-box;margin:0;padding:0}
body{font-family:-apple-system,BlinkMacSystemFont,'Helvetica Neue',Arial,sans-serif;font-size:13px;background:#fff;color:#1d1d1f;padding:32px 40px;line-height:1.5;max-width:960px}
h1{font-size:17px;font-weight:600;color:#1d1d1f;margin-bottom:2px}
.generated{font-size:11px;color:#8a8a8e;margin-bottom:28px}
.section{margin-bottom:28px}
.section-title{font-size:11px;font-weight:600;color:#6e6e73;text-transform:uppercase;letter-spacing:.06em;margin-bottom:8px;padding-bottom:5px;border-bottom:1px solid #d2d2d7}
.request-line{font-family:'SF Mono','Menlo','Courier New',monospace;font-size:13px;color:#1d1d1f;background:#f5f5f7;border:1px solid #d2d2d7;border-radius:6px;padding:10px 14px;word-break:break-all}
.method{font-weight:700;margin-right:10px}
.proto{color:#6e6e73;margin-left:10px}
table{border-collapse:collapse;width:100%}
tr:last-child td{border-bottom:none}
td{padding:5px 0;border-bottom:1px solid #f0f0f0;vertical-align:top;word-break:break-all}
td.k{color:#1d1d1f;font-weight:500;white-space:nowrap;width:36%;padding-right:16px}
td.v{color:#3c3c43;font-family:'SF Mono','Menlo','Courier New',monospace;font-size:12px}
td.empty{color:#8a8a8e;font-style:italic}
pre{font-family:'SF Mono','Menlo','Courier New',monospace;font-size:12px;background:#f5f5f7;border:1px solid #d2d2d7;border-radius:6px;padding:12px;overflow:auto;white-space:pre-wrap;word-break:break-all;color:#1d1d1f}
.body-note{font-size:11px;color:#8a8a8e;margin-bottom:6px}
</style>
</head>
<body>
<h1>Zoraxy Route Debugger</h1>
<div class="generated">%s</div>

<div class="section">
  <div class="section-title">Request Line</div>
  <div class="request-line"><span class="method">%s</span>%s<span class="proto">%s</span></div>
</div>

<div class="section">
  <div class="section-title">Request Details</div>
  <table>
    <tr><td class="k">Host</td><td class="v">%s</td></tr>
    <tr><td class="k">Request-URI (raw)</td><td class="v">%s</td></tr>
    <tr><td class="k">Parsed URL</td><td class="v">%s</td></tr>
    <tr><td class="k">Protocol</td><td class="v">%s</td></tr>
    <tr><td class="k">Content-Length</td><td class="v">%s</td></tr>
    <tr><td class="k">Transfer-Encoding</td><td class="v">%s</td></tr>
  </table>
</div>

<div class="section">
  <div class="section-title">Client &amp; Connection</div>
  <table>
    <tr><td class="k">Remote Address</td><td class="v">%s</td></tr>
    <tr><td class="k">TLS</td><td class="v">%s</td></tr>
    %s
  </table>
</div>

<div class="section">
  <div class="section-title">Headers</div>
  <table>%s</table>
</div>

<div class="section">
  <div class="section-title">Cookies</div>
  <table>%s</table>
</div>

<div class="section">
  <div class="section-title">Query Parameters</div>
  <table>%s</table>
</div>

<div class="section">
  <div class="section-title">Request Body</div>
  <div class="body-note">%s</div>
  %s
</div>
</body>
</html>`

func (rd *RouteDebugger) renderHTML(w http.ResponseWriter, info requestDebugInfo) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	e := html.EscapeString

	// TLS summary value and optional extra rows.
	tlsValue := "No (plain HTTP)"
	var tlsExtraRows strings.Builder
	if info.IsTLS {
		tlsValue = "Yes"
		tlsRow := func(k, v string) {
			tlsExtraRows.WriteString(fmt.Sprintf(
				"<tr><td class=\"k\">%s</td><td class=\"v\">%s</td></tr>",
				e(k), e(v),
			))
		}
		tlsRow("TLS Version", info.TLSVersion)
		tlsRow("Cipher Suite", info.TLSCipher)
		if info.TLSServerName != "" {
			tlsRow("Server Name (SNI)", info.TLSServerName)
		}
		if info.TLSNextProto != "" {
			tlsRow("ALPN Protocol", info.TLSNextProto)
		}
	}

	// Content-Length display.
	clStr := fmt.Sprintf("%d", info.ContentLength)
	if info.ContentLength == -1 {
		clStr = "unknown (-1)"
	}

	// Transfer-Encoding display.
	teStr := "(none)"
	if len(info.TransferEncoding) > 0 {
		teStr = strings.Join(info.TransferEncoding, ", ")
	}

	// Header rows.
	headerRows := buildTableRows(info.HeaderKeys, func(k string) []string { return info.Headers[k] }, "No headers")

	// Cookie rows.
	cookieRows := buildTableRows(info.CookieNames, func(k string) []string { return []string{info.Cookies[k]} }, "No cookies")

	// Query-parameter rows.
	queryRows := buildTableRows(info.QueryKeys, func(k string) []string { return info.QueryParams[k] }, "No query parameters")

	// Body block.
	bodyNote := fmt.Sprintf("%d bytes", info.BodySize)
	if info.BodyTruncated {
		bodyNote = fmt.Sprintf("&gt;%d bytes — preview truncated", maxBodyPreviewBytes)
	}
	bodyBlock := `<span style="color:#8a8a8e;font-style:italic">Empty body</span>`
	if info.BodyPreview != "" {
		bodyBlock = fmt.Sprintf("<pre>%s</pre>", e(info.BodyPreview))
	}

	page := fmt.Sprintf(htmlTemplate,
		e(info.Timestamp),
		// Request line
		e(info.Method),
		e(info.ParsedURL),
		e(info.Proto),
		// Request details table
		e(info.Host),
		e(info.RequestURI),
		e(info.ParsedURL),
		e(info.Proto),
		e(clStr),
		e(teStr),
		// Client & connection
		e(info.RemoteAddr),
		e(tlsValue),
		tlsExtraRows.String(),
		// Sections
		headerRows,
		cookieRows,
		queryRows,
		bodyNote,
		bodyBlock,
	)

	fmt.Fprint(w, page)
}

// buildTableRows generates <tr> rows for a sorted key list using a value
// resolver. Emits a single "empty" row when the key list is empty.
func buildTableRows(keys []string, values func(string) []string, emptyMsg string) string {
	if len(keys) == 0 {
		return fmt.Sprintf(`<tr><td colspan="2" class="empty">%s</td></tr>`, html.EscapeString(emptyMsg))
	}
	var sb strings.Builder
	for _, k := range keys {
		for _, v := range values(k) {
			sb.WriteString(fmt.Sprintf(
				"<tr><td class=\"k\">%s</td><td class=\"v\">%s</td></tr>",
				html.EscapeString(k), html.EscapeString(v),
			))
		}
	}
	return sb.String()
}
