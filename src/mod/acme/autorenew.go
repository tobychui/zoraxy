package acme

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
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
	Enabled                         bool
	LegacyEmail                     string `json:"Email,omitempty"`
	RenewAll                        bool
	FilesToRenew                    []string
	DNSServers                      string
	CertificateAuthorities          []CertificateAuthority
	DefaultCertificateAuthority     string
	CertificateAuthorityAssignments map[string]string
}

// CertificateAuthority is a reusable ACME issuer profile. Multiple profiles
// may use the same provider or different custom ACME directories.
type CertificateAuthority struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Type    string `json:"type"`
	Email   string `json:"email"`
	URL     string `json:"url,omitempty"`
	SkipTLS bool   `json:"skipTLS"`
}

type certificateAuthorityResponse struct {
	Authorities      []CertificateAuthority `json:"authorities"`
	DefaultAuthority string                 `json:"defaultAuthority"`
	Assignments      map[string]string      `json:"assignments"`
}

var invalidCAIDCharacters = regexp.MustCompile(`[^a-z0-9]+`)

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

	// acme_conf.json predates configurable CA profiles. Populate the new
	// structure automatically and import the legacy singleton preference when it
	// is available in the system database.
	if thisRenewer.migrateCertificateAuthorities() {
		if err := thisRenewer.saveRenewConfigToFile(); err != nil {
			return nil, errors.New("Failed to migrate acme auto renewer config: " + err.Error())
		}
	}
	thisRenewer.migrateLegacyACMEState()

	thisRenewer.Logf("ACME early renew set to "+fmt.Sprint(earlyRenewDays)+" days and check interval set to "+fmt.Sprint(renewCheckInterval)+" seconds", nil)

	if thisRenewer.RenewerConfig.Enabled {
		//Start the renew ticker
		thisRenewer.StartAutoRenewTicker()

		//Check and renew certificate on startup
		go thisRenewer.CheckAndRenewCertificates()
	}

	return &thisRenewer, nil
}

// migrateLegacyACMEState assigns each legacy account and EAB credential pair
// to at most one profile. The default profile gets first choice. Keeping the
// legacy records in place preserves ObtainCert compatibility, while managed
// profiles only read their isolated copies at runtime.
func (a *AutoRenewer) migrateLegacyACMEState() {
	if a.AcmeHandler == nil || a.AcmeHandler.Database == nil {
		return
	}

	authorities := make([]CertificateAuthority, 0, len(a.RenewerConfig.CertificateAuthorities))
	if defaultAuthority, ok := a.certificateAuthorityByID(a.RenewerConfig.DefaultCertificateAuthority); ok {
		authorities = append(authorities, defaultAuthority)
	}
	for _, authority := range a.RenewerConfig.CertificateAuthorities {
		if authority.ID != a.RenewerConfig.DefaultCertificateAuthority {
			authorities = append(authorities, authority)
		}
	}

	type migrationCandidate struct {
		authority    CertificateAuthority
		directoryURL string
	}
	candidates := make([]migrationCandidate, 0, len(authorities))
	for _, authority := range authorities {
		directoryURL := strings.TrimSpace(authority.URL)
		if authority.Type != "custom" {
			resolvedURL, err := loadCAApiServerFromName(authority.Type, a.AcmeHandler.TestMode)
			if err != nil {
				continue
			}
			directoryURL = resolvedURL
		}
		if directoryURL != "" && authority.ID != "" {
			candidates = append(candidates, migrationCandidate{authority: authority, directoryURL: directoryURL})
		}
	}

	claimedAccounts := map[string]bool{}
	claimedEAB := map[string]bool{}
	// Reserve legacy state that was already copied on an earlier startup. This
	// prevents a later default-profile change from copying it to another profile.
	for _, candidate := range candidates {
		authority := candidate.authority
		directoryURL := candidate.directoryURL
		if a.AcmeHandler.Database.TableExists(acmeAccountTable) &&
			a.AcmeHandler.Database.KeyExists(acmeAccountTable, accountDBKey(directoryURL, authority.Email, authority.ID)) {
			claimedAccounts[legacyAccountDBKey(directoryURL, authority.Email)] = true
		}
		if a.AcmeHandler.Database.TableExists("acme") {
			profilePrefix := eabProfileKeyPrefix(authority.ID)
			if a.AcmeHandler.Database.KeyExists("acme", profilePrefix+"_kid") ||
				a.AcmeHandler.Database.KeyExists("acme", profilePrefix+"_hmacEncoded") {
				claimedEAB[directoryURL] = true
			}
		}
	}

	for _, candidate := range candidates {
		authority := candidate.authority
		directoryURL := candidate.directoryURL
		if a.AcmeHandler.Database.TableExists(acmeAccountTable) {
			legacyKey := legacyAccountDBKey(directoryURL, authority.Email)
			profileKey := accountDBKey(directoryURL, authority.Email, authority.ID)
			if !claimedAccounts[legacyKey] && a.AcmeHandler.Database.KeyExists(acmeAccountTable, legacyKey) {
				claimedAccounts[legacyKey] = true
				var user ACMEUser
				if err := a.AcmeHandler.Database.Read(acmeAccountTable, legacyKey, &user); err != nil {
					a.Logf("Failed to read legacy ACME account for profile "+authority.Name, err)
				} else if err := a.AcmeHandler.Database.Write(acmeAccountTable, profileKey, &user); err != nil {
					a.Logf("Failed to migrate legacy ACME account to profile "+authority.Name, err)
				}
			}
		}

		if a.AcmeHandler.Database.TableExists("acme") {
			legacyPrefix := directoryURL
			profilePrefix := eabProfileKeyPrefix(authority.ID)
			if !claimedEAB[legacyPrefix] &&
				a.AcmeHandler.Database.KeyExists("acme", legacyPrefix+"_kid") &&
				a.AcmeHandler.Database.KeyExists("acme", legacyPrefix+"_hmacEncoded") {
				claimedEAB[legacyPrefix] = true
				var kid string
				var hmacEncoded string
				if err := a.AcmeHandler.Database.Read("acme", legacyPrefix+"_kid", &kid); err != nil {
					a.Logf("Failed to read legacy EAB KID for profile "+authority.Name, err)
					continue
				}
				if err := a.AcmeHandler.Database.Read("acme", legacyPrefix+"_hmacEncoded", &hmacEncoded); err != nil {
					a.Logf("Failed to read legacy EAB HMAC for profile "+authority.Name, err)
					continue
				}
				if err := a.AcmeHandler.Database.Write("acme", profilePrefix+"_kid", kid); err != nil {
					a.Logf("Failed to migrate legacy EAB KID to profile "+authority.Name, err)
					continue
				}
				if err := a.AcmeHandler.Database.Write("acme", profilePrefix+"_hmacEncoded", hmacEncoded); err != nil {
					a.Logf("Failed to migrate legacy EAB HMAC to profile "+authority.Name, err)
					continue
				}
			}
		}
	}
}

func (a *AutoRenewer) migrateCertificateAuthorities() bool {
	changed := false
	if a.RenewerConfig.CertificateAuthorityAssignments == nil {
		a.RenewerConfig.CertificateAuthorityAssignments = map[string]string{}
		changed = true
	}

	if len(a.RenewerConfig.CertificateAuthorities) == 0 {
		caType := "Let's Encrypt"
		caURL := ""
		skipTLS := false
		if a.AcmeHandler != nil && a.AcmeHandler.Database != nil {
			a.AcmeHandler.Database.Read("acmepref", "prefca", &caType)
			a.AcmeHandler.Database.Read("acmepref", "prefcaurl", &caURL)
			a.AcmeHandler.Database.Read("acmepref", "skipTLS", &skipTLS)
		}
		if caType != "custom" && (a.AcmeHandler == nil || !IsSupportedCA(caType, a.AcmeHandler.TestMode)) {
			caType, caURL, skipTLS = "Let's Encrypt", "", false
		} else if caType != "custom" {
			// The legacy preference kept the previous custom URL and SkipTLS
			// values when switching back to a named provider. They must not be
			// carried into that provider's profile.
			caURL, skipTLS = "", false
		}
		name := caType
		if caType == "custom" {
			name = "Custom ACME Server"
		}
		profile := CertificateAuthority{ID: caID(name), Name: name, Type: caType, Email: a.RenewerConfig.LegacyEmail, URL: caURL, SkipTLS: skipTLS}
		a.RenewerConfig.CertificateAuthorities = []CertificateAuthority{profile}
		a.RenewerConfig.DefaultCertificateAuthority = profile.ID
		changed = true

		// Before CA profiles existed, every certificate retained its issuer in
		// <certificate>.json. Preserve that issuer during migration instead of
		// silently renewing all existing certificates through the new default.
		if entries, err := os.ReadDir(a.CertFolder); err == nil {
			for _, entry := range entries {
				if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
					continue
				}
				certificateName := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
				certificateInfo, err := LoadCertInfoJSON(filepath.Join(a.CertFolder, entry.Name()))
				if err != nil || certificateInfo.AcmeName == "" {
					continue
				}

				authority := CertificateAuthority{
					Name:    certificateInfo.AcmeName,
					Type:    certificateInfo.AcmeName,
					Email:   a.RenewerConfig.LegacyEmail,
					URL:     certificateInfo.AcmeUrl,
					SkipTLS: certificateInfo.SkipTLS,
				}
				if authority.Type == "custom" {
					authority.Name = "Custom ACME Server"
				} else {
					// Named providers resolve their directory from ca.json. The URL in
					// the legacy metadata is the resolved URL, not a user override.
					authority.URL = ""
					authority.SkipTLS = false
				}

				authorityID := a.matchingCertificateAuthorityID(authority)
				if authorityID == "" {
					authority.ID = a.uniqueCAID(authority.Name)
					a.RenewerConfig.CertificateAuthorities = append(a.RenewerConfig.CertificateAuthorities, authority)
					authorityID = authority.ID
				}
				a.RenewerConfig.CertificateAuthorityAssignments[certificateName] = authorityID
			}
		}
	}
	for i := range a.RenewerConfig.CertificateAuthorities {
		// Also covers configurations written during the transition to profile
		// based CAs before email became profile-specific.
		if a.RenewerConfig.CertificateAuthorities[i].Email == "" && a.RenewerConfig.LegacyEmail != "" {
			a.RenewerConfig.CertificateAuthorities[i].Email = a.RenewerConfig.LegacyEmail
			changed = true
		}
	}
	if a.RenewerConfig.LegacyEmail != "" {
		a.RenewerConfig.LegacyEmail = ""
		changed = true
	}

	if _, ok := a.certificateAuthorityByID(a.RenewerConfig.DefaultCertificateAuthority); !ok {
		a.RenewerConfig.DefaultCertificateAuthority = a.RenewerConfig.CertificateAuthorities[0].ID
		changed = true
	}
	for filename, id := range a.RenewerConfig.CertificateAuthorityAssignments {
		if _, ok := a.certificateAuthorityByID(id); !ok {
			delete(a.RenewerConfig.CertificateAuthorityAssignments, filename)
			changed = true
		}
	}
	return changed
}

func (a *AutoRenewer) matchingCertificateAuthorityID(candidate CertificateAuthority) string {
	for _, authority := range a.RenewerConfig.CertificateAuthorities {
		if authority.Type == candidate.Type && authority.Email == candidate.Email &&
			authority.URL == candidate.URL && authority.SkipTLS == candidate.SkipTLS {
			return authority.ID
		}
	}
	return ""
}

func caID(name string) string {
	id := strings.Trim(invalidCAIDCharacters.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-"), "-")
	if id == "" {
		return "certificate-authority"
	}
	return id
}

func (a *AutoRenewer) uniqueCAID(name string) string {
	base := caID(name)
	id := base
	for suffix := 2; ; suffix++ {
		if _, exists := a.certificateAuthorityByID(id); !exists {
			return id
		}
		id = fmt.Sprintf("%s-%d", base, suffix)
	}
}

func (a *AutoRenewer) certificateAuthorityByID(id string) (CertificateAuthority, bool) {
	for _, authority := range a.RenewerConfig.CertificateAuthorities {
		if authority.ID == id {
			return authority, true
		}
	}
	return CertificateAuthority{}, false
}

// CertificateAuthorityFor returns the explicit CA for a certificate, falling
// back to the configured default profile.
func (a *AutoRenewer) CertificateAuthorityFor(filename string) CertificateAuthority {
	id := a.RenewerConfig.CertificateAuthorityAssignments[filename]
	if authority, ok := a.certificateAuthorityByID(id); ok {
		return authority
	}
	authority, _ := a.certificateAuthorityByID(a.RenewerConfig.DefaultCertificateAuthority)
	return authority
}

func (a *AutoRenewer) validateCertificateAuthority(authority CertificateAuthority) error {
	if strings.TrimSpace(authority.Name) == "" {
		return errors.New("Certificate authority name is required")
	}
	if _, err := mail.ParseAddress(strings.TrimSpace(authority.Email)); err != nil {
		return errors.New("Invalid ACME email: " + err.Error())
	}
	if authority.Type != "custom" && (a.AcmeHandler == nil || !IsSupportedCA(authority.Type, a.AcmeHandler.TestMode)) {
		return errors.New("The specified ACME CA is not supported")
	}
	if authority.Type == "custom" {
		parsedURL, err := url.Parse(strings.TrimSpace(authority.URL))
		if err != nil || (parsedURL.Scheme != "http" && parsedURL.Scheme != "https") || parsedURL.Host == "" {
			return errors.New("Invalid custom CA URL provided")
		}
	}
	return nil
}

// HandleCertificateAuthorities lists, creates, updates and removes reusable CA
// profiles stored in acme_conf.json.
func (a *AutoRenewer) HandleCertificateAuthorities(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		response := certificateAuthorityResponse{
			Authorities:      a.RenewerConfig.CertificateAuthorities,
			DefaultAuthority: a.RenewerConfig.DefaultCertificateAuthority,
			Assignments:      a.RenewerConfig.CertificateAuthorityAssignments,
		}
		js, _ := json.Marshal(response)
		utils.SendJSONResponse(w, string(js))
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "405 - Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	action, err := utils.PostPara(r, "action")
	if err != nil {
		utils.SendErrorResponse(w, "Action not set")
		return
	}
	switch action {
	case "save":
		name, _ := utils.PostPara(r, "name")
		caType, _ := utils.PostPara(r, "type")
		email, _ := utils.PostPara(r, "email")
		customURL, _ := utils.PostPara(r, "url")
		skipTLS, _ := utils.PostBool(r, "skipTLS")
		id, _ := utils.PostPara(r, "id")
		authority := CertificateAuthority{ID: id, Name: strings.TrimSpace(name), Type: caType, Email: strings.TrimSpace(email), URL: strings.TrimSpace(customURL), SkipTLS: skipTLS}
		if err := a.validateCertificateAuthority(authority); err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
		if authority.Type != "custom" {
			authority.URL = ""
			authority.SkipTLS = false
		}
		if authority.ID == "" {
			authority.ID = a.uniqueCAID(authority.Name)
			a.RenewerConfig.CertificateAuthorities = append(a.RenewerConfig.CertificateAuthorities, authority)
		} else {
			updated := false
			for i := range a.RenewerConfig.CertificateAuthorities {
				if a.RenewerConfig.CertificateAuthorities[i].ID == authority.ID {
					a.RenewerConfig.CertificateAuthorities[i] = authority
					updated = true
					break
				}
			}
			if !updated {
				utils.SendErrorResponse(w, "Certificate authority not found")
				return
			}
		}
		if setDefault, _ := utils.PostBool(r, "setDefault"); setDefault {
			a.RenewerConfig.DefaultCertificateAuthority = authority.ID
		}
		if err := a.saveRenewConfigToFile(); err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
		js, _ := json.Marshal(authority)
		utils.SendJSONResponse(w, string(js))
	case "delete":
		id, err := utils.PostPara(r, "id")
		if err != nil {
			utils.SendErrorResponse(w, "Certificate authority not set")
			return
		}
		if len(a.RenewerConfig.CertificateAuthorities) == 1 {
			utils.SendErrorResponse(w, "At least one certificate authority is required")
			return
		}
		for filename, assignedID := range a.RenewerConfig.CertificateAuthorityAssignments {
			if assignedID == id {
				utils.SendErrorResponse(w, "Certificate authority is assigned to "+filename)
				return
			}
		}
		found := false
		filtered := make([]CertificateAuthority, 0, len(a.RenewerConfig.CertificateAuthorities)-1)
		for _, authority := range a.RenewerConfig.CertificateAuthorities {
			if authority.ID == id {
				found = true
				continue
			}
			filtered = append(filtered, authority)
		}
		if !found {
			utils.SendErrorResponse(w, "Certificate authority not found")
			return
		}
		a.RenewerConfig.CertificateAuthorities = filtered
		if a.RenewerConfig.DefaultCertificateAuthority == id {
			a.RenewerConfig.DefaultCertificateAuthority = filtered[0].ID
		}
		if err := a.saveRenewConfigToFile(); err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
		if a.AcmeHandler != nil && a.AcmeHandler.Database != nil && a.AcmeHandler.Database.TableExists("acme") {
			eabKeyPrefix := eabProfileKeyPrefix(id)
			a.AcmeHandler.Database.Delete("acme", eabKeyPrefix+"_kid")
			a.AcmeHandler.Database.Delete("acme", eabKeyPrefix+"_hmacEncoded")
		}
		utils.SendOK(w)
	default:
		utils.SendErrorResponse(w, "Invalid action")
	}
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

	var filesToRenew []string
	if opr == "setSelected" {
		files, err := utils.PostPara(r, "domains")
		if err != nil {
			utils.SendErrorResponse(w, "Domains is not defined")
			return
		}

		//Parse it int array of string
		err = json.Unmarshal([]byte(files), &filesToRenew)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
	} else if opr != "setAuto" {
		utils.SendErrorResponse(w, "invalid operation given")
		return
	}

	assignments := a.RenewerConfig.CertificateAuthorityAssignments
	if assignmentsJSON, assignmentsErr := utils.PostPara(r, "certificateAuthorities"); assignmentsErr == nil {
		assignments = map[string]string{}
		if err := json.Unmarshal([]byte(assignmentsJSON), &assignments); err != nil {
			utils.SendErrorResponse(w, "Invalid certificate authority assignments: "+err.Error())
			return
		}
		for filename, id := range assignments {
			if _, ok := a.certificateAuthorityByID(id); !ok {
				utils.SendErrorResponse(w, "Unknown certificate authority for "+filename)
				return
			}
		}
	}

	a.RenewerConfig.RenewAll = opr == "setAuto"
	if opr == "setSelected" {
		a.RenewerConfig.FilesToRenew = filesToRenew
	}
	a.RenewerConfig.CertificateAuthorityAssignments = assignments
	if err := a.saveRenewConfigToFile(); err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	utils.SendOK(w)
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
	var renewedDomains []string
	var err error
	scope, _ := utils.GetPara(r, "scope")
	switch scope {
	case "all":
		renewedDomains, err = a.checkAndRenewCertificates(true, nil)
	case "selected":
		renewedDomains, err = a.checkAndRenewCertificates(false, a.RenewerConfig.FilesToRenew)
	default:
		renewedDomains, err = a.CheckAndRenewCertificates()
	}
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
			for _, authority := range a.RenewerConfig.CertificateAuthorities {
				if _, err := mail.ParseAddress(authority.Email); err != nil {
					utils.SendErrorResponse(w, "ACME email is not set for certificate authority "+authority.Name)
					return
				}
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
	defaultAuthority := a.CertificateAuthorityFor("")
	if r.Method == http.MethodGet {
		// Backward-compatible endpoint for older clients. Email is stored on the
		// default CA profile, never as a global setting.
		js, _ := json.Marshal(defaultAuthority.Email)
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

		for i := range a.RenewerConfig.CertificateAuthorities {
			if a.RenewerConfig.CertificateAuthorities[i].ID == defaultAuthority.ID {
				a.RenewerConfig.CertificateAuthorities[i].Email = email
				break
			}
		}
		if err := a.saveRenewConfigToFile(); err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		utils.SendOK(w)
	} else {
		http.Error(w, "405 - Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Check and renew certificates. This check all the certificates in the
// certificate folder and return a list of certs that is renewed in this call
// Return string array with length 0 when no cert is expired
func (a *AutoRenewer) CheckAndRenewCertificates() ([]string, error) {
	return a.checkAndRenewCertificates(a.RenewerConfig.RenewAll, a.RenewerConfig.FilesToRenew)
}

func (a *AutoRenewer) checkAndRenewCertificates(renewAll bool, filesToRenew []string) ([]string, error) {
	certFolder := a.CertFolder
	files, err := os.ReadDir(certFolder)
	if err != nil {
		a.Logf("Read certificate store failed", err)
		return []string{}, err
	}

	expiredCertList := []*ExpiredCerts{}
	if renewAll {
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
			if contains(filesToRenew, certName) {
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

		// The renew policy owns the CA choice. Certificate-specific DNS settings
		// remain sourced from the certificate metadata.
		authority := a.CertificateAuthorityFor(certName)
		_, err = a.AcmeHandler.ObtainCertWithProfile(expiredCert.Domains, certName, authority.Email, authority.Type, authority.URL, authority.SkipTLS, certInfo.UseDNS, certInfo.PropTimeout, dnsServers, authority.ID)
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

	eabKeyPrefix := acmeDirectoryURL
	if authorityID, authorityErr := utils.GetPara(r, "authorityID"); authorityErr == nil && authorityID != "" {
		if _, ok := a.certificateAuthorityByID(authorityID); !ok {
			utils.SendErrorResponse(w, "Certificate authority not found")
			return
		}
		eabKeyPrefix = eabProfileKeyPrefix(authorityID)
	}

	a.AcmeHandler.Database.Write("acme", eabKeyPrefix+"_kid", kid)
	a.AcmeHandler.Database.Write("acme", eabKeyPrefix+"_hmacEncoded", hmacEncoded)

	utils.SendOK(w)

}

func eabProfileKeyPrefix(authorityID string) string {
	return "ca_profile_" + authorityID
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
