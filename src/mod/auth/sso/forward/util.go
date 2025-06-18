package forward

import (
	"net"
	"net/http"
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

func rSetForwardedHeaders(r, req *http.Request) {
	if r.RemoteAddr != "" {
		before, _, _ := strings.Cut(r.RemoteAddr, ":")

		if ip := net.ParseIP(before); ip != nil {
			req.Header.Set(HeaderXForwardedFor, ip.String())
		}
	}

	req.Header.Set(HeaderXForwardedMethod, r.Method)
	req.Header.Set(HeaderXForwardedProto, scheme(r))
	req.Header.Set(HeaderXForwardedHost, r.Host)
	req.Header.Set(HeaderXForwardedURI, r.URL.Path)
}
