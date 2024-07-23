package loadbalance

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"net/http"
)

/*
	Origin Picker

	This script contains the code to pick the best origin
	by this request.
*/

// GetRequestUpstreamTarget return the upstream target where this
// request should be routed
func (m *RouteManager) GetRequestUpstreamTarget(w http.ResponseWriter, r *http.Request, origins []*Upstream, useStickySession bool) (*Upstream, error) {
	if len(origins) == 0 {
		return nil, errors.New("no upstream is defined for this host")
	}
	var targetOrigin = origins[0]
	if useStickySession {
		//Use stick session, check which origins this request previously used
		targetOriginId, err := m.getSessionHandler(r, origins)
		if err != nil {
			//No valid session found. Assign a new upstream
			targetOrigin, index, err := getRandomUpstreamByWeight(origins)
			if err != nil {
				fmt.Println("Oops. Unable to get random upstream")
				targetOrigin = origins[0]
				index = 0
			}
			m.setSessionHandler(w, r, targetOrigin.OriginIpOrDomain, index)
			return targetOrigin, nil
		}

		//Valid session found. Resume the previous session
		return origins[targetOriginId], nil
	} else {
		//Do not use stick session. Get a random one
		var err error
		targetOrigin, _, err = getRandomUpstreamByWeight(origins)
		if err != nil {
			log.Println(err)
			targetOrigin = origins[0]
		}

	}

	//fmt.Println("DEBUG: Picking origin " + targetOrigin.OriginIpOrDomain)
	return targetOrigin, nil
}

/* Features related to session access */
//Set a new origin for this connection by session
func (m *RouteManager) setSessionHandler(w http.ResponseWriter, r *http.Request, originIpOrDomain string, index int) error {
	session, err := m.SessionStore.Get(r, "STICKYSESSION")
	if err != nil {
		return err
	}
	session.Values["zr_sid_origin"] = originIpOrDomain
	session.Values["zr_sid_index"] = index
	session.Options.MaxAge = 86400 //1 day
	session.Options.Path = "/"
	err = session.Save(r, w)
	if err != nil {
		return err
	}
	return nil
}

// Get the previous connected origin from session
func (m *RouteManager) getSessionHandler(r *http.Request, upstreams []*Upstream) (int, error) {
	// Get existing session
	session, err := m.SessionStore.Get(r, "STICKYSESSION")
	if err != nil {
		return -1, err
	}

	// Retrieve session values for origin
	originDomainRaw := session.Values["zr_sid_origin"]
	originIDRaw := session.Values["zr_sid_index"]

	if originDomainRaw == nil || originIDRaw == nil {
		return -1, errors.New("no session has been set")
	}
	originDomain := originDomainRaw.(string)
	originID := originIDRaw.(int)

	//Check if it has been modified
	if len(upstreams) < originID || upstreams[originID].OriginIpOrDomain != originDomain {
		//Mismatch or upstreams has been updated
		return -1, errors.New("upstreams has been changed")
	}

	return originID, nil
}

/* Functions related to random upstream picking */
// Get a random upstream by the weights defined in Upstream struct, return the upstream, index value and any error
func getRandomUpstreamByWeight(upstreams []*Upstream) (*Upstream, int, error) {
	// If there is only one upstream, return it
	if len(upstreams) == 1 {
		return upstreams[0], 0, nil
	}

	// Calculate total weight for upstreams with weight > 0
	totalWeight := 0
	fallbackUpstreams := make([]*Upstream, 0)

	for _, upstream := range upstreams {
		if upstream.Weight > 0 {
			totalWeight += upstream.Weight
		} else {
			fallbackUpstreams = append(fallbackUpstreams, upstream) // Collect fallback upstreams
		}
	}

	// If there are no upstreams with weight > 0, return a fallback upstream if available
	if totalWeight == 0 {
		if len(fallbackUpstreams) > 0 {
			// Randomly select one of the fallback upstreams
			index := rand.Intn(len(fallbackUpstreams))
			return fallbackUpstreams[index], index, nil
		}
		// No upstreams available at all
		return nil, -1, errors.New("no valid upstream servers available")
	}

	// Random weight between 0 and total weight
	randomWeight := rand.Intn(totalWeight)

	// Select an upstream based on the random weight
	for i, upstream := range upstreams {
		if upstream.Weight > 0 { // Only consider upstreams with weight > 0
			if randomWeight < upstream.Weight {
				return upstream, i, nil // Return the selected upstream and its index
			}
			randomWeight -= upstream.Weight
		}
	}

	// If we reach here, it means we should return a fallback upstream if available
	if len(fallbackUpstreams) > 0 {
		index := rand.Intn(len(fallbackUpstreams))
		return fallbackUpstreams[index], index, nil
	}

	return nil, -1, errors.New("failed to pick an upstream origin server")
}

// IntRange returns a random integer in the range from min to max.
func intRange(min, max int) (int, error) {
	var result int
	switch {
	case min > max:
		// Fail with error
		return result, errors.New("min is greater than max")
	case max == min:
		result = max
	case max > min:
		b := rand.Intn(max-min) + min
		result = min + int(b)
	}
	return result, nil
}
