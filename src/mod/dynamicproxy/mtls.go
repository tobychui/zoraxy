package dynamicproxy

/*
	mtls.go

	Per-host client certificate authentication (mutual TLS).

	Two layers cooperate:
	 1. The TLS handshake (GetConfigForClient) asks for a client certificate only on
	    hosts that enable mTLS, so every other host keeps behaving exactly as before.
	 2. The request handler re-checks the policy for the Host the request is actually
	    addressed to. The handshake alone is not enough: a client can connect using the
	    SNI of an open host and then send a Host header of a protected one, and HTTP/2
	    may reuse one connection for several hostnames.
*/

import (
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"

	"imuslab.com/zoraxy/mod/mtls"
)

// Headers describing the client certificate, always stripped from inbound requests so
// they cannot be spoofed by a client, then set only for a verified certificate.
var mtlsRequestHeaders = []string{
	"X-Client-Cert-Verified",
	"X-Client-Cert-Subject",
	"X-Client-Cert-CN",
	"X-Client-Cert-Serial",
	"X-Client-Cert-Fingerprint",
}

type mtlsPolicy struct {
	Mode       string
	CAs        []string
	BypassAuth bool
}

// getMTLSPolicy reads the mTLS settings of an endpoint. A nil endpoint, a missing
// manager or an unknown mode all resolve to "off".
func (router *Router) getMTLSPolicy(ep *ProxyEndpoint) mtlsPolicy {
	if ep == nil || router.Option.MTLSManager == nil {
		return mtlsPolicy{Mode: mtls.ModeOff}
	}
	router.tlsBehaviorMutex.RLock()
	defer router.tlsBehaviorMutex.RUnlock()
	if ep.TlsOptions == nil {
		return mtlsPolicy{Mode: mtls.ModeOff}
	}
	mode := ep.TlsOptions.MTLSMode
	if mode != mtls.ModeOptional && mode != mtls.ModeRequired {
		return mtlsPolicy{Mode: mtls.ModeOff}
	}
	return mtlsPolicy{
		Mode:       mode,
		CAs:        append([]string{}, ep.TlsOptions.MTLSCAs...),
		BypassAuth: ep.TlsOptions.MTLSBypassAuth,
	}
}

// newMTLSConfigForClient returns the GetConfigForClient hook for the TLS listener.
// Hosts without mTLS return nil so the base config (and its session resumption) is used.
func (router *Router) newMTLSConfigForClient(base *tls.Config) func(*tls.ClientHelloInfo) (*tls.Config, error) {
	var cache sync.Map //Per policy TLS config, keeps session ticket keys stable across handshakes
	return func(hello *tls.ClientHelloInfo) (*tls.Config, error) {
		if hello.ServerName == "" || router.Option.MTLSManager == nil {
			return nil, nil
		}
		policy := router.getMTLSPolicy(router.GetProxyEndpointFromHostname(hello.ServerName))
		if policy.Mode == mtls.ModeOff {
			return nil, nil
		}

		cas := append([]string{}, policy.CAs...)
		sort.Strings(cas)
		key := strconv.FormatUint(router.Option.MTLSManager.Generation(), 10) + "|" + policy.Mode + "|" + strings.Join(cas, "\n")
		if cached, ok := cache.Load(key); ok {
			return cached.(*tls.Config), nil
		}

		cfg := base.Clone()
		cfg.GetConfigForClient = nil
		cfg.ClientCAs = router.Option.MTLSManager.GetPool(policy.CAs)
		if policy.Mode == mtls.ModeRequired {
			cfg.ClientAuth = tls.RequireAndVerifyClientCert
		} else {
			cfg.ClientAuth = tls.VerifyClientCertIfGiven
		}
		cache.Store(key, cfg)
		return cfg, nil
	}
}

// checkMTLS enforces the endpoint's mTLS policy on a request. It returns handled=true if a
// response has already been written (the request must not continue), and verified=true if
// the client presented a certificate trusted by this host.
func (router *Router) checkMTLS(w http.ResponseWriter, r *http.Request, sep *ProxyEndpoint) (handled bool, verified bool, policy mtlsPolicy) {
	for _, header := range mtlsRequestHeaders {
		r.Header.Del(header)
	}

	policy = router.getMTLSPolicy(sep)
	if policy.Mode == mtls.ModeOff {
		return false, false, policy
	}

	var leaf *x509.Certificate
	if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		sniMatchesHost := router.GetProxyEndpointFromHostname(r.TLS.ServerName) == sep
		if sniMatchesHost && len(r.TLS.VerifiedChains) > 0 {
			//The handshake already verified it against this host's CA set
			leaf = r.TLS.PeerCertificates[0]
		} else {
			//Connection was negotiated for another hostname, verify against this host's CAs
			leaf = mtls.VerifyPeer(r.TLS, router.Option.MTLSManager.GetPool(policy.CAs))
		}
	}

	if leaf == nil {
		if policy.Mode == mtls.ModeRequired {
			status := http.StatusForbidden
			if r.TLS != nil && r.TLS.ServerName != "" && router.GetProxyEndpointFromHostname(r.TLS.ServerName) != sep {
				//Misdirected request: the client should retry on a new connection
				//that presents its certificate for this hostname
				status = http.StatusMisdirectedRequest
			}
			router.logRequest(r, false, status, "mtls-blocked", r.Host, "blocked", sep)
			http.Error(w, "A valid client certificate is required to access this site", status)
			return true, false, policy
		}
		return false, false, policy
	}

	r.Header.Set("X-Client-Cert-Verified", "true")
	r.Header.Set("X-Client-Cert-Subject", leaf.Subject.String())
	r.Header.Set("X-Client-Cert-CN", leaf.Subject.CommonName)
	r.Header.Set("X-Client-Cert-Serial", leaf.SerialNumber.Text(16))
	r.Header.Set("X-Client-Cert-Fingerprint", mtls.Fingerprint(leaf))
	return false, true, policy
}
