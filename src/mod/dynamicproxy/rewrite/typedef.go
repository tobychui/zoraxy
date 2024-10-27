package rewrite

import (
	"encoding/json"

	"imuslab.com/zoraxy/mod/dynamicproxy/permissionpolicy"
)

/*
	typdef.go

	This script handle the type definition for custom headers
*/

/* Custom Header Related Data structure */
// Header injection direction type
type HeaderDirection int

const (
	HeaderDirection_ZoraxyToUpstream   HeaderDirection = 0 //Inject (or remove) header to request out-going from Zoraxy to backend server
	HeaderDirection_ZoraxyToDownstream HeaderDirection = 1 //Inject (or remove) header to request out-going from Zoraxy to client (e.g. browser)
)

// User defined headers to add into a proxy endpoint
type UserDefinedHeader struct {
	Direction HeaderDirection
	Key       string
	Value     string
	IsRemove  bool //Instead of set, remove this key instead
}

type HeaderRewriteOptions struct {
	UserDefinedHeaders           []*UserDefinedHeader                //Custom headers to append when proxying requests from this endpoint
	HSTSMaxAge                   int64                               //HSTS max age, set to 0 for disable HSTS headers
	HSTSIncludeSubdomains        bool                                //Include subdomains in HSTS header
	EnablePermissionPolicyHeader bool                                //Enable injection of permission policy header
	PermissionPolicy             *permissionpolicy.PermissionsPolicy //Permission policy header
}

// Utilities for header rewrite
func (h *UserDefinedHeader) GetDirection() HeaderDirection {
	return h.Direction
}

// Copy eturns a deep copy of the UserDefinedHeader
func (h *UserDefinedHeader) Copy() *UserDefinedHeader {
	result := UserDefinedHeader{}
	js, _ := json.Marshal(h)
	json.Unmarshal(js, &result)
	return &result
}
