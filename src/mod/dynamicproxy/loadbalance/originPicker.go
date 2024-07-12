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
	var ret *Upstream
	sum := 0
	for _, c := range upstreams {
		sum += c.Weight
	}
	r, err := intRange(0, sum)
	if err != nil {
		return ret, -1, err
	}
	counter := 0
	for _, c := range upstreams {
		r -= c.Weight
		if r < 0 {
			return c, counter, nil
		}
		counter++
	}

	if ret == nil {
		//All fallback
		//use the first one that is with weight = 0
		fallbackUpstreams := []*Upstream{}
		fallbackUpstreamsOriginalID := []int{}
		for ix, upstream := range upstreams {
			if upstream.Weight == 0 {
				fallbackUpstreams = append(fallbackUpstreams, upstream)
				fallbackUpstreamsOriginalID = append(fallbackUpstreamsOriginalID, ix)
			}
		}
		upstreamID := rand.Intn(len(fallbackUpstreams))
		return fallbackUpstreams[upstreamID], fallbackUpstreamsOriginalID[upstreamID], nil
	}
	return ret, -1, errors.New("failed to pick an upstream origin server")
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
