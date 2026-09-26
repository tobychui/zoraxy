// Package v335 provides migration logic from Zoraxy v.3.3.4 to v3.3.5.
//
// It migrates ACME certificate JSON files:
//   - Renames recursive_ns to disable_recursive_nss_check (inverted boolean)
//   - Adds disable_authoritative_nss_check (default false)
package v335

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// UpdateFrom334To335 migrates cert JSON files to the new NSs check field names.
func UpdateFrom334To335() error {
	certStore := "./conf/certs"

	jsonFiles, err := filepath.Glob(filepath.Join(certStore, "*.json"))
	if err != nil {
		return fmt.Errorf("failed to glob cert JSON files: %w", err)
	}

	for _, jsonFile := range jsonFiles {
		// Skip non-cert JSON files like fallback.json
		if filepath.Base(jsonFile) == "fallback.json" {
			continue
		}

		data, err := os.ReadFile(jsonFile)
		if err != nil {
			fmt.Printf("Warning: failed to read %s: %v\n", jsonFile, err)
			continue
		}

		var raw map[string]any
		if err := json.Unmarshal(data, &raw); err != nil {
			fmt.Printf("Warning: failed to parse %s: %v\n", jsonFile, err)
			continue
		}

		// Skip if already migrated
		if _, exists := raw["disable_recursive_nss_check"]; exists {
			continue
		}

		// Migrate old recursive_ns → disable_recursive_nss_check (inverted)
		if oldVal, ok := raw["recursive_ns"]; ok {
			// old recursive_ns: true (check enabled) → disable_recursive_nss_check: false
			// old recursive_ns: false (check disabled) → disable_recursive_nss_check: true
			if boolVal, ok := oldVal.(bool); ok {
				raw["disable_recursive_nss_check"] = !boolVal
			} else {
				// Default: disable recursive NSs check
				raw["disable_recursive_nss_check"] = true
			}
			delete(raw, "recursive_ns")
		} else {
			// No old field: default to disabled
			raw["disable_recursive_nss_check"] = true
		}

		// Add disable_authoritative_nss_check if missing (default false)
		if _, exists := raw["disable_authoritative_nss_check"]; !exists {
			raw["disable_authoritative_nss_check"] = false
		}

		newData, err := json.MarshalIndent(raw, "", "    ")
		if err != nil {
			fmt.Printf("Warning: failed to marshal %s: %v\n", jsonFile, err)
			continue
		}

		if err := os.WriteFile(jsonFile, newData, 0o644); err != nil {
			fmt.Printf("Warning: failed to write %s: %v\n", jsonFile, err)
			continue
		}

		fmt.Println("Migrated " + filepath.Base(jsonFile))
	}

	return nil
}

// ProbeNSCheckMigrationNeeded checks if any cert JSON file still contains the
// old "recursive_ns" key, indicating the v3.3.4→v3.3.5 migration has not run yet.
func ProbeNSCheckMigrationNeeded() bool {
	certStore := "./conf/certs"
	jsonFiles, err := filepath.Glob(filepath.Join(certStore, "*.json"))
	if err != nil {
		return false
	}
	for _, jsonFile := range jsonFiles {
		if filepath.Base(jsonFile) == "fallback.json" {
			continue
		}
		data, err := os.ReadFile(jsonFile)
		if err != nil {
			continue
		}
		if strings.Contains(string(data), `"recursive_ns"`) {
			return true
		}
	}
	return false
}
