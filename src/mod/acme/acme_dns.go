package acme

import (
	"encoding/json"
	"strconv"

	"github.com/go-acme/lego/v4/challenge"
	"imuslab.com/zoraxy/mod/acme/acmedns"
)

// Preprocessor function to get DNS challenge provider by name
func GetDnsChallengeProviderByName(dnsProvider string, dnsCredentials string, ppgTimeout int) (challenge.Provider, error) {
	//Unpack the dnsCredentials (json string) to map
	var dnsCredentialsMap map[string]interface{}
	err := json.Unmarshal([]byte(dnsCredentials), &dnsCredentialsMap)
	if err != nil {
		return nil, err
	}

	//Clear the PollingInterval and PropagationTimeout field and conert to int
	userDefinedPollingInterval := 2
	if dnsCredentialsMap["PollingInterval"] != nil {
		userDefinedPollingIntervalRaw := dnsCredentialsMap["PollingInterval"].(string)
		delete(dnsCredentialsMap, "PollingInterval")
		convertedPollingInterval, err := strconv.Atoi(userDefinedPollingIntervalRaw)
		if err == nil {
			userDefinedPollingInterval = convertedPollingInterval
		}
	}

	userDefinedPropagationTimeout := ppgTimeout
	if dnsCredentialsMap["PropagationTimeout"] != nil {
		userDefinedPropagationTimeoutRaw := dnsCredentialsMap["PropagationTimeout"].(string)
		delete(dnsCredentialsMap, "PropagationTimeout")
		convertedPropagationTimeout, err := strconv.Atoi(userDefinedPropagationTimeoutRaw)
		if err == nil {
			//Overwrite the default propagation timeout if it is requeted from UI
			userDefinedPropagationTimeout = convertedPropagationTimeout
		}
	}

	//Restructure dnsCredentials string from map
	dnsCredentialsBytes, err := json.Marshal(dnsCredentialsMap)
	if err != nil {
		return nil, err
	}
	dnsCredentials = string(dnsCredentialsBytes)

	//Using acmedns CICD pipeline generated datatype to optain the DNS provider
	return acmedns.GetDNSProviderByJsonConfig(
		dnsProvider,
		dnsCredentials,
		int64(userDefinedPropagationTimeout),
		int64(userDefinedPollingInterval),
	)
}
