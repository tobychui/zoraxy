package rewrite

/*
 rewrite.go

 This script handle the rewrite logic for custom headers
*/

import (
	"strconv"

	"imuslab.com/zoraxy/mod/dynamicproxy/permissionpolicy"
)

// SplitInboundOutboundHeaders split user defined headers into upstream and downstream headers
// return upstream header and downstream header key-value pairs
// if the header is expected to be deleted, the value will be set to empty string
func SplitUpDownStreamHeaders(rewriteOptions *HeaderRewriteOptions) ([][]string, [][]string) {
	if len(rewriteOptions.UserDefinedHeaders) == 0 && rewriteOptions.HSTSMaxAge == 0 && !rewriteOptions.EnablePermissionPolicyHeader {
		//Early return if there are no defined headers
		return [][]string{}, [][]string{}
	}

	//Use pre-allocation for faster performance
	//Downstream +2 for Permission Policy and HSTS
	upstreamHeaders := make([][]string, len(rewriteOptions.UserDefinedHeaders))
	downstreamHeaders := make([][]string, len(rewriteOptions.UserDefinedHeaders)+2)
	upstreamHeaderCounter := 0
	downstreamHeaderCounter := 0

	//Sort the headers into upstream or downstream
	for _, customHeader := range rewriteOptions.UserDefinedHeaders {
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
	if rewriteOptions.HSTSMaxAge > 0 {
		if rewriteOptions.HSTSIncludeSubdomains {
			//Endpoint listening domain includes wildcards.
			downstreamHeaders[downstreamHeaderCounter] = []string{"Strict-Transport-Security", "max-age=" + strconv.Itoa(int(rewriteOptions.HSTSMaxAge)) + "; includeSubdomains"}
		} else {
			downstreamHeaders[downstreamHeaderCounter] = []string{"Strict-Transport-Security", "max-age=" + strconv.Itoa(int(rewriteOptions.HSTSMaxAge))}
		}

		downstreamHeaderCounter++
	}

	//Check if the endpoint require Permission Policy
	if rewriteOptions.EnablePermissionPolicyHeader {
		var usingPermissionPolicy *permissionpolicy.PermissionsPolicy
		if rewriteOptions.PermissionPolicy != nil {
			//Custom permission policy
			usingPermissionPolicy = rewriteOptions.PermissionPolicy
		} else {
			//Permission policy is enabled but not customized. Use default
			usingPermissionPolicy = permissionpolicy.GetDefaultPermissionPolicy()
		}

		downstreamHeaders[downstreamHeaderCounter] = usingPermissionPolicy.ToKeyValueHeader()
		downstreamHeaderCounter++
	}

	return upstreamHeaders, downstreamHeaders
}
