package acme

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"os"
	"path/filepath"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	autorenew.go

	This script handle auto renew
*/

type AutoRenewConfig struct {
	Enabled      bool     //Automatic renew is enabled
	Email        string   //Email for acme
	RenewAll     bool     //Renew all or selective renew with the slice below
	FilesToRenew []string //If RenewAll is false, renew these certificate files
	DNSServers   string   // DNS servers
}

type AutoRenewer struct {
	ConfigFilePath    string
	CertFolder        string
	AcmeHandler       *ACMEHandler
	RenewerConfig     *AutoRenewConfig
	RenewTickInterval int64
	EarlyRenewDays    int //How many days before cert expire to renew certificate
	TickerstopChan    chan bool
	Logger            *logger.Logger //System wide logger
}

type ExpiredCerts struct {
	Domains  []string
	Filepath string
}

// Create an auto renew agent, require config filepath and auto scan & renew interval (seconds)
// Set renew check interval to 0 for auto (1 day)
func NewAutoRenewer(config string, certFolder string, renewCheckInterval int64, earlyRenewDays int, AcmeHandler *ACMEHandler, logger *logger.Logger) (*AutoRenewer, error) {
	if renewCheckInterval == 0 {
		renewCheckInterval = 86400 //1 day
	}

	if earlyRenewDays == 0 {
		earlyRenewDays = 30
	}

	//Load the config file. If not found, create one
	if !utils.FileExists(config) {
		//Create one
		os.MkdirAll(filepath.Dir(config), 0775)
		newConfig := AutoRenewConfig{
			RenewAll:     true,
			FilesToRenew: []string{},
		}
		js, _ := json.MarshalIndent(newConfig, "", " ")
		err := os.WriteFile(config, js, 0775)
		if err != nil {
			return nil, errors.New("Failed to create acme auto renewer config: " + err.Error())
		}
	}

	renewerConfig := AutoRenewConfig{}
	content, err := os.ReadFile(config)
	if err != nil {
		return nil, errors.New("Failed to open acme auto renewer config: " + err.Error())
	}

	err = json.Unmarshal(content, &renewerConfig)
	if err != nil {
		return nil, errors.New("Malformed acme config file: " + err.Error())
	}

	//Create an Auto renew object
	thisRenewer := AutoRenewer{
		ConfigFilePath:    config,
		CertFolder:        certFolder,
		AcmeHandler:       AcmeHandler,
		RenewerConfig:     &renewerConfig,
		RenewTickInterval: renewCheckInterval,
		EarlyRenewDays:    earlyRenewDays,
		Logger:            logger,
	}

	thisRenewer.Logf("ACME early renew set to "+fmt.Sprint(earlyRenewDays)+" days and check interval set to "+fmt.Sprint(renewCheckInterval)+" seconds", nil)

	if thisRenewer.RenewerConfig.Enabled {
		//Start the renew ticker
		thisRenewer.StartAutoRenewTicker()

		//Check and renew certificate on startup
		go thisRenewer.CheckAndRenewCertificates()
	}

	return &thisRenewer, nil
}

func (a *AutoRenewer) Logf(message string, err error) {
	a.Logger.PrintAndLog("cert-renew", message, err)
}

func (a *AutoRenewer) StartAutoRenewTicker() {
	//Stop the previous ticker if still running
	if a.TickerstopChan != nil {
		a.TickerstopChan <- true
	}

	time.Sleep(1 * time.Second)

	ticker := time.NewTicker(time.Duration(a.RenewTickInterval) * time.Second)
	done := make(chan bool)

	//Start the ticker to check and renew every x seconds
	go func(a *AutoRenewer) {
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				a.Logf("Check and renew certificates in progress", nil)
				a.CheckAndRenewCertificates()
			}
		}
	}(a)

	a.TickerstopChan = done
}

func (a *AutoRenewer) StopAutoRenewTicker() {
	if a.TickerstopChan != nil {
		a.TickerstopChan <- true
	}

	a.TickerstopChan = nil
}

// Handle update auto renew domains
// Set opr for different mode of operations
// opr = setSelected -> Enter a list of file names (or matching rules) for auto renew
// opr = setAuto -> Set to use auto detect certificates and renew
func (a *AutoRenewer) HandleSetAutoRenewDomains(w http.ResponseWriter, r *http.Request) {
	opr, err := utils.PostPara(r, "opr")
	if err != nil {
		utils.SendErrorResponse(w, "Operation not set")
		return
	}

	if opr == "setSelected" {
		files, err := utils.PostPara(r, "domains")
		if err != nil {
			utils.SendErrorResponse(w, "Domains is not defined")
			return
		}

		//Parse it int array of string
		matchingRuleFiles := []string{}
		err = json.Unmarshal([]byte(files), &matchingRuleFiles)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		//Update the configs
		a.RenewerConfig.RenewAll = false
		a.RenewerConfig.FilesToRenew = matchingRuleFiles
		a.saveRenewConfigToFile()
		utils.SendOK(w)
	} else if opr == "setAuto" {
		a.RenewerConfig.RenewAll = true
		a.saveRenewConfigToFile()
		utils.SendOK(w)
	} else {
		utils.SendErrorResponse(w, "invalid operation given")
	}

}

// if auto renew all is true (aka auto scan), it will return []string{"*"}
func (a *AutoRenewer) HandleLoadAutoRenewDomains(w http.ResponseWriter, r *http.Request) {
	results := []string{}
	if a.RenewerConfig.RenewAll {
		//Auto pick which cert to renew.
		results = append(results, "*")
	} else {
		//Manually set the files to renew
		results = a.RenewerConfig.FilesToRenew
	}

	js, _ := json.Marshal(results)
	utils.SendJSONResponse(w, string(js))
}

func (a *AutoRenewer) HandleRenewPolicy(w http.ResponseWriter, r *http.Request) {
	//Load the current value
	js, _ := json.Marshal(a.RenewerConfig.RenewAll)
	utils.SendJSONResponse(w, string(js))
}

func (a *AutoRenewer) HandleRenewNow(w http.ResponseWriter, r *http.Request) {
	renewedDomains, err := a.CheckAndRenewCertificates()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	message := "Domains renewed"
	if len(renewedDomains) == 0 {
		message = ("All certificates are up-to-date!")
	} else {
		message = ("The following domains have been renewed: " + strings.Join(renewedDomains, ","))
	}

	js, _ := json.Marshal(message)
	utils.SendJSONResponse(w, string(js))
}

// HandleAutoRenewEnable get and set the auto renew enable state
func (a *AutoRenewer) HandleAutoRenewEnable(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		js, _ := json.Marshal(a.RenewerConfig.Enabled)
		utils.SendJSONResponse(w, string(js))
	} else if r.Method == http.MethodPost {
		val, err := utils.PostBool(r, "enable")
		if err != nil {
			utils.SendErrorResponse(w, "invalid or empty enable state")
		}
		if val {
			//Check if the email is not empty
			if a.RenewerConfig.Email == "" {
				utils.SendErrorResponse(w, "Email is not set")
				return
			}
			a.RenewerConfig.Enabled = true
			a.saveRenewConfigToFile()
			a.Logf("ACME auto renew enabled", nil)
			a.StartAutoRenewTicker()
		} else {
			a.RenewerConfig.Enabled = false
			a.saveRenewConfigToFile()
			a.Logf("ACME auto renew disabled", nil)
			a.StopAutoRenewTicker()
		}
	} else {
		http.Error(w, "405 - Method not allowed", http.StatusMethodNotAllowed)
	}

}

func (a *AutoRenewer) HandleACMEEmail(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		//Return the current email to user
		js, _ := json.Marshal(a.RenewerConfig.Email)
		utils.SendJSONResponse(w, string(js))
	} else if r.Method == http.MethodPost {
		email, err := utils.PostPara(r, "set")
		if err != nil {
			utils.SendErrorResponse(w, "invalid or empty email given")
			return
		}

		//Check if the email is valid
		_, err = mail.ParseAddress(email)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		//Set the new config
		a.RenewerConfig.Email = email
		a.saveRenewConfigToFile()

		utils.SendOK(w)
	} else {
		http.Error(w, "405 - Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Check and renew certificates. This check all the certificates in the
// certificate folder and return a list of certs that is renewed in this call
// Return string array with length 0 when no cert is expired
func (a *AutoRenewer) CheckAndRenewCertificates() ([]string, error) {
	certFolder := a.CertFolder
	files, err := os.ReadDir(certFolder)
	if err != nil {
		a.Logf("Read certificate store failed", err)
		return []string{}, err
	}

	expiredCertList := []*ExpiredCerts{}
	if a.RenewerConfig.RenewAll {
		//Scan and renew all
		for _, file := range files {
			if filepath.Ext(file.Name()) == ".crt" || filepath.Ext(file.Name()) == ".pem" {
				//This is a public key file
				certBytes, err := os.ReadFile(filepath.Join(certFolder, file.Name()))
				if err != nil {
					continue
				}
				if CertExpireSoon(certBytes, a.EarlyRenewDays) || CertIsExpired(certBytes) {
					//This cert is expired
					DNSName, err := ExtractDomains(certBytes)
					if err != nil {
						//Maybe self signed. Ignore this
						a.Logf("Encounted error when trying to resolve DNS name for cert "+file.Name(), err)
						continue
					}

					expiredCertList = append(expiredCertList, &ExpiredCerts{
						Filepath: filepath.Join(certFolder, file.Name()),
						Domains:  DNSName,
					})
				}
			}
		}
	} else {
		//Only renew those in the list
		for _, file := range files {
			fileName := file.Name()
			certName := fileName[:len(fileName)-len(filepath.Ext(fileName))]
			if contains(a.RenewerConfig.FilesToRenew, certName) {
				//This is the one to auto renew
				certBytes, err := os.ReadFile(filepath.Join(certFolder, file.Name()))
				if err != nil {
					continue
				}
				if CertExpireSoon(certBytes, a.EarlyRenewDays) || CertIsExpired(certBytes) {
					//This cert is expired
					DNSName, err := ExtractDomains(certBytes)
					if err != nil {
						//Maybe self signed. Ignore this
						a.Logf("Encounted error when trying to resolve DNS name for cert "+file.Name(), err)
						continue
					}

					expiredCertList = append(expiredCertList, &ExpiredCerts{
						Filepath: filepath.Join(certFolder, file.Name()),
						Domains:  DNSName,
					})
				}
			}
		}
	}

	return a.renewExpiredDomains(expiredCertList)
}

// Close the auto renewer
func (a *AutoRenewer) Close() {
	if a.TickerstopChan != nil {
		a.TickerstopChan <- true
	}
}

// Renew the certificate by filename extract all DNS name from the
// certificate and renew them one by one by calling to the acmeHandler
func (a *AutoRenewer) renewExpiredDomains(certs []*ExpiredCerts) ([]string, error) {
	renewedCertFiles := []string{}
	for _, expiredCert := range certs {
		a.Logf("Renewing "+expiredCert.Filepath+" (Might take a few minutes)", nil)
		fileName := filepath.Base(expiredCert.Filepath)
		certName := fileName[:len(fileName)-len(filepath.Ext(fileName))]

		// Load certificate info for ACME detail
		certInfoFilename := fmt.Sprintf("%s/%s.json", filepath.Dir(expiredCert.Filepath), certName)
		certInfo, err := LoadCertInfoJSON(certInfoFilename)
		if err != nil {
			a.Logf("Renew "+certName+"certificate error, can't get the ACME detail for certificate, trying org section as ca", err)

			if CAName, extractErr := ExtractIssuerNameFromPEM(expiredCert.Filepath); extractErr != nil {
				a.Logf("Extract issuer name for cert error, using default ca", err)
				certInfo = &CertificateInfoJSON{}
			} else {
				certInfo = &CertificateInfoJSON{AcmeName: CAName}
			}
		}

		//For upgrading config from older version of Zoraxy which don't have timeout
		if certInfo.PropTimeout == 0 {
			//Set default timeout
			certInfo.PropTimeout = 300
		}

		// Extract DNS servers from the certificate info if available
		var dnsServers string
		if len(certInfo.DNSServers) > 0 {
			dnsServers = strings.Join(certInfo.DNSServers, ",")
		}

		// Extract SANs from the existing PEM to ensure all domains are included
		sanDomains, errSan := ExtractDomainsFromPEM(expiredCert.Filepath)
		if errSan == nil && len(sanDomains) > 0 {
			expiredCert.Domains = sanDomains
			a.Logf("Using SANs from PEM for renewal: "+strings.Join(sanDomains, ","), nil)
		} else {
			a.Logf("Could not extract SANs from PEM for "+fileName+", using original domains", errSan)
		}

		_, err = a.AcmeHandler.ObtainCert(expiredCert.Domains, certName, a.RenewerConfig.Email, certInfo.AcmeName, certInfo.AcmeUrl, certInfo.SkipTLS, certInfo.UseDNS, certInfo.PropTimeout, dnsServers)
		if err != nil {
			a.Logf("Renew "+fileName+"("+strings.Join(expiredCert.Domains, ",")+") failed", err)
		} else {
			a.Logf("Successfully renewed "+filepath.Base(expiredCert.Filepath), nil)
			renewedCertFiles = append(renewedCertFiles, filepath.Base(expiredCert.Filepath))
		}
	}

	return renewedCertFiles, nil
}

// Write the current renewer config to file
func (a *AutoRenewer) saveRenewConfigToFile() error {
	js, _ := json.MarshalIndent(a.RenewerConfig, "", " ")
	return os.WriteFile(a.ConfigFilePath, js, 0775)
}

// Handle update auto renew EAD configuration
func (a *AutoRenewer) HanldeSetEAB(w http.ResponseWriter, r *http.Request) {
	kid, err := utils.GetPara(r, "kid")
	if err != nil {
		utils.SendErrorResponse(w, "kid not set")
		return
	}

	hmacEncoded, err := utils.GetPara(r, "hmacEncoded")
	if err != nil {
		utils.SendErrorResponse(w, "hmacEncoded not set")
		return
	}

	acmeDirectoryURL, err := utils.GetPara(r, "acmeDirectoryURL")
	if err != nil {
		utils.SendErrorResponse(w, "acmeDirectoryURL not set")
		return
	}

	if !a.AcmeHandler.Database.TableExists("acme") {
		a.AcmeHandler.Database.NewTable("acme")
	}

	a.AcmeHandler.Database.Write("acme", acmeDirectoryURL+"_kid", kid)
	a.AcmeHandler.Database.Write("acme", acmeDirectoryURL+"_hmacEncoded", hmacEncoded)

	utils.SendOK(w)

}

// Handle update auto renew DNS configuration
func (a *AutoRenewer) HandleSetDNS(w http.ResponseWriter, r *http.Request) {
	dnsProvider, err := utils.PostPara(r, "dnsProvider")
	if err != nil {
		utils.SendErrorResponse(w, "dnsProvider not set")
		return
	}

	dnsCredentials, err := utils.PostPara(r, "dnsCredentials")
	if err != nil {
		utils.SendErrorResponse(w, "dnsCredentials not set")
		return
	}

	filename, err := utils.PostPara(r, "filename")
	if err != nil {
		utils.SendErrorResponse(w, "filename not set")
		return
	}

	dnsServers, err := utils.PostPara(r, "dnsServers")
	if err != nil {
		dnsServers = ""
	}

	if !a.AcmeHandler.Database.TableExists("acme") {
		a.AcmeHandler.Database.NewTable("acme")
	}

	a.AcmeHandler.Database.Write("acme", filename+"_dns_provider", dnsProvider)
	a.AcmeHandler.Database.Write("acme", filename+"_dns_credentials", dnsCredentials)
	a.AcmeHandler.Database.Write("acme", filename+"_dns_servers", dnsServers)

	utils.SendOK(w)

}
