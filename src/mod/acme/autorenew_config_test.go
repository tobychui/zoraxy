package acme

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	legoacme "github.com/go-acme/lego/v5/acme"
	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/database/dbinc"
	"imuslab.com/zoraxy/mod/info/logger"
)

func TestAutoRenewConfigMigrationMovesEmailIntoDefaultCA(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "acme_conf.json")
	legacyConfig := []byte(`{
 "Enabled": false,
 "Email": "admin@example.com",
 "RenewAll": true,
 "FilesToRenew": [],
 "DNSServers": ""
}`)
	if err := os.WriteFile(configPath, legacyConfig, 0600); err != nil {
		t.Fatal(err)
	}
	testLogger, _ := logger.NewFmtLogger()
	renewer, err := NewAutoRenewer(configPath, tempDir, 86400, 30, &ACMEHandler{}, testLogger)
	if err != nil {
		t.Fatal(err)
	}

	if len(renewer.RenewerConfig.CertificateAuthorities) != 1 {
		t.Fatalf("expected one migrated CA, got %d", len(renewer.RenewerConfig.CertificateAuthorities))
	}
	authority := renewer.RenewerConfig.CertificateAuthorities[0]
	if authority.Type != "Let's Encrypt" || authority.Email != "admin@example.com" {
		t.Fatalf("unexpected migrated authority: %#v", authority)
	}
	if renewer.RenewerConfig.DefaultCertificateAuthority != authority.ID {
		t.Fatal("migrated authority was not made the default")
	}

	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(content, &stored); err != nil {
		t.Fatal(err)
	}
	if _, exists := stored["Email"]; exists {
		t.Fatal("legacy global Email field was not removed")
	}
	if _, exists := stored["CertificateAuthorities"]; !exists {
		t.Fatal("migrated CertificateAuthorities field was not saved")
	}
}

func TestCertificateAuthorityForUsesAssignmentAndDefault(t *testing.T) {
	renewer := &AutoRenewer{RenewerConfig: &AutoRenewConfig{
		CertificateAuthorities: []CertificateAuthority{
			{ID: "public", Name: "Public", Type: "Let's Encrypt", Email: "public@example.com"},
			{ID: "internal", Name: "Internal", Type: "custom", Email: "internal@example.com", URL: "https://ca.example/acme"},
		},
		DefaultCertificateAuthority:     "public",
		CertificateAuthorityAssignments: map[string]string{"internal.example.com": "internal"},
	}}

	if got := renewer.CertificateAuthorityFor("internal.example.com"); got.ID != "internal" {
		t.Fatalf("expected assigned CA, got %#v", got)
	}
	if got := renewer.CertificateAuthorityFor("public.example.com"); got.ID != "public" {
		t.Fatalf("expected default CA, got %#v", got)
	}
}

func TestAutoRenewConfigMigrationPreservesCertificateIssuer(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "acme_conf.json")
	legacyConfig := []byte(`{
 "Enabled": false,
 "Email": "admin@example.com",
 "RenewAll": true,
 "FilesToRenew": [],
 "DNSServers": ""
}`)
	if err := os.WriteFile(configPath, legacyConfig, 0600); err != nil {
		t.Fatal(err)
	}
	certificateInfo := []byte(`{
 "acme_name": "custom",
 "acme_url": "https://internal-ca.example/acme/directory",
 "skip_tls": true
}`)
	if err := os.WriteFile(filepath.Join(tempDir, "internal.example.com.json"), certificateInfo, 0600); err != nil {
		t.Fatal(err)
	}

	testLogger, _ := logger.NewFmtLogger()
	renewer, err := NewAutoRenewer(configPath, tempDir, 86400, 30, &ACMEHandler{}, testLogger)
	if err != nil {
		t.Fatal(err)
	}

	authority := renewer.CertificateAuthorityFor("internal.example.com")
	if authority.Type != "custom" || authority.URL != "https://internal-ca.example/acme/directory" || !authority.SkipTLS {
		t.Fatalf("legacy certificate issuer was not preserved: %#v", authority)
	}
	if authority.ID == renewer.RenewerConfig.DefaultCertificateAuthority {
		t.Fatal("custom certificate was incorrectly assigned to the global default authority")
	}
}

func TestEABProfileKeyPrefixIsProfileSpecific(t *testing.T) {
	first := eabProfileKeyPrefix("zerossl-primary")
	second := eabProfileKeyPrefix("zerossl-secondary")
	if first == second {
		t.Fatal("different CA profiles must not share EAB storage keys")
	}
}

func TestAccountDBKeyIsProfileSpecific(t *testing.T) {
	first := accountDBKey("https://acme.example/directory", "admin@example.com", "primary")
	second := accountDBKey("https://acme.example/directory", "admin@example.com", "secondary")
	if first == second {
		t.Fatal("different CA profiles must not share ACME account storage keys")
	}
}

func TestLegacyACMEStateMigratesOnlyToDefaultProfile(t *testing.T) {
	tempDir := t.TempDir()
	db, err := database.NewDatabase(filepath.Join(tempDir, "system.db"), dbinc.BackendLevelDB)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.NewTable(acmeAccountTable); err != nil {
		t.Fatal(err)
	}
	if err := db.NewTable("acme"); err != nil {
		t.Fatal(err)
	}

	directoryURL := "https://acme.example/directory"
	email := "admin@example.com"
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	legacyUser := &ACMEUser{
		Email:        email,
		Registration: &legoacme.ExtendedAccount{Location: "https://acme.example/account/1"},
		key:          privateKey,
	}
	if err := db.Write(acmeAccountTable, legacyAccountDBKey(directoryURL, email), legacyUser); err != nil {
		t.Fatal(err)
	}
	if err := db.Write("acme", directoryURL+"_kid", "legacy-kid"); err != nil {
		t.Fatal(err)
	}
	if err := db.Write("acme", directoryURL+"_hmacEncoded", "legacy-hmac"); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(tempDir, "acme_conf.json")
	config := AutoRenewConfig{
		RenewAll: true,
		CertificateAuthorities: []CertificateAuthority{
			{ID: "primary", Name: "Primary", Type: "custom", Email: email, URL: directoryURL},
			{ID: "secondary", Name: "Secondary", Type: "custom", Email: email, URL: directoryURL},
		},
		DefaultCertificateAuthority:     "primary",
		CertificateAuthorityAssignments: map[string]string{},
	}
	content, err := json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, content, 0600); err != nil {
		t.Fatal(err)
	}

	testLogger, _ := logger.NewFmtLogger()
	handler := &ACMEHandler{Database: db, Logger: testLogger}
	if _, err := NewAutoRenewer(configPath, tempDir, 86400, 30, handler, testLogger); err != nil {
		t.Fatal(err)
	}

	primaryAccountKey := accountDBKey(directoryURL, email, "primary")
	secondaryAccountKey := accountDBKey(directoryURL, email, "secondary")
	if !db.KeyExists(acmeAccountTable, primaryAccountKey) {
		t.Fatal("legacy account was not migrated to the default profile")
	}
	if db.KeyExists(acmeAccountTable, secondaryAccountKey) {
		t.Fatal("legacy account was shared with a second profile")
	}
	if !db.KeyExists("acme", eabProfileKeyPrefix("primary")+"_kid") {
		t.Fatal("legacy EAB credentials were not migrated to the default profile")
	}
	if db.KeyExists("acme", eabProfileKeyPrefix("secondary")+"_kid") {
		t.Fatal("legacy EAB credentials were shared with a second profile")
	}
	if _, _, ok := handler.loadACMEAccount(directoryURL, email, "secondary"); ok {
		t.Fatal("profile without its own account fell back to the legacy account")
	}

	// Changing the default later must not let another profile claim the same
	// legacy identity on the next startup.
	config.DefaultCertificateAuthority = "secondary"
	content, err = json.Marshal(config)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, content, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := NewAutoRenewer(configPath, tempDir, 86400, 30, handler, testLogger); err != nil {
		t.Fatal(err)
	}
	if db.KeyExists(acmeAccountTable, secondaryAccountKey) {
		t.Fatal("legacy account was shared after changing the default profile")
	}
	if db.KeyExists("acme", eabProfileKeyPrefix("secondary")+"_kid") {
		t.Fatal("legacy EAB credentials were shared after changing the default profile")
	}
}
