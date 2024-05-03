package acme

import (
	"log"
	"os"
	"strings"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/providers/dns"
)

func GetDnsChallengeProviderByName(dnsProvider string, dnsCredentials string) (challenge.Provider, error) {
	credentials := extractDnsCredentials(dnsCredentials)
	setCredentialsIntoEnvironmentVariables(credentials)

	provider, err := dns.NewDNSChallengeProviderByName(dnsProvider)
	return provider, err
}

func setCredentialsIntoEnvironmentVariables(credentials map[string]string) {
	for key, value := range credentials {
		err := os.Setenv(key, value)
		if err != nil {
			log.Printf("Failed to set environment variable %s: %v", key, err)
		}
	}
}

func extractDnsCredentials(input string) map[string]string {
	result := make(map[string]string)

	// Split the input string by newline character
	lines := strings.Split(input, "\n")

	// Iterate over each line
	for _, line := range lines {
		// Split the line by "=" character
		parts := strings.Split(line, "=")

		// Check if the line is in the correct format
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			// Add the key-value pair to the map
			result[key] = value
		}
	}

	return result
}
