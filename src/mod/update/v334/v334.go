// Package v334 provides migration logic from Zoraxy v3.3.3 to v3.3.4.
//
// It renames legacy default.pem / default.key / default.json files
// back to their original certificate name (based on CN) and writes
// fallback.json for the new name-based fallback system.
package v334

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"imuslab.com/zoraxy/mod/utils"
)

// UpdateFrom333To334 migrates legacy default.* certificate files.
// In older versions, setting a cert as default renamed it to default.pem/key/json.
// This migration renames them back to their original name (based on CN in the cert)
// and writes a fallback.json metadata file for the new name-based approach.
func UpdateFrom333To334() error {
	certStore := "./conf/certs"

	// Check if the previous default cert exists. If yes, get its hostname from cert contents
	defaultPubKey := filepath.Join(certStore, "default.pem")
	defaultPriKey := filepath.Join(certStore, "default.key")
	defaultJSON := filepath.Join(certStore, "default.json")

	if !utils.FileExists(defaultPubKey) || !utils.FileExists(defaultPriKey) {
		return nil
	}

	// Move the existing default cert to its original name
	certBytes, err := os.ReadFile(defaultPubKey)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", defaultPubKey, err)
	}

	block, _ := pem.Decode(certBytes)
	if block == nil {
		return fmt.Errorf("failed to decode PEM in %s", defaultPubKey)
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate in %s: %w", defaultPubKey, err)
	}

	originalName := cert.Subject.CommonName
	originalName = strings.TrimSpace(originalName)
	if trimDomain, ok := strings.CutPrefix(originalName, "*"); ok {
		originalName = "_" + trimDomain
	}
	originalPemName := filepath.Join(certStore, originalName+".pem")
	originalKeyName := filepath.Join(certStore, originalName+".key")
	originalJSONName := filepath.Join(certStore, originalName+".json")

	err = os.Rename(defaultPubKey, originalPemName)
	if err != nil {
		return fmt.Errorf("failed to rename %s to %s", defaultPubKey, originalPemName)
	}
	fmt.Println("Migrated " + defaultPubKey + " → " + originalPemName)

	err = os.Rename(defaultPriKey, originalKeyName)
	if err != nil {
		return fmt.Errorf("failed to rename %s to %s", defaultPriKey, originalKeyName)
	}
	fmt.Println("Migrated " + defaultPriKey + " → " + originalKeyName)

	if utils.FileExists(defaultJSON) {
		err = os.Rename(defaultJSON, originalJSONName)
		if err != nil {
			return fmt.Errorf("failed to rename %s to %s", defaultJSON, originalJSONName)
		}
		fmt.Println("Migrated " + defaultJSON + " → " + originalJSONName)
	}

	fbPath := filepath.Join(certStore, "fallback.json")
	fbData, err := json.Marshal(map[string]string{"fallbackCert": originalName})
	if err != nil {
		return fmt.Errorf("failed to marshal fallback config: %w", err)
	}
	if err := os.WriteFile(fbPath, fbData, 0o644); err != nil {
		return fmt.Errorf("failed to write %s: %w", fbPath, err)
	}
	fmt.Println("Wrote " + fbPath + " with fallbackCert: " + originalName)

	return nil
}
