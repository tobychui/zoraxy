package loadbalance

import (
	"errors"
	"math/rand"
	"net/http"
)

/*
	Origin Picker

	This script contains the code to pick the best origin
	by this request.
*/

const (
	STICKY_SESSION_NAME = "zr_sticky_session"
)

// GetRequestUpstreamTarget return the upstream target where this
// request should be routed
func (m *RouteManager) GetRequestUpstreamTarget(w http.ResponseWriter, r *http.Request, origins []*Upstream, useStickySession bool, disableAutoFallback bool) (*Upstream, error) {
	if len(origins) == 0 {
		return nil, errors.New("no upstream is defined for this host")
	}

	//Pick the origin
	if useStickySession {
		//Use stick session, check which origins this request previously used
		targetOriginId, err := m.getSessionHandler(r, origins)
		if err != nil {
			// No valid session found or origin is offline
			// Filter the offline origins (but only if there's more than 1 upstream and auto-fallback is not disabled)
			originalUpstreamCount := len(origins)
			origins = m.FilterOfflineOrigins(origins, originalUpstreamCount, disableAutoFallback)
			if len(origins) == 0 {
				return nil, errors.New("no online upstream is available for origin: " + r.Host)
			}

			//Get a random origin
			targetOrigin, index, err := getRandomUpstreamByWeight(origins)
			if err != nil {
				m.println("Unable to get random upstream", err)
				targetOrigin = origins[0]
				index = 0
			}

			//fmt.Println("DEBUG: (Sticky Session) Registering session origin " + origins[index].OriginIpOrDomain)
			m.setSessionHandler(w, r, targetOrigin.OriginIpOrDomain, index)
			return targetOrigin, nil
		}

		//Valid session found and origin is online
		//fmt.Println("DEBUG: (Sticky Session) Picking origin " + origins[targetOriginId].OriginIpOrDomain)
		return origins[targetOriginId], nil
	}

	//No sticky session, get a random origin
	//Filter the offline origins (but only if there's more than 1 upstream and auto-fallback is not disabled)
	originalUpstreamCount := len(origins)
	origins = m.FilterOfflineOrigins(origins, originalUpstreamCount, disableAutoFallback)
	if len(origins) == 0 {
		return nil, errors.New("no online upstream is available for origin: " + r.Host)
	}

	//Get a random origin
	targetOrigin, _, err := getRandomUpstreamByWeight(origins)
	if err != nil {
		m.println("Failed to get next origin", err)
		targetOrigin = origins[0]
	}

	//fmt.Println("DEBUG: Picking origin " + targetOrigin.OriginIpOrDomain)
	return targetOrigin, nil
}

// GetUsableUpstreamCounts return the number of usable upstreams
func (m *RouteManager) GetUsableUpstreamCounts(origins []*Upstream, disableAutoFallback bool) int {
	originalUpstreamCount := len(origins)
	origins = m.FilterOfflineOrigins(origins, originalUpstreamCount, disableAutoFallback)
	return len(origins)
}

/* Features related to session access */
//Set a new origin for this connection by session
func (m *RouteManager) setSessionHandler(w http.ResponseWriter, r *http.Request, originIpOrDomain string, index int) error {
	session, err := m.SessionStore.Get(r, STICKY_SESSION_NAME)
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
	session, err := m.SessionStore.Get(r, STICKY_SESSION_NAME)
	if err != nil {
		return -1, err
	}

	// Retrieve session values for origin
	originDomainRaw := session.Values["zr_sid_origin"]
	originIDRaw := session.Values["zr_sid_index"]

	if originDomainRaw == nil || originIDRaw == nil || originIDRaw == -1 {
		return -1, errors.New("no session has been set")
	}
	originDomain := originDomainRaw.(string)
	//originID := originIDRaw.(int)

	//Check if the upstream still exists
	for i, upstream := range upstreams {
		if upstream.OriginIpOrDomain == originDomain {
			if !m.IsTargetOnline(originDomain) {
				//Origin is offline
				return -1, errors.New("origin is offline")
			}

			//Ok, the origin is still online
			return i, nil
		}
	}

	return -1, errors.New("origin is no longer exists")
}

/* Functions related to random upstream picking */
// Get a random upstream by the weights defined in Upstream struct, return the upstream, index value and any error
func getRandomUpstreamByWeight(upstreams []*Upstream) (*Upstream, int, error) {
	// If there is only one upstream, return it
	if len(upstreams) == 1 {
		return upstreams[0], 0, nil
	}

	// Preserve the index with upstreams
	type upstreamWithIndex struct {
		Upstream *Upstream
		Index    int
	}

	// Calculate total weight for upstreams with weight > 0
	totalWeight := 0
	fallbackUpstreams := make([]upstreamWithIndex, 0, len(upstreams))

	for index, upstream := range upstreams {
		if upstream.Weight > 0 {
			totalWeight += upstream.Weight
		} else {
			// Collect fallback upstreams
			fallbackUpstreams = append(fallbackUpstreams, upstreamWithIndex{upstream, index})
		}
	}

	// If there are no upstreams with weight > 0, return a fallback upstream if available
	if totalWeight == 0 {
		if len(fallbackUpstreams) > 0 {
			// Randomly select one of the fallback upstreams
			randIndex := rand.Intn(len(fallbackUpstreams))
			return fallbackUpstreams[randIndex].Upstream, fallbackUpstreams[randIndex].Index, nil
		}
		// No upstreams available at all
		return nil, -1, errors.New("no valid upstream servers available")
	}

	// Random weight between 0 and total weight
	randomWeight := rand.Intn(totalWeight)

	// Select an upstream based on the random weight
	for index, upstream := range upstreams {
		if upstream.Weight > 0 { // Only consider upstreams with weight > 0
			if randomWeight < upstream.Weight {
				// Return the selected upstream and its index
				return upstream, index, nil
			}
			randomWeight -= upstream.Weight
		}
	}

	// If we reach here, it means we should return a fallback upstream if available
	if len(fallbackUpstreams) > 0 {
		randIndex := rand.Intn(len(fallbackUpstreams))
		return fallbackUpstreams[randIndex].Upstream, fallbackUpstreams[randIndex].Index, nil
	}

	return nil, -1, errors.New("failed to pick an upstream origin server")
}
