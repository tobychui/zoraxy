package tlscert

import (
	"path/filepath"
	"strings"
)

// This remove the certificates in the list where either the
// public key or the private key is missing
func getCertPairs(certFiles []string) []string {
	crtMap := make(map[string]bool)
	keyMap := make(map[string]bool)

	for _, filename := range certFiles {
		switch filepath.Ext(filename) {
		case ".crt":
			crtMap[strings.TrimSuffix(filename, ".crt")] = true
		case ".key":
			keyMap[strings.TrimSuffix(filename, ".key")] = true
		default:
			continue
		}
	}

	var result []string
	for domain := range crtMap {
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

// Check if a requesting domain is a subdomain of a given domain
func isSubdomain(subdomain, domain string) bool {
	subdomainParts := strings.Split(subdomain, ".")
	domainParts := strings.Split(domain, ".")
	if len(subdomainParts) < len(domainParts) {
		return false
	}
	for i := range domainParts {
		if subdomainParts[len(subdomainParts)-1-i] != domainParts[len(domainParts)-1-i] {
			return false
		}
	}
	return true
}
