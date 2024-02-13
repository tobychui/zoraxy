package dynamicproxy

/*
	ProxyEndpoint.go
	author: tobychui

	This script handle the proxy endpoint object actions
	so proxyEndpoint can be handled like a proper oop object

	Most of the functions are implemented in dynamicproxy.go
*/

// Update change in the current running proxy endpoint config
func (ep *ProxyEndpoint) UpdateToRuntime() {
	ep.parent.ProxyEndpoints.Store(ep.RootOrMatchingDomain, ep)
}

// Remove this proxy endpoint from running proxy endpoint list
func (ep *ProxyEndpoint) Remove() error {
	ep.parent.ProxyEndpoints.Delete(ep.RootOrMatchingDomain)
	return nil
}

// ProxyEndpoint remove provide global access by key
func (router *Router) RemoveProxyEndpointByRootname(rootnameOrMatchingDomain string) error {
	targetEpt, err := router.LoadProxy(rootnameOrMatchingDomain)
	if err != nil {
		return err
	}

	return targetEpt.Remove()
}
