package dynamicproxy

import (
	"encoding/json"
	"errors"
	"strings"

	"golang.org/x/text/cases"
	"golang.org/x/text/language"
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
	for _, header := range ep.UserDefinedHeaders {
		if strings.EqualFold(header.Key, key) {
			return true
		}
	}
	return false
}

// Remvoe a user defined header from the list
func (ep *ProxyEndpoint) RemoveUserDefinedHeader(key string) error {
	newHeaderList := []*UserDefinedHeader{}
	for _, header := range ep.UserDefinedHeaders {
		if !strings.EqualFold(header.Key, key) {
			newHeaderList = append(newHeaderList, header)
		}
	}

	ep.UserDefinedHeaders = newHeaderList

	return nil
}

// Add a user defined header to the list, duplicates will be automatically removed
func (ep *ProxyEndpoint) AddUserDefinedHeader(newHeaderRule *UserDefinedHeader) error {
	if ep.UserDefinedHeaderExists(newHeaderRule.Key) {
		ep.RemoveUserDefinedHeader(newHeaderRule.Key)
	}

	newHeaderRule.Key = cases.Title(language.Und, cases.NoLower).String(newHeaderRule.Key)
	ep.UserDefinedHeaders = append(ep.UserDefinedHeaders, newHeaderRule)
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

// Delete a vdir rule by its matching path
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

	if ep.ProxyType == ProxyType_Root {
		parentRouter.Root = readyRoutingRule
	} else if ep.ProxyType == ProxyType_Host {
		ep.Remove()
		parentRouter.AddProxyRouteToRuntime(readyRoutingRule)
	} else {
		return nil, errors.New("unsupported proxy type")
	}

	return readyRoutingRule, nil
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
	ep.parent.ProxyEndpoints.Delete(ep.RootOrMatchingDomain)
	return nil
}

// Write changes to runtime without respawning the proxy handler
// use prepare -> remove -> add if you change anything in the endpoint
// that effects the proxy routing src / dest
func (ep *ProxyEndpoint) UpdateToRuntime() {
	ep.parent.ProxyEndpoints.Store(ep.RootOrMatchingDomain, ep)
}
