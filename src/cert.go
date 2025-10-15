package main

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"

	"imuslab.com/zoraxy/mod/utils"
)

// Handle front-end toggling TLS mode
func handleToggleTLSProxy(w http.ResponseWriter, r *http.Request) {
	currentTlsSetting := true //Default to true
	if dynamicProxyRouter.Option != nil {
		currentTlsSetting = dynamicProxyRouter.Option.UseTls
	}
	if sysdb.KeyExists("settings", "usetls") {
		sysdb.Read("settings", "usetls", &currentTlsSetting)
	}

	switch r.Method {
	case http.MethodGet:
		//Get the current status
		js, _ := json.Marshal(currentTlsSetting)
		utils.SendJSONResponse(w, string(js))
	case http.MethodPost:
		newState, err := utils.PostBool(r, "set")
		if err != nil {
			utils.SendErrorResponse(w, "new state not set or invalid")
			return
		}
		if newState {
			sysdb.Write("settings", "usetls", true)
			SystemWideLogger.Println("Enabling TLS mode on reverse proxy")
			dynamicProxyRouter.UpdateTLSSetting(true)
		} else {
			sysdb.Write("settings", "usetls", false)
			SystemWideLogger.Println("Disabling TLS mode on reverse proxy")
			dynamicProxyRouter.UpdateTLSSetting(false)
		}
		utils.SendOK(w)
	default:
		http.Error(w, "405 - Method not allowed", http.StatusMethodNotAllowed)
	}
}

func minTlsVersionStringToUint16(version string) uint16 {
	// Update the setting
	var tlsVersionUint16 uint16
	switch version {
	case "1.0":
		tlsVersionUint16 = 0x0301
	case "1.1":
		tlsVersionUint16 = 0x0302
	case "1.2":
		tlsVersionUint16 = 0x0303
	case "1.3":
		tlsVersionUint16 = 0x0304
	}
	return tlsVersionUint16
}

// Handle the GET and SET of reverse proxy minimum TLS version
func handleSetTlsMinVersion(w http.ResponseWriter, r *http.Request) {
	newVersion, err := utils.PostPara(r, "set")
	if err != nil {
		// GET
		var minTLSVersion string = "1.2" // Default to 1.2
		if sysdb.KeyExists("settings", "minTLSVersion") {
			sysdb.Read("settings", "minTLSVersion", &minTLSVersion)
		}
		js, _ := json.Marshal(minTLSVersion)
		utils.SendJSONResponse(w, string(js))
		return
	}

	// Validate input
	allowed := map[string]bool{"1.0": true, "1.1": true, "1.2": true, "1.3": true}
	if !allowed[newVersion] {
		utils.SendErrorResponse(w, "invalid TLS version")
		return
	}

	sysdb.Write("settings", "minTLSVersion", newVersion)
	tlsVersionUint16 := minTlsVersionStringToUint16(newVersion)
	// Update the setting
	SystemWideLogger.PrintAndLog("TLS", "Updating minimum TLS version to v"+newVersion+" or above", nil)
	dynamicProxyRouter.SetTlsMinVersion(tlsVersionUint16)
	utils.SendOK(w)
}

func handleCertTryResolve(w http.ResponseWriter, r *http.Request) {
	// get the domain
	domain, err := utils.GetPara(r, "domain")
	if err != nil {
		utils.SendErrorResponse(w, "invalid domain given")
		return
	}

	// get the proxy rule, the pass in domain value must be root or matching domain
	proxyRule, err := dynamicProxyRouter.GetProxyEndpointById(domain, false)
	if err != nil {
		//Try to resolve the domain via alias
		proxyRule, err = dynamicProxyRouter.GetProxyEndpointByAlias(domain)
		if err != nil {
			//No matching rule found
			utils.SendErrorResponse(w, "proxy rule not found for domain: "+domain)
			return
		}
	}

	// list all the alias domains for this rule
	allDomains := []string{proxyRule.RootOrMatchingDomain}
	aliasDomains := []string{}
	for _, alias := range proxyRule.MatchingDomainAlias {
		if alias != "" {
			aliasDomains = append(aliasDomains, alias)
			allDomains = append(allDomains, alias)
		}
	}

	// Try to resolve the domain
	domainKeyPairs := map[string]string{}
	for _, thisDomain := range allDomains {
		pubkey, prikey, err := tlsCertManager.GetCertificateByHostname(thisDomain)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		//Make sure pubkey and private key are not empty
		if pubkey == "" || prikey == "" {
			domainKeyPairs[thisDomain] = ""
		} else {
			//Store the key pair
			keyname := strings.TrimSuffix(filepath.Base(pubkey), filepath.Ext(pubkey))
			if keyname == "localhost" {
				//Internal certs like localhost should not be used
				//report as "fallback" key
				keyname = "fallback certificate"
			}
			domainKeyPairs[thisDomain] = keyname
		}

	}

	//A domain must be UseDNSValidation if it is a wildcard domain or its alias is a wildcard domain
	useDNSValidation := strings.HasPrefix(proxyRule.RootOrMatchingDomain, "*")
	for _, alias := range aliasDomains {
		if strings.HasPrefix(alias, "*") || strings.HasPrefix(domain, "*") {
			useDNSValidation = true
		}
	}

	type CertInfo struct {
		Domain           string            `json:"domain"`
		AliasDomains     []string          `json:"alias_domains"`
		DomainKeyPair    map[string]string `json:"domain_key_pair"`
		UseDNSValidation bool              `json:"use_dns_validation"`
	}

	result := &CertInfo{
		Domain:           proxyRule.RootOrMatchingDomain,
		AliasDomains:     aliasDomains,
		DomainKeyPair:    domainKeyPairs,
		UseDNSValidation: useDNSValidation,
	}

	js, _ := json.Marshal(result)
	utils.SendJSONResponse(w, string(js))
}

func handleSetDomainPreferredCertificate(w http.ResponseWriter, r *http.Request) {
	//Get the domain
	domain, err := utils.PostPara(r, "domain")
	if err != nil {
		utils.SendErrorResponse(w, "invalid domain given")
		return
	}

	//Get the certificate name
	certName, err := utils.PostPara(r, "certname")
	if err != nil {
		utils.SendErrorResponse(w, "invalid certificate name given")
		return
	}

	//Load the target endpoint
	ept, err := dynamicProxyRouter.GetProxyEndpointById(domain, true)
	if err != nil {
		utils.SendErrorResponse(w, "proxy rule not found for domain: "+domain)
		return
	}

	//Set the preferred certificate for the domain
	err = dynamicProxyRouter.SetPreferredCertificateForDomain(ept, domain, certName)
	if err != nil {
		utils.SendErrorResponse(w, "failed to set preferred certificate: "+err.Error())
		return
	}

	err = SaveReverseProxyConfig(ept)
	if err != nil {
		utils.SendErrorResponse(w, "failed to save reverse proxy config: "+err.Error())
		return
	}

	utils.SendOK(w)
}
