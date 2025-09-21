package acme

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/certcrypto"
	"github.com/go-acme/lego/v4/certificate"
	"github.com/go-acme/lego/v4/challenge/dns01"
	"github.com/go-acme/lego/v4/challenge/http01"
	"github.com/go-acme/lego/v4/lego"
	"github.com/go-acme/lego/v4/registration"
	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/utils"
)

var defaultNameservers = []string{
	"8.8.8.8:53", // Google DNS
	"8.8.4.4:53", // Google DNS
	"1.1.1.1:53", // Cloudflare DNS
	"1.0.0.1:53", // Cloudflare DNS
}

type CertificateInfoJSON struct {
	AcmeName    string   `json:"acme_name"`  //ACME provider name
	AcmeUrl     string   `json:"acme_url"`   //Custom ACME URL (if any)
	SkipTLS     bool     `json:"skip_tls"`   //Skip TLS verification of upstream
	UseDNS      bool     `json:"dns"`        //Use DNS challenge
	PropTimeout int      `json:"prop_time"`  //Propagation timeout
	DNSServers  []string `json:"dnsServers"` // DNS servers
}

// ACMEUser represents a user in the ACME system.
type ACMEUser struct {
	Email        string
	Registration *registration.Resource
	key          crypto.PrivateKey
}

type EABConfig struct {
	Kid     string `json:"kid"`
	HmacKey string `json:"HmacKey"`
}

// GetEmail returns the email of the ACMEUser.
func (u *ACMEUser) GetEmail() string {
	return u.Email
}

// GetRegistration returns the registration resource of the ACMEUser.
func (u ACMEUser) GetRegistration() *registration.Resource {
	return u.Registration
}

// GetPrivateKey returns the private key of the ACMEUser.
func (u *ACMEUser) GetPrivateKey() crypto.PrivateKey {
	return u.key
}

// ACMEHandler handles ACME-related operations.
type ACMEHandler struct {
	DefaultAcmeServer string
	Port              string
	Database          *database.Database
	Logger            *logger.Logger
}

// NewACME creates a new ACMEHandler instance.
func NewACME(defaultAcmeServer string, port string, database *database.Database, logger *logger.Logger) *ACMEHandler {
	return &ACMEHandler{
		DefaultAcmeServer: defaultAcmeServer,
		Port:              port,
		Database:          database,
		Logger:            logger,
	}
}

func (a *ACMEHandler) Logf(message string, err error) {
	a.Logger.PrintAndLog("ACME", message, err)
}

// Close closes the ACMEHandler.
// ACME Handler does not need to close anything
// Function defined for future compatibility
func (a *ACMEHandler) Close() error {
	return nil
}

// ObtainCert obtains a certificate for the specified domains.
func (a *ACMEHandler) ObtainCert(domains []string, certificateName string, email string, caName string, caUrl string, skipTLS bool, useDNS bool, propagationTimeout int, dnsServers string) (bool, error) {
	a.Logf("Obtaining certificate for: "+strings.Join(domains, ", "), nil)

	// generate private key
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		a.Logf("Private key generation failed", err)
		return false, err
	}

	// create a admin user for our new generation
	adminUser := ACMEUser{
		Email: email,
		key:   privateKey,
	}

	// create config
	config := lego.NewConfig(&adminUser)

	// skip TLS verify if need
	// Ref: https://github.com/go-acme/lego/blob/6af2c756ac73a9cb401621afca722d0f4112b1b8/lego/client_config.go#L74
	if skipTLS {
		a.Logf("Ignoring TLS/SSL Verification Error for ACME Server", nil)
		config.HTTPClient.Transport = &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout:   30 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
			TLSHandshakeTimeout:   30 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: true,
			},
		}
	}

	//Fallback to Let's Encrypt if it is not set
	if caName == "" {
		caName = "Let's Encrypt"
	}

	// setup the custom ACME url endpoint.
	if caUrl != "" {
		config.CADirURL = caUrl
	}

	// if not custom ACME url, load it from ca.json
	if caName == "custom" {
		a.Logf("Using Custom ACME "+caUrl+" for CA Directory URL", nil)
	} else {
		caLinkOverwrite, err := loadCAApiServerFromName(caName)
		if err == nil {
			config.CADirURL = caLinkOverwrite
			a.Logf("Using "+caLinkOverwrite+" for CA Directory URL", nil)
		} else {
			// (caName == "" || caUrl == "") will use default acme
			config.CADirURL = a.DefaultAcmeServer
			a.Logf("Using Default ACME "+a.DefaultAcmeServer+" for CA Directory URL", nil)
		}
	}

	config.Certificate.KeyType = certcrypto.RSA2048

	client, err := lego.NewClient(config)
	if err != nil {
		a.Logf("Failed to spawn new ACME client from current config", err)
		return false, err
	}

	// Load certificate info from JSON file
	certInfo, err := LoadCertInfoJSON(fmt.Sprintf("./conf/certs/%s.json", certificateName))
	if err == nil {
		useDNS = certInfo.UseDNS
		if dnsServers == "" && certInfo.DNSServers != nil && len(certInfo.DNSServers) > 0 {
			dnsServers = strings.Join(certInfo.DNSServers, ",")
		}
		propagationTimeout = certInfo.PropTimeout
	}

	// Clean DNS servers
	dnsNameservers := strings.Split(dnsServers, ",")
	for i := range dnsNameservers {
		dnsNameservers[i] = strings.TrimSpace(dnsNameservers[i])
	}

	// setup how to receive challenge
	if useDNS {
		if !a.Database.TableExists("acme") {
			a.Database.NewTable("acme")
			return false, errors.New("DNS Provider and DNS Credential configuration required for ACME Provider (Error -1)")
		}

		if !a.Database.KeyExists("acme", certificateName+"_dns_provider") || !a.Database.KeyExists("acme", certificateName+"_dns_credentials") {
			return false, errors.New("DNS Provider and DNS Credential configuration required for ACME Provider (Error -2)")
		}

		var dnsCredentials string
		err := a.Database.Read("acme", certificateName+"_dns_credentials", &dnsCredentials)
		if err != nil {
			a.Logf("Read DNS credential failed", err)
			return false, err
		}

		var dnsProvider string
		err = a.Database.Read("acme", certificateName+"_dns_provider", &dnsProvider)
		if err != nil {
			a.Logf("Read DNS Provider failed", err)
			return false, err
		}

		provider, err := GetDnsChallengeProviderByName(dnsProvider, dnsCredentials, propagationTimeout)
		if err != nil {
			a.Logf("Unable to resolve DNS challenge provider", err)
			return false, err
		}

		if len(dnsNameservers) > 0 && dnsNameservers[0] != "" {
			a.Logf("Using DNS servers: "+strings.Join(dnsNameservers, ", "), nil)
			err = client.Challenge.SetDNS01Provider(provider, dns01.AddRecursiveNameservers(dnsNameservers))
		} else {
			// Use default DNS-01 nameservers if dnsServers is empty
			err = client.Challenge.SetDNS01Provider(provider, dns01.AddRecursiveNameservers(defaultNameservers))
		}
		if err != nil {
			a.Logf("Failed to resolve DNS01 Provider", err)
			return false, err
		}
	} else {
		err = client.Challenge.SetHTTP01Provider(http01.NewProviderServer("", a.Port))
		if err != nil {
			a.Logf("Failed to resolve HTTP01 Provider", err)
			return false, err
		}
	}

	// New users will need to register
	/*
		reg, err := client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			log.Println(err)
			return false, err
		}
	*/
	var reg *registration.Resource
	// New users will need to register
	if client.GetExternalAccountRequired() {
		a.Logf("External Account Required for this ACME Provider", nil)
		// IF KID and HmacEncoded is overidden

		if !a.Database.TableExists("acme") {
			a.Database.NewTable("acme")
			return false, errors.New("kid and HmacEncoded configuration required for ACME Provider (Error -1)")
		}

		if !a.Database.KeyExists("acme", config.CADirURL+"_kid") || !a.Database.KeyExists("acme", config.CADirURL+"_hmacEncoded") {
			return false, errors.New("kid and HmacEncoded configuration required for ACME Provider (Error -2)")
		}

		var kid string
		var hmacEncoded string
		err := a.Database.Read("acme", config.CADirURL+"_kid", &kid)
		if err != nil {
			a.Logf("Failed to read kid from database", err)
			return false, err
		}

		err = a.Database.Read("acme", config.CADirURL+"_hmacEncoded", &hmacEncoded)
		if err != nil {
			a.Logf("Failed to read HMAC from database", err)
			return false, err
		}

		a.Logf("EAB Credential retrieved: "+kid+" / "+hmacEncoded, nil)
		if kid != "" && hmacEncoded != "" {
			reg, err = client.Registration.RegisterWithExternalAccountBinding(registration.RegisterEABOptions{
				TermsOfServiceAgreed: true,
				Kid:                  kid,
				HmacEncoded:          hmacEncoded,
			})
		}
		if err != nil {
			a.Logf("Register with external account binder failed", err)
			return false, err
		}
		//return false, errors.New("External Account Required for this ACME Provider.")
	} else {
		reg, err = client.Registration.Register(registration.RegisterOptions{TermsOfServiceAgreed: true})
		if err != nil {
			a.Logf("Unable to register client", err)
			return false, err
		}
	}
	adminUser.Registration = reg

	// obtain the certificate
	request := certificate.ObtainRequest{
		Domains: domains,
		Bundle:  true,
	}
	certificates, err := client.Certificate.Obtain(request)
	if err != nil {
		a.Logf("Obtain certificate failed", err)
		return false, err
	}

	// Each certificate comes back with the cert bytes, the bytes of the client's
	// private key, and a certificate URL.
	err = os.WriteFile("./conf/certs/"+certificateName+".pem", certificates.Certificate, 0777)
	if err != nil {
		a.Logf("Failed to write public key to disk", err)
		return false, err
	}
	err = os.WriteFile("./conf/certs/"+certificateName+".key", certificates.PrivateKey, 0777)
	if err != nil {
		a.Logf("Failed to write private key to disk", err)
		return false, err
	}

	// Save certificate's ACME info for renew usage
	certInfo = &CertificateInfoJSON{
		AcmeName:    caName,
		AcmeUrl:     caUrl,
		SkipTLS:     skipTLS,
		UseDNS:      useDNS,
		PropTimeout: propagationTimeout,
		DNSServers:  dnsNameservers,
	}

	certInfoBytes, err := json.Marshal(certInfo)
	if err != nil {
		a.Logf("Marshal certificate renew config failed", err)
		return false, err
	}

	err = os.WriteFile("./conf/certs/"+certificateName+".json", certInfoBytes, 0777)
	if err != nil {
		a.Logf("Failed to write certificate renew config to file", err)
		return false, err
	}

	return true, nil
}

// CheckCertificate returns a list of domains that are in expired certificates.
// It will return all domains that is in expired certificates
// *** if there is a vaild certificate contains the domain and there is a expired certificate contains the same domain
// it will said expired as well!
func (a *ACMEHandler) CheckCertificate() []string {
	// read from dir
	filenames, err := os.ReadDir("./conf/certs/")

	expiredCerts := []string{}

	if err != nil {
		a.Logf("Failed to load certificate folder", err)
		return []string{}
	}

	for _, filename := range filenames {
		certFilepath := filepath.Join("./conf/certs/", filename.Name())

		certBytes, err := os.ReadFile(certFilepath)
		if err != nil {
			// Unable to load this file
			continue
		} else {
			// Cert loaded. Check its expiry time
			block, _ := pem.Decode(certBytes)
			if block != nil {
				cert, err := x509.ParseCertificate(block.Bytes)
				if err == nil {
					elapsed := time.Since(cert.NotAfter)
					if elapsed > 0 {
						// if it is expired then add it in
						// make sure it's uniqueless
						for _, dnsName := range cert.DNSNames {
							if !contains(expiredCerts, dnsName) {
								expiredCerts = append(expiredCerts, dnsName)
							}
						}
						if !contains(expiredCerts, cert.Subject.CommonName) {
							expiredCerts = append(expiredCerts, cert.Subject.CommonName)
						}
					}
				}
			}
		}
	}

	return expiredCerts
}

// return the current port number
func (a *ACMEHandler) Getport() string {
	return a.Port
}

// contains checks if a string is present in a slice.
func contains(slice []string, str string) bool {
	for _, s := range slice {
		if s == str {
			return true
		}
	}
	return false
}

// HandleGetExpiredDomains handles the HTTP GET request to retrieve the list of expired domains.
// It calls the CheckCertificate method to obtain the expired domains and sends a JSON response
// containing the list of expired domains.
func (a *ACMEHandler) HandleGetExpiredDomains(w http.ResponseWriter, r *http.Request) {
	type ExpiredDomains struct {
		Domain []string `json:"domain"`
	}

	info := ExpiredDomains{
		Domain: a.CheckCertificate(),
	}

	js, _ := json.MarshalIndent(info, "", " ")
	utils.SendJSONResponse(w, string(js))
}

// HandleRenewCertificate handles the HTTP GET request to renew a certificate for the provided domains.
// It retrieves the domains and filename parameters from the request, calls the ObtainCert method
// to renew the certificate, and sends a JSON response indicating the result of the renewal process.
func (a *ACMEHandler) HandleRenewCertificate(w http.ResponseWriter, r *http.Request) {
	domainPara, err := utils.PostPara(r, "domains")
	
	//Clean each domain
	cleanedDomains := []string{}
	if (domainPara != "") {
		for _, d := range strings.Split(domainPara, ",") {
			// Apply normalization on each domain
			nd, err := NormalizeDomain(d)
			if err != nil {
				utils.SendErrorResponse(w, jsonEscape(err.Error()))
				return
			}	
			cleanedDomains = append(cleanedDomains, nd) 
		}
	}

	if err != nil {
		utils.SendErrorResponse(w, jsonEscape(err.Error()))
		return
	}

	filename, err := utils.PostPara(r, "filename")
	if err != nil {
		utils.SendErrorResponse(w, jsonEscape(err.Error()))
		return
	}
	//Make sure the wildcard * do not goes into the filename
	filename = strings.ReplaceAll(filename, "*", "_")

	email, err := utils.PostPara(r, "email")
	if err != nil {
		utils.SendErrorResponse(w, jsonEscape(err.Error()))
		return
	}

	var caUrl string

	ca, err := utils.PostPara(r, "ca")
	if err != nil {
		a.Logf("CA not set. Using default", nil)
		ca, caUrl = "", ""
	}

	if ca == "custom" {
		caUrl, err = utils.PostPara(r, "caURL")
		if err != nil {
			a.Logf("Custom CA set but no URL provide, Using default", nil)
			ca, caUrl = "", ""
		}
	}

	if ca == "" {
		//default. Use Let's Encrypt
		ca = "Let's Encrypt"
	}

	var skipTLS bool

	if skipTLSString, err := utils.PostPara(r, "skipTLS"); err != nil {
		skipTLS = false
	} else if skipTLSString != "true" {
		skipTLS = false
	} else {
		skipTLS = true
	}

	var dns bool

	if dnsString, err := utils.PostPara(r, "dns"); err != nil {
		dns = false
	} else if dnsString != "true" {
		dns = false
	} else {
		dns = true
	}


	// Default propagation timeout is 300 seconds
	propagationTimeout := 300
	if dns {
		ppgTimeout, err := utils.PostPara(r, "ppgTimeout")
		if err == nil {
			propagationTimeout, err = strconv.Atoi(ppgTimeout)
			if err != nil {
				utils.SendErrorResponse(w, "Invalid propagation timeout value")
				return
			}
			if propagationTimeout < 60 {
				//Minimum propagation timeout is 60 seconds
				propagationTimeout = 60
			}
		}
	}

	// Extract SANs from existing PEM to ensure all domains are included
	pemPath := fmt.Sprintf("./conf/certs/%s.pem", filename)
	sanDomains, err := ExtractDomainsFromPEM(pemPath)
	if err == nil {
		// Merge domainPara + SANs
		domainSet := map[string]struct{}{}
		for _, d := range cleanedDomains {
			domainSet[d] = struct{}{}
		}
		for _, d := range sanDomains {
			domainSet[d] = struct{}{}
		}

		// Rebuild cleanedDomains with all unique domains
		cleanedDomains = []string{}
		for d := range domainSet {
			cleanedDomains = append(cleanedDomains, d)
		}

		a.Logf("Renewal domains including SANs from PEM: "+strings.Join(cleanedDomains, ","), nil)
	} else {
		a.Logf("Could not extract SANs from PEM, using domainPara only", err)
	}


	// Extract DNS servers from the request
	var dnsServers []string
	dnsServersPara, err := utils.PostPara(r, "dnsServers")
	if err == nil && dnsServersPara != "" {
		dnsServers = strings.Split(dnsServersPara, ",")
		for i := range dnsServers {
			dnsServers[i] = strings.TrimSpace(dnsServers[i])
		}
	}

	// Convert DNS servers slice to a single string
	dnsServersString := strings.Join(dnsServers, ",")

	result, err := a.ObtainCert(cleanedDomains, filename, email, ca, caUrl, skipTLS, dns, propagationTimeout, dnsServersString)
	if err != nil {
		utils.SendErrorResponse(w, jsonEscape(err.Error()))
		return
	}
	utils.SendJSONResponse(w, strconv.FormatBool(result))
}

// Escape JSON string
func jsonEscape(i string) string {
	b, err := json.Marshal(i)
	if err != nil {
		//log.Println("Unable to escape json data: " + err.Error())
		return i
	}
	s := string(b)
	return s[1 : len(s)-1]
}

// Helper function to check if a port is in use
func IsPortInUse(port int) bool {
	address := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return true // Port is in use
	}
	defer listener.Close()
	return false // Port is not in use

}

// Load cert information from json file
func LoadCertInfoJSON(filename string) (*CertificateInfoJSON, error) {
	certInfoBytes, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	certInfo := &CertificateInfoJSON{}
	if err = json.Unmarshal(certInfoBytes, certInfo); err != nil {
		return nil, err
	}

	// Clean DNS servers
	for i := range certInfo.DNSServers {
		certInfo.DNSServers[i] = strings.TrimSpace(certInfo.DNSServers[i])
	}

	return certInfo, nil
}
