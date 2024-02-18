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

// Get the cloest subdomain certificate from a list of domains
func matchClosestDomainCertificate(subdomain string, domains []string) string {
	var matchingDomain string = ""
	maxLength := 0

	for _, domain := range domains {
		if strings.HasSuffix(subdomain, "."+domain) && len(domain) > maxLength {
			matchingDomain = domain
			maxLength = len(domain)
		}
	}

	return matchingDomain
}
