package dpcore

import (
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
	if rrr.ProxyDomain != u.Host {
		//New location domain not matching proxy target domain.
		//Do not modify location header
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

// Debug functions
func ReplaceLocationHost(urlString string, rrr *ResponseRewriteRuleSet, useTLS bool) (string, error) {
	return replaceLocationHost(urlString, rrr, useTLS)
}
