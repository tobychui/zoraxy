package acme

import (
	"github.com/go-acme/lego/v4/challenge"
	"imuslab.com/zoraxy/mod/acme/acmedns"
)

func GetDnsChallengeProviderByName(dnsProvider string, dnsCredentials string) (challenge.Provider, error) {

	//Original Implementation
	/*credentials, err := extractDnsCredentials(dnsCredentials)
	if err != nil {
		return nil, err
	}
	setCredentialsIntoEnvironmentVariables(credentials)

	provider, err := dns.NewDNSChallengeProviderByName(dnsProvider)
	*/

	//New implementation using acmedns CICD pipeline generated datatype
	return acmedns.GetDNSProviderByJsonConfig(dnsProvider, dnsCredentials)
}

/*
	Original implementation of DNS ACME using OS.Env as payload
*/
/*
func setCredentialsIntoEnvironmentVariables(credentials map[string]string) {
	for key, value := range credentials {
		err := os.Setenv(key, value)
		if err != nil {
			log.Println("[ERR] Failed to set environment variable %s: %v", key, err)
		} else {
			log.Println("[INFO] Environment variable %s set successfully", key)
		}
	}
}


func extractDnsCredentials(input string) (map[string]string, error) {
	result := make(map[string]string)

	// Split the input string by newline character
	lines := strings.Split(input, "\n")

	// Iterate over each line
	for _, line := range lines {
		// Split the line by "=" character
		//use SpliyN to make sure not to split the value if the value is base64
		parts := strings.SplitN(line, "=", 1)

		// Check if the line is in the correct format
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			// Add the key-value pair to the map
			result[key] = value

			if value == "" || key == "" {
				//invalid config
				return result, errors.New("DNS credential extract failed")
			}
		}
	}

	return result, nil
}

*/
