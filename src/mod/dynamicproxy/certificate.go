package dynamicproxy

import (
	"encoding/json"
	"errors"
	"fmt"

	"imuslab.com/zoraxy/mod/tlscert"
)

func (router *Router) ResolveHostSpecificTlsBehaviorForHostname(hostname string) (*tlscert.HostSpecificTlsBehavior, error) {
	if hostname == "" {
		return nil, errors.New("hostname cannot be empty")
	}

	ept := router.GetProxyEndpointFromHostname(hostname)
	if ept == nil {
		return tlscert.GetDefaultHostSpecificTlsBehavior(), nil
	}

	// Check if the endpoint has a specific TLS behavior
	if ept.TlsOptions != nil {
		imported := &tlscert.HostSpecificTlsBehavior{}
		router.tlsBehaviorMutex.RLock()
		// Deep copy the TlsOptions using JSON marshal/unmarshal
		data, err := json.Marshal(ept.TlsOptions)
		if err != nil {
			router.tlsBehaviorMutex.RUnlock()
			return nil, fmt.Errorf("failed to deepcopy TlsOptions: %w", err)
		}
		router.tlsBehaviorMutex.RUnlock()
		if err := json.Unmarshal(data, imported); err != nil {
			return nil, fmt.Errorf("failed to deepcopy TlsOptions: %w", err)
		}
		return imported, nil
	}

	return tlscert.GetDefaultHostSpecificTlsBehavior(), nil
}

func (router *Router) SetPreferredCertificateForDomain(ept *ProxyEndpoint, domain string, certName string) error {
	if ept == nil || certName == "" {
		return errors.New("endpoint and certificate name cannot be empty")
	}

	// Set the preferred certificate for the endpoint
	if ept.TlsOptions == nil {
		ept.TlsOptions = tlscert.GetDefaultHostSpecificTlsBehavior()
	}

	router.tlsBehaviorMutex.Lock()
	if ept.TlsOptions.PreferredCertificate == nil {
		ept.TlsOptions.PreferredCertificate = make(map[string]string)
	}
	ept.TlsOptions.PreferredCertificate[domain] = certName
	router.tlsBehaviorMutex.Unlock()

	return nil
}
