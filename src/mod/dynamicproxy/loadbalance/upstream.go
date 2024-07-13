package loadbalance

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
)

// StartProxy create and start a HTTP proxy using dpcore
// Example of webProxyEndpoint: https://example.com:443 or http://192.168.1.100:8080
func (u *Upstream) StartProxy() error {
	//Filter the tailing slash if any
	domain := u.OriginIpOrDomain
	if len(domain) == 0 {
		return errors.New("invalid endpoint config")
	}
	if domain[len(domain)-1:] == "/" {
		domain = domain[:len(domain)-1]
	}

	if !strings.HasPrefix("http://", domain) && !strings.HasPrefix("https://", domain) {
		//TLS is not hardcoded in proxy target domain
		if u.RequireTLS {
			domain = "https://" + domain
		} else {
			domain = "http://" + domain
		}
	}

	//Create a new proxy agent for this upstream
	path, err := url.Parse(domain)
	if err != nil {
		return err
	}

	proxy := dpcore.NewDynamicProxyCore(path, "", &dpcore.DpcoreOptions{
		IgnoreTLSVerification: u.SkipCertValidations,
		FlushInterval:         100 * time.Millisecond,
	})

	u.proxy = proxy
	return nil
}

// IsReady return the proxy ready state of the upstream server
// Return false if StartProxy() is not called on this upstream before
func (u *Upstream) IsReady() bool {
	return u.proxy != nil
}

// Clone return a new deep copy object of the identical upstream
func (u *Upstream) Clone() *Upstream {
	newUpstream := Upstream{}
	js, _ := json.Marshal(u)
	json.Unmarshal(js, &newUpstream)
	return &newUpstream
}

// ServeHTTP uses this upstream proxy router to route the current request
func (u *Upstream) ServeHTTP(w http.ResponseWriter, r *http.Request, rrr *dpcore.ResponseRewriteRuleSet) error {
	//Auto rewrite to upstream origin if not set
	if rrr.ProxyDomain == "" {
		rrr.ProxyDomain = u.OriginIpOrDomain
	}

	return u.proxy.ServeHTTP(w, r, rrr)
}

// String return the string representations of endpoints in this upstream
func (u *Upstream) String() string {
	return u.OriginIpOrDomain
}
