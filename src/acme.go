package main

import (
	"encoding/json"
	"fmt"
	"io"
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
	SystemWideLogger.Println("Starting ACME handler")
	rand.Seed(time.Now().UnixNano())
	// Generate a random port above 30000
	port := getRandomPort(30000)

	// Check if the port is already in use
	for acme.IsPortInUse(port) {
		port = getRandomPort(30000)
	}

	return acme.NewACME(strconv.Itoa(port), sysdb, SystemWideLogger, *acmeTestMode)
}

// Restart ACME handler and auto renewer
func restartACMEHandler() {
	SystemWideLogger.Println("Restarting ACME handler")
	//Clos the current handler and auto renewer
	acmeHandler.Close()
	acmeAutoRenewer.Close()
	acmeDeregisterSpecialRoutingRule()

	//Reinit the handler with a new random port
	acmeHandler = initACME()

	acmeRegisterSpecialRoutingRule()
}

// create the special routing rule for ACME
func acmeRegisterSpecialRoutingRule() {
	SystemWideLogger.Println("Assigned temporary port:" + acmeHandler.Getport())

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

			resBody, err := io.ReadAll(res.Body)
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
		SystemWideLogger.PrintAndLog("ACME", "Unable register temp port for DNS resolver", err)
	}
}

// remove the special routing rule for ACME
func acmeDeregisterSpecialRoutingRule() {
	SystemWideLogger.Println("Removing ACME routing rule")
	dynamicProxyRouter.RemoveRoutingRule("acme-autorenew")
}

// This function check if the renew setup is satisfied. If not, toggle them automatically
func AcmeCheckAndHandleRenewCertificate(w http.ResponseWriter, r *http.Request) {
	requireRestoreHttpsRedirect := false
	requireRestorePort80 := false
	dnsPara, _ := utils.PostBool(r, "dns")
	if !dnsPara {
		//HTTP-01 challenge
		switch dynamicProxyRouter.Option.Port {
		case 443:
			//Check if port 80 is enabled
			if !dynamicProxyRouter.Option.ListenOnPort80 {
				//Enable port 80 temporarily
				SystemWideLogger.PrintAndLog("ACME", "Temporarily enabling port 80 listener to handle ACME request ", nil)
				dynamicProxyRouter.UpdatePort80ListenerState(true)
				requireRestorePort80 = true
				time.Sleep(2 * time.Second)
			}

			//Enable port 80 to 443 redirect
			if !dynamicProxyRouter.Option.ForceHttpsRedirect {
				SystemWideLogger.Println("Temporary enabling HTTP to HTTPS redirect for ACME certificate renew requests")
				dynamicProxyRouter.UpdateHttpToHttpsRedirectSetting(true)
				//Mark that we need to restore this setting after renewal
				requireRestoreHttpsRedirect = true
			}

		case 80:
			//Go ahead

		default:
			//This port do not support ACME
			utils.SendErrorResponse(w, "ACME renew only support web server listening on port 80 (http) or 443 (https)")
			return
		}
	}

	//Add a 2 second delay to make sure everything is settle down
	time.Sleep(2 * time.Second)

	// Pass over to the acmeHandler to deal with the communication
	acmeHandler.HandleRenewCertificate(w, r)

	//Update the TLS cert store buffer
	tlsCertManager.UpdateLoadedCertList()

	//Restore original settings only if they were changed
	if requireRestorePort80 {
		//Restore port 80 listener
		SystemWideLogger.PrintAndLog("ACME", "Restoring previous port 80 listener settings", nil)
		dynamicProxyRouter.UpdatePort80ListenerState(false)
	}
	if requireRestoreHttpsRedirect {
		//Restore HTTP to HTTPS redirect setting that was temporarily enabled
		SystemWideLogger.PrintAndLog("ACME", "Restoring HTTP to HTTPS redirect settings", nil)
		dynamicProxyRouter.UpdateHttpToHttpsRedirectSetting(false)
	}

}

// HandleACMEPreferredCA return the user preferred / default CA for new subdomain auto creation
func HandleACMEPreferredCA(w http.ResponseWriter, r *http.Request) {
	ca, err := utils.PostPara(r, "set")
	if err != nil {
		//Return the current ca to user
		prefCA := "Let's Encrypt"
		sysdb.Read("acmepref", "prefca", &prefCA)
		js, _ := json.Marshal(prefCA)
		utils.SendJSONResponse(w, string(js))
	} else {
		//Check if the CA is supported
		acme.IsSupportedCA(ca, *acmeTestMode)
		//Set the new config
		sysdb.Write("acmepref", "prefca", ca)
		SystemWideLogger.Println("Updating prefered ACME CA to " + ca)
		utils.SendOK(w)
	}

}
