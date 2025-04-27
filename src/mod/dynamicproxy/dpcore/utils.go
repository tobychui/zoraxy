package dpcore

import (
	"bytes"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// replaceLocationHost rewrite the backend server's location header to a new URL based on the given proxy rules
// If you have issues with tailing slash, you can try to fix them here (and remember to PR :D )
func replaceLocationHost(urlString string, rrr *ResponseRewriteRuleSet, useTLS bool) (string, error) {
	u, err := url.Parse(urlString)
	if err != nil {
		return "", err
	}

	//Update the schemetic if the proxying target is http
	//but exposed as https to the internet via Zoraxy
	if useTLS {
		u.Scheme = "https"
	} else {
		u.Scheme = "http"
	}

	//Issue #39: Check if it is location target match the proxying domain
	//E.g. Proxy config: blog.example.com -> example.com/blog
	//Check if it is actually redirecting to example.com instead of a new domain
	//like news.example.com.
	// The later check bypass apache screw up method of redirection header
	// e.g. https://imuslab.com -> http://imuslab.com:443
	if rrr.ProxyDomain != u.Host && !strings.Contains(u.Host, rrr.OriginalHost+":") {
		//New location domain not matching proxy target domain.
		//Do not modify location header
		return urlString, nil
	}

	//Issue #626: Check if the location header is another subdomain with port
	//E.g. Proxy config: blog.example.com -> 127.0.0.1:80
	//Check if it is actually redirecting to (*.)blog.example.com:8080 instead of current domain
	//like Location: http://x.blog.example.com:1234/
	_, newLocationPort, err := net.SplitHostPort(u.Host)
	if (newLocationPort == "80" || newLocationPort == "443") && err == nil {
		//Port 80 or 443, some web server use this to switch between http and https
		//E.g. http://example.com:80 -> https://example.com:443
		//E.g. http://example.com:443 -> https://example.com:80
		//That usually means the user have invalidly configured the web server to use port 80 or 443
		//for http or https. We should not modify the location header in this case.

	} else if strings.Contains(u.Host, ":") && err == nil {
		//Other port numbers. Do not modify location header
		return urlString, nil
	}

	u.Host = rrr.OriginalHost

	if strings.Contains(rrr.ProxyDomain, "/") {
		//The proxy domain itself seems contain subpath.
		//Trim it off from Location header to prevent URL segment duplicate
		//E.g. Proxy config: blog.example.com -> example.com/blog
		//Location Header: /blog/post?id=1
		//Expected Location Header send to client:
		// blog.example.com/post?id=1 instead of blog.example.com/blog/post?id=1

		ProxyDomainURL := "http://" + rrr.ProxyDomain
		if rrr.UseTLS {
			ProxyDomainURL = "https://" + rrr.ProxyDomain
		}
		ru, err := url.Parse(ProxyDomainURL)
		if err == nil {
			//Trim off the subpath
			u.Path = strings.TrimPrefix(u.Path, ru.Path)
		}
	}

	return u.String(), nil
}

// Debug functions for replaceLocationHost
func ReplaceLocationHost(urlString string, rrr *ResponseRewriteRuleSet, useTLS bool) (string, error) {
	return replaceLocationHost(urlString, rrr, useTLS)
}

// isExternalDomainName check and return if the hostname is external domain name (e.g. github.com)
// instead of internal (like 192.168.1.202:8443 (ip address) or domains end with .local or .internal)
func isExternalDomainName(hostname string) bool {
	host, _, err := net.SplitHostPort(hostname)
	if err != nil {
		//hostname doesnt contain port
		ip := net.ParseIP(hostname)
		if ip != nil {
			//IP address, not a domain name
			return false
		}
	} else {
		//Hostname contain port, use hostname without port to check if it is ip
		ip := net.ParseIP(host)
		if ip != nil {
			//IP address, not a domain name
			return false
		}
	}

	//Check if it is internal DNS assigned domains
	internalDNSTLD := []string{".local", ".internal", ".localhost", ".home.arpa"}
	for _, tld := range internalDNSTLD {
		if strings.HasSuffix(strings.ToLower(hostname), tld) {
			return false
		}
	}

	return true
}

// DeepCopyRequest returns a deep copy of the given http.Request.
func DeepCopyRequest(req *http.Request) (*http.Request, error) {
	// Copy the URL
	urlCopy := *req.URL

	// Copy the headers
	headersCopy := make(http.Header, len(req.Header))
	for k, vv := range req.Header {
		vvCopy := make([]string, len(vv))
		copy(vvCopy, vv)
		headersCopy[k] = vvCopy
	}

	// Copy the cookies
	cookiesCopy := make([]*http.Cookie, len(req.Cookies()))
	for i, cookie := range req.Cookies() {
		cookieCopy := *cookie
		cookiesCopy[i] = &cookieCopy
	}

	// Copy the body, if present
	var bodyCopy io.ReadCloser
	if req.Body != nil {
		var buf bytes.Buffer
		if _, err := buf.ReadFrom(req.Body); err != nil {
			return nil, err
		}
		// Reset the request body so it can be read again
		if err := req.Body.Close(); err != nil {
			return nil, err
		}
		req.Body = io.NopCloser(&buf)
		bodyCopy = io.NopCloser(bytes.NewReader(buf.Bytes()))
	}

	// Create the new request
	reqCopy := &http.Request{
		Method:           req.Method,
		URL:              &urlCopy,
		Proto:            req.Proto,
		ProtoMajor:       req.ProtoMajor,
		ProtoMinor:       req.ProtoMinor,
		Header:           headersCopy,
		Body:             bodyCopy,
		ContentLength:    req.ContentLength,
		TransferEncoding: append([]string(nil), req.TransferEncoding...),
		Close:            req.Close,
		Host:             req.Host,
		Form:             req.Form,
		PostForm:         req.PostForm,
		MultipartForm:    req.MultipartForm,
		Trailer:          req.Trailer,
		RemoteAddr:       req.RemoteAddr,
		TLS:              req.TLS,
		// Cancel and Context are not copied as it might cause issues
	}

	return reqCopy, nil
}
