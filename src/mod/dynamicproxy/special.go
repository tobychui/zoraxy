package dynamicproxy

import (
	"errors"
	"net/http"
)

/*
	Special.go

	This script handle special routing rules
	by external modules
*/

type RoutingRule struct {
	ID             string
	MatchRule      func(r *http.Request) bool
	RoutingHandler http.Handler
	Enabled        bool
}

//Router functions
//Check if a routing rule exists given its id
func (router *Router) GetRoutingRuleById(rrid string) (*RoutingRule, error) {
	for _, rr := range router.routingRules {
		if rr.ID == rrid {
			return rr, nil
		}
	}

	return nil, errors.New("routing rule with given id not found")
}

//Add a routing rule to the router
func (router *Router) AddRoutingRules(rr *RoutingRule) error {
	_, err := router.GetRoutingRuleById(rr.ID)
	if err != nil {
		//routing rule with given id already exists
		return err
	}

	router.routingRules = append(router.routingRules, rr)
	return nil
}

//Remove a routing rule from the router
func (router *Router) RemoveRoutingRule(rrid string) {
	newRoutingRules := []*RoutingRule{}
	for _, rr := range router.routingRules {
		if rr.ID != rrid {
			newRoutingRules = append(newRoutingRules, rr)
		}
	}

	router.routingRules = newRoutingRules
}

//Get all routing rules
func (router *Router) GetAllRoutingRules() []*RoutingRule {
	return router.routingRules
}

//Get the matching routing rule that describe this request.
//Return nil if no routing rule is match
func (router *Router) GetMatchingRoutingRule(r *http.Request) *RoutingRule {
	for _, thisRr := range router.routingRules {
		if thisRr.IsMatch(r) {
			return thisRr
		}
	}
	return nil
}

//Routing Rule functions
//Check if a request object match the
func (e *RoutingRule) IsMatch(r *http.Request) bool {
	if !e.Enabled {
		return false
	}
	return e.MatchRule(r)
}

func (e *RoutingRule) Route(w http.ResponseWriter, r *http.Request) {
	e.RoutingHandler.ServeHTTP(w, r)
}
