package loadbalance

import (
	"errors"
	"fmt"
	"net/http"
)

/*
	Origin Picker

	This script contains the code to pick the best origin
	by this request.
*/

// GetRequestUpstreamTarget return the upstream target where this
// request should be routed
func (m *RouteManager) GetRequestUpstreamTarget(r *http.Request, origins []*Upstream) (*Upstream, error) {
	if len(origins) == 0 {
		return nil, errors.New("no upstream is defined for this host")
	}

	//TODO: Add upstream picking algorithm here
	fmt.Println("DEBUG: Picking origin " + origins[0].OriginIpOrDomain)
	return origins[0], nil
}
