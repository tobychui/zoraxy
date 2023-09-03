package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"regexp"
	"strconv"
	"time"

	"imuslab.com/zoraxy/mod/acme"
	"imuslab.com/zoraxy/mod/dynamicproxy"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	acme.go

	This script handle special routing required for acme auto cert renew functions
*/

// Helper function to generate a random port above a specified value
func getRandomPort(minPort int) int {
	return rand.Intn(65535-minPort) + minPort
}

// init the new ACME instance
func initACME() *acme.ACMEHandler {
	log.Println("Starting ACME handler")
	rand.Seed(time.Now().UnixNano())
	// Generate a random port above 30000
	port := getRandomPort(30000)

	// Check if the port is already in use
	for acme.IsPortInUse(port) {
		port = getRandomPort(30000)
	}

	return acme.NewACME("https://acme-staging-v02.api.letsencrypt.org/directory", strconv.Itoa(port), "./conf/acme_auth.json")
}

// create the special routing rule for ACME
func acmeRegisterSpecialRoutingRule() {
	log.Println("Assigned temporary port:" + acmeHandler.Getport())

	err := dynamicProxyRouter.AddRoutingRules(&dynamicproxy.RoutingRule{
		ID: "acme-autorenew",
		MatchRule: func(r *http.Request) bool {
			found, _ := regexp.MatchString("/.well-known/acme-challenge/*", r.RequestURI)
			return found
		},
		RoutingHandler: func(w http.ResponseWriter, r *http.Request) {

			req, err := http.NewRequest(http.MethodGet, "http://localhost:"+acmeHandler.Getport()+r.RequestURI, nil)
			req.Host = r.Host
			if err != nil {
				fmt.Printf("client: could not create request: %s\n", err)
				return
			}
			res, err := http.DefaultClient.Do(req)
			if err != nil {
				fmt.Printf("client: error making http request: %s\n", err)
				return
			}

			resBody, err := ioutil.ReadAll(res.Body)
			defer res.Body.Close()
			if err != nil {
				fmt.Printf("error reading: %s\n", err)
				return
			}
			w.Write(resBody)
		},
		Enabled:                true,
		UseSystemAccessControl: false,
	})

	if err != nil {
		log.Println("[Err] " + err.Error())
	}
}

// This function check if the renew setup is satisfied. If not, toggle them automatically
func AcmeCheckAndHandleRenewCertificate(w http.ResponseWriter, r *http.Request) {
	isForceHttpsRedirectEnabledOriginally := false
	if dynamicProxyRouter.Option.Port == 443 {
		//Enable port 80 to 443 redirect
		if !dynamicProxyRouter.Option.ForceHttpsRedirect {
			log.Println("Temporary enabling HTTP to HTTPS redirect for ACME certificate renew requests")
			dynamicProxyRouter.UpdateHttpToHttpsRedirectSetting(true)
		} else {
			//Set this to true, so after renew, do not turn it off
			isForceHttpsRedirectEnabledOriginally = true
		}

	} else if dynamicProxyRouter.Option.Port == 80 {
		//Go ahead

	} else {
		//This port do not support ACME
		utils.SendErrorResponse(w, "ACME renew only support web server listening on port 80 (http) or 443 (https)")
	}

	// Pass over to the acmeHandler to deal with the communication
	acmeHandler.HandleRenewCertificate(w, r)

	if dynamicProxyRouter.Option.Port == 443 {
		if !isForceHttpsRedirectEnabledOriginally {
			//Default is off. Turn the redirection off
			log.Println("Restoring HTTP to HTTPS redirect settings")
			dynamicProxyRouter.UpdateHttpToHttpsRedirectSetting(false)
		}
	}
}
