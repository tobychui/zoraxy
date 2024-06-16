package dynamicproxy

import (
	"strconv"

	"imuslab.com/zoraxy/mod/dynamicproxy/permissionpolicy"
)

/*
	CustomHeader.go

	This script handle parsing and injecting custom headers
	into the dpcore routing logic
*/

// SplitInboundOutboundHeaders split user defined headers into upstream and downstream headers
// return upstream header and downstream header key-value pairs
// if the header is expected to be deleted, the value will be set to empty string
func (ept *ProxyEndpoint) SplitInboundOutboundHeaders() ([][]string, [][]string) {
	if len(ept.UserDefinedHeaders) == 0 {
		//Early return if there are no defined headers
		return [][]string{}, [][]string{}
	}

	//Use pre-allocation for faster performance
	//Downstream +2 for Permission Policy and HSTS
	upstreamHeaders := make([][]string, len(ept.UserDefinedHeaders))
	downstreamHeaders := make([][]string, len(ept.UserDefinedHeaders)+2)
	upstreamHeaderCounter := 0
	downstreamHeaderCounter := 0

	//Sort the headers into upstream or downstream
	for _, customHeader := range ept.UserDefinedHeaders {
		thisHeaderSet := make([]string, 2)
		thisHeaderSet[0] = customHeader.Key
		thisHeaderSet[1] = customHeader.Value
		if customHeader.IsRemove {
			//Prevent invalid config
			thisHeaderSet[1] = ""
		}

		//Assign to slice
		if customHeader.Direction == HeaderDirection_ZoraxyToUpstream {
			upstreamHeaders[upstreamHeaderCounter] = thisHeaderSet
			upstreamHeaderCounter++
		} else if customHeader.Direction == HeaderDirection_ZoraxyToDownstream {
			downstreamHeaders[downstreamHeaderCounter] = thisHeaderSet
			downstreamHeaderCounter++
		}
	}

	//Check if the endpoint require HSTS headers
	if ept.HSTSMaxAge > 0 {
		downstreamHeaders[downstreamHeaderCounter] = []string{"Strict-Transport-Security", "max-age=" + strconv.Itoa(int(ept.HSTSMaxAge))}
		downstreamHeaderCounter++
	}

	//Check if the endpoint require Permission Policy
	if ept.EnablePermissionPolicyHeader {
		var usingPermissionPolicy *permissionpolicy.PermissionsPolicy
		if ept.PermissionPolicy != nil {
			//Custom permission policy
			usingPermissionPolicy = ept.PermissionPolicy
		} else {
			//Permission policy is enabled but not customized. Use default
			usingPermissionPolicy = permissionpolicy.GetDefaultPermissionPolicy()
		}

		downstreamHeaders[downstreamHeaderCounter] = usingPermissionPolicy.ToKeyValueHeader()
		downstreamHeaderCounter++
	}

	return upstreamHeaders, downstreamHeaders
}
