package dynamicproxy

import "errors"

/*
	ProxyEndpoint.go
	author: tobychui

	This script handle the proxy endpoint object actions
	so proxyEndpoint can be handled like a proper oop object

	Most of the functions are implemented in dynamicproxy.go
*/

//Get the string version of proxy type
func (ep *ProxyEndpoint) GetProxyTypeString() string {
	if ep.ProxyType == ProxyType_Subdomain {
		return "subd"
	} else if ep.ProxyType == ProxyType_Vdir {
		return "vdir"
	}

	return "unknown"
}

//Update change in the current running proxy endpoint config
func (ep *ProxyEndpoint) UpdateToRuntime() {
	if ep.IsVdir() {
		ep.parent.ProxyEndpoints.Store(ep.RootOrMatchingDomain, ep)

	} else if ep.IsSubDomain() {
		ep.parent.SubdomainEndpoint.Store(ep.RootOrMatchingDomain, ep)
	}
}

//Return true if the endpoint type is virtual directory
func (ep *ProxyEndpoint) IsVdir() bool {
	return ep.ProxyType == ProxyType_Vdir
}

//Return true if the endpoint type is subdomain
func (ep *ProxyEndpoint) IsSubDomain() bool {
	return ep.ProxyType == ProxyType_Subdomain
}

//Remove this proxy endpoint from running proxy endpoint list
func (ep *ProxyEndpoint) Remove() error {
	//fmt.Println(ptype, key)
	if ep.IsVdir() {
		ep.parent.ProxyEndpoints.Delete(ep.RootOrMatchingDomain)
		return nil
	} else if ep.IsSubDomain() {
		ep.parent.SubdomainEndpoint.Delete(ep.RootOrMatchingDomain)
		return nil
	}
	return errors.New("invalid or unsupported type")

}

//ProxyEndpoint remove provide global access by key
func (router *Router) RemoveProxyEndpointByRootname(proxyType string, rootnameOrMatchingDomain string) error {
	targetEpt, err := router.LoadProxy(proxyType, rootnameOrMatchingDomain)
	if err != nil {
		return err
	}

	return targetEpt.Remove()
}
