package main

import (
	"log"
	"net/http"

	"imuslab.com/zoraxy/mod/dynamicproxy"
)

/*
	acme.go

	This script handle special routing required for acme auto cert renew functions
*/

func acmeRegisterSpecialRoutingRule() {
	err := dynamicProxyRouter.AddRoutingRules(&dynamicproxy.RoutingRule{
		ID: "acme-autorenew",
		MatchRule: func(r *http.Request) bool {
			if r.RequestURI == "/.well-known/" {
				return true
			}

			return false
		},
		RoutingHandler: func(w http.ResponseWriter, r *http.Request) {
			w.Write([]byte("HELLO WORLD, THIS IS ACME REQUEST HANDLER"))
		},
		Enabled: true,
	})

	if err != nil {
		log.Println("[Err] " + err.Error())
	}
}
