package dynamicproxy

import (
	"encoding/json"
	"errors"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"imuslab.com/zoraxy/mod/dynamicproxy/loadbalance"
	"imuslab.com/zoraxy/mod/dynamicproxy/rewrite"
)

/*
	endpoint.go
	author: tobychui

	This script handle the proxy endpoint object actions
	so proxyEndpoint can be handled like a proper oop object

	Most of the functions are implemented in dynamicproxy.go
*/

/*
	User Defined Header Functions
*/

// Check if a user define header exists in this endpoint, ignore case
func (ep *ProxyEndpoint) UserDefinedHeaderExists(key string) bool {
	endpointProxyRewriteRules := GetDefaultHeaderRewriteRules()
	if ep.HeaderRewriteRules != nil {
		endpointProxyRewriteRules = ep.HeaderRewriteRules
	}

	for _, header := range endpointProxyRewriteRules.UserDefinedHeaders {
		if strings.EqualFold(header.Key, key) {
			return true
		}
	}
	return false
}

// Remvoe a user defined header from the list
func (ep *ProxyEndpoint) RemoveUserDefinedHeader(key string) error {
	newHeaderList := []*rewrite.UserDefinedHeader{}
	if ep.HeaderRewriteRules == nil {
		ep.HeaderRewriteRules = GetDefaultHeaderRewriteRules()
	}
	for _, header := range ep.HeaderRewriteRules.UserDefinedHeaders {
		if !strings.EqualFold(header.Key, key) {
			newHeaderList = append(newHeaderList, header)
		}
	}

	ep.HeaderRewriteRules.UserDefinedHeaders = newHeaderList

	return nil
}

// Add a user defined header to the list, duplicates will be automatically removed
func (ep *ProxyEndpoint) AddUserDefinedHeader(newHeaderRule *rewrite.UserDefinedHeader) error {
	if ep.UserDefinedHeaderExists(newHeaderRule.Key) {
		ep.RemoveUserDefinedHeader(newHeaderRule.Key)
	}

	if ep.HeaderRewriteRules == nil {
		ep.HeaderRewriteRules = GetDefaultHeaderRewriteRules()
	}
	newHeaderRule.Key = cases.Title(language.Und, cases.NoLower).String(newHeaderRule.Key)
	ep.HeaderRewriteRules.UserDefinedHeaders = append(ep.HeaderRewriteRules.UserDefinedHeaders, newHeaderRule)
	return nil
}

/*
	Virtual Directory Functions
*/

// Get virtual directory handler from given URI
func (ep *ProxyEndpoint) GetVirtualDirectoryHandlerFromRequestURI(requestURI string) *VirtualDirectoryEndpoint {
	for _, vdir := range ep.VirtualDirectories {
		if strings.HasPrefix(requestURI, vdir.MatchingPath) {
			thisVdir := vdir
			return thisVdir
		}
	}
	return nil
}

// Get virtual directory handler by matching path (exact match required)
func (ep *ProxyEndpoint) GetVirtualDirectoryRuleByMatchingPath(matchingPath string) *VirtualDirectoryEndpoint {
	for _, vdir := range ep.VirtualDirectories {
		if vdir.MatchingPath == matchingPath {
			thisVdir := vdir
			return thisVdir
		}
	}
	return nil
}

// Delete a vdir rule by its matching path
func (ep *ProxyEndpoint) RemoveVirtualDirectoryRuleByMatchingPath(matchingPath string) error {
	entryFound := false
	newVirtualDirectoryList := []*VirtualDirectoryEndpoint{}
	for _, vdir := range ep.VirtualDirectories {
		if vdir.MatchingPath == matchingPath {
			entryFound = true
		} else {
			newVirtualDirectoryList = append(newVirtualDirectoryList, vdir)
		}
	}

	if entryFound {
		//Update the list of vdirs
		ep.VirtualDirectories = newVirtualDirectoryList
		return nil
	}
	return errors.New("target virtual directory routing rule not found")
}

// Add a vdir rule by its matching path
func (ep *ProxyEndpoint) AddVirtualDirectoryRule(vdir *VirtualDirectoryEndpoint) (*ProxyEndpoint, error) {
	//Check for matching path duplicate
	if ep.GetVirtualDirectoryRuleByMatchingPath(vdir.MatchingPath) != nil {
		return nil, errors.New("rule with same matching path already exists")
	}

	//Append it to the list of virtual directory
	ep.VirtualDirectories = append(ep.VirtualDirectories, vdir)

	//Prepare to replace the current routing rule
	parentRouter := ep.parent
	readyRoutingRule, err := parentRouter.PrepareProxyRoute(ep)
	if err != nil {
		return nil, err
	}

	if ep.ProxyType == ProxyTypeRoot {
		parentRouter.Root = readyRoutingRule
	} else if ep.ProxyType == ProxyTypeHost {
		ep.Remove()
		parentRouter.AddProxyRouteToRuntime(readyRoutingRule)
	} else {
		return nil, errors.New("unsupported proxy type")
	}

	return readyRoutingRule, nil
}

/* Upstream related wrapper functions */
//Check if there already exists another upstream with identical origin
func (ep *ProxyEndpoint) UpstreamOriginExists(originURL string) bool {
	for _, origin := range ep.ActiveOrigins {
		if origin.OriginIpOrDomain == originURL {
			return true
		}
	}
	for _, origin := range ep.InactiveOrigins {
		if origin.OriginIpOrDomain == originURL {
			return true
		}
	}
	return false
}

// Get a upstream origin from given origin ip or domain
func (ep *ProxyEndpoint) GetUpstreamOriginByMatchingIP(originIpOrDomain string) (*loadbalance.Upstream, error) {
	for _, origin := range ep.ActiveOrigins {
		if origin.OriginIpOrDomain == originIpOrDomain {
			return origin, nil
		}
	}

	for _, origin := range ep.InactiveOrigins {
		if origin.OriginIpOrDomain == originIpOrDomain {
			return origin, nil
		}
	}
	return nil, errors.New("target upstream origin not found")
}

// Add upstream to endpoint and update it to runtime
func (ep *ProxyEndpoint) AddUpstreamOrigin(newOrigin *loadbalance.Upstream, activate bool) error {
	//Check if the upstream already exists
	if ep.UpstreamOriginExists(newOrigin.OriginIpOrDomain) {
		return errors.New("upstream with same origin already exists")
	}

	if activate {
		//Add it to the active origin list
		err := newOrigin.StartProxy()
		if err != nil {
			return err
		}
		ep.ActiveOrigins = append(ep.ActiveOrigins, newOrigin)
	} else {
		//Add to inactive origin list
		ep.InactiveOrigins = append(ep.InactiveOrigins, newOrigin)
	}

	ep.UpdateToRuntime()
	return nil
}

// Remove upstream from endpoint and update it to runtime
func (ep *ProxyEndpoint) RemoveUpstreamOrigin(originIpOrDomain string) error {
	//Just to make sure there are no spaces
	originIpOrDomain = strings.TrimSpace(originIpOrDomain)

	//Check if the upstream already been removed
	if !ep.UpstreamOriginExists(originIpOrDomain) {
		//Not exists in the first place
		return nil
	}

	newActiveOriginList := []*loadbalance.Upstream{}
	for _, origin := range ep.ActiveOrigins {
		if origin.OriginIpOrDomain != originIpOrDomain {
			newActiveOriginList = append(newActiveOriginList, origin)
		}
	}

	newInactiveOriginList := []*loadbalance.Upstream{}
	for _, origin := range ep.InactiveOrigins {
		if origin.OriginIpOrDomain != originIpOrDomain {
			newInactiveOriginList = append(newInactiveOriginList, origin)
		}
	}
	//Ok, set the origin list to the new one
	ep.ActiveOrigins = newActiveOriginList
	ep.InactiveOrigins = newInactiveOriginList
	ep.UpdateToRuntime()
	return nil
}

// Check if the proxy endpoint hostname or alias name contains subdomain wildcard
func (ep *ProxyEndpoint) ContainsWildcardName(skipAliasCheck bool) bool {
	hostname := ep.RootOrMatchingDomain
	aliasHostnames := ep.MatchingDomainAlias

	wildcardCheck := func(hostname string) bool {
		return len(hostname) > 0 && hostname[0] == '*'
	}

	if wildcardCheck(hostname) {
		return true
	}

	if !skipAliasCheck {
		for _, aliasHostname := range aliasHostnames {
			if wildcardCheck(aliasHostname) {
				return true
			}
		}
	}

	return false
}

// Create a deep clone object of the proxy endpoint
// Note the returned object is not activated. Call to prepare function before pushing into runtime
func (ep *ProxyEndpoint) Clone() *ProxyEndpoint {
	clonedProxyEndpoint := ProxyEndpoint{}
	js, _ := json.Marshal(ep)
	json.Unmarshal(js, &clonedProxyEndpoint)
	return &clonedProxyEndpoint
}

// Remove this proxy endpoint from running proxy endpoint list
func (ep *ProxyEndpoint) Remove() error {
	lookupHostname := strings.ToLower(ep.RootOrMatchingDomain)
	ep.parent.ProxyEndpoints.Delete(lookupHostname)
	return nil
}

// Check if the proxy endpoint is enabled
func (ep *ProxyEndpoint) IsEnabled() bool {
	return !ep.Disabled
}

// Write changes to runtime without respawning the proxy handler
// use prepare -> remove -> add if you change anything in the endpoint
// that effects the proxy routing src / dest
func (ep *ProxyEndpoint) UpdateToRuntime() {
	lookupHostname := strings.ToLower(ep.RootOrMatchingDomain)
	ep.parent.ProxyEndpoints.Store(lookupHostname, ep)
}
