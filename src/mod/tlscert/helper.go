package tlscert

import (
	"path/filepath"
	"strings"
)

// This remove the certificates in the list where either the
// public key or the private key is missing
func getCertPairs(certFiles []string) []string {
	pemMap := make(map[string]bool)
	keyMap := make(map[string]bool)

	for _, filename := range certFiles {
		if filepath.Ext(filename) == ".pem" {
			pemMap[strings.TrimSuffix(filename, ".pem")] = true
		} else if filepath.Ext(filename) == ".key" {
			keyMap[strings.TrimSuffix(filename, ".key")] = true
		}
	}

	var result []string
	for domain := range pemMap {
		if keyMap[domain] {
			result = append(result, domain)
		}
	}

	return result
}

// Convert a domain name to a filename format
func domainToFilename(domain string, ext string) string {
	// Replace wildcard '*' with '_'
	domain = strings.TrimSpace(domain)
	if strings.HasPrefix(domain, "*") {
		domain = "_" + strings.TrimPrefix(domain, "*")
	}

	if strings.HasPrefix(".", ext) {
		ext = strings.TrimPrefix(ext, ".")
	}

	// Add .pem extension
	ext = strings.TrimPrefix(ext, ".") // Ensure ext does not start with a dot
	return domain + "." + ext
}

func filenameToDomain(filename string) string {
	// Remove the extension
	ext := filepath.Ext(filename)
	if ext != "" {
		filename = strings.TrimSuffix(filename, ext)
	}

	if strings.HasPrefix(filename, "_") {
		filename = "*" + filename[1:]
	}

	return filename
}
