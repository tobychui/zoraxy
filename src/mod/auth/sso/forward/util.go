package forward

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

func scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}

	return "http"
}

func headerCookieRedact(r *http.Request, names []string, exclude bool) {
	if len(names) == 0 {
		return
	}

	original := r.Cookies()

	if len(original) == 0 {
		return
	}

	var cookies []string

	for _, cookie := range original {
		if exclude && stringInSlice(cookie.Name, names) {
			continue
		} else if !exclude && !stringInSlice(cookie.Name, names) {
			continue
		}

		cookies = append(cookies, cookie.String())
	}

	value := strings.Join(cookies, "; ")

	r.Header.Set(HeaderCookie, value)

	return
}

func headerCopyExcluded(original, destination http.Header, excludedHeaders []string) {
	for key, values := range original {
		// We should never copy the headers in the below list.
		if stringInSliceFold(key, doNotCopyHeaders) {
			continue
		}

		if stringInSliceFold(key, excludedHeaders) {
			continue
		}

		destination[key] = append(destination[key], values...)
	}
}

func headerCopyIncluded(original, destination http.Header, includedHeaders []string, allIfEmpty bool) {
	if allIfEmpty && len(includedHeaders) == 0 {
		headerCopyAll(original, destination)
	} else {
		headerCopyIncludedExact(original, destination, includedHeaders)
	}
}

func headerCopyAll(original, destination http.Header) {
	for key, values := range original {
		// We should never copy the headers in the below list, even if they're in the list provided by a user.
		if stringInSliceFold(key, doNotCopyHeaders) {
			continue
		}

		destination[key] = append(destination[key], values...)
	}
}

func headerCopyIncludedExact(original, destination http.Header, keys []string) {
	for key, values := range original {
		// We should never copy the headers in the below list, even if they're in the list provided by a user.
		if stringInSliceFold(key, doNotCopyHeaders) {
			continue
		}

		if !stringInSliceFold(key, keys) {
			continue
		}

		destination[key] = append(destination[key], values...)
	}
}

func stringInSlice(needle string, haystack []string) bool {
	if len(haystack) == 0 {
		return false
	}

	for _, v := range haystack {
		if needle == v {
			return true
		}
	}

	return false
}

func stringInSliceFold(needle string, haystack []string) bool {
	if len(haystack) == 0 {
		return false
	}

	for _, v := range haystack {
		if strings.EqualFold(needle, v) {
			return true
		}
	}

	return false
}

func rSetIPHeader(r, req *http.Request, headers ...string) {
	if r.RemoteAddr == "" || len(headers) == 0 {
		return
	}

	before, _, _ := strings.Cut(r.RemoteAddr, ":")

	ip := net.ParseIP(before)
	if ip == nil {
		return
	}

	for _, header := range headers {
		req.Header.Set(header, ip.String())
	}
}

func rSetXForwardedHeaders(r, req *http.Request) {
	rSetIPHeader(r, req, HeaderXForwardedFor)
	req.Header.Set(HeaderXForwardedMethod, r.Method)
	req.Header.Set(HeaderXForwardedProto, scheme(r))
	req.Header.Set(HeaderXForwardedHost, r.Host)
	req.Header.Set(HeaderXForwardedURI, r.URL.Path)
}

func rSetXOriginalHeaders(r, req *http.Request) {
	// The X-Forwarded-For header has larger support, so we include both.
	rSetIPHeader(r, req, HeaderXOriginalIP, HeaderXForwardedFor)

	original := &url.URL{
		Scheme: scheme(r),
		Host:   r.Host,
		Path:   r.URL.Path,
	}

	req.Header.Set(HeaderXOriginalMethod, r.Method)
	req.Header.Set(HeaderXOriginalURL, original.String())
}

func rCopyBody(req, freq *http.Request) (err error) {
	body, err := io.ReadAll(req.Body)
	if err != nil {
		return err
	}

	if len(body) == 0 {
		return nil
	}

	req.Body = io.NopCloser(bytes.NewReader(body))
	freq.Body = io.NopCloser(bytes.NewReader(body))

	return nil
}

func cleanSplit(s string) []string {
	if s == "" {
		return nil
	}

	return strings.Split(s, ",")
}
