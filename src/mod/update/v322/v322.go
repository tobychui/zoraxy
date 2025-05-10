package v322

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"imuslab.com/zoraxy/mod/dynamicproxy/permissionpolicy"
	"imuslab.com/zoraxy/mod/dynamicproxy/rewrite"
	"imuslab.com/zoraxy/mod/update/updateutil"
)

// UpdateFrom321To322 updates proxy config files from v3.2.1 to v3.2.2
func UpdateFrom321To322() error {
	// Load the configs
	oldConfigFiles, err := filepath.Glob("./conf/proxy/*.config")
	if err != nil {
		return err
	}

	// Backup all the files
	err = os.MkdirAll("./conf/proxy-321.old/", 0775)
	if err != nil {
		return err
	}

	for _, oldConfigFile := range oldConfigFiles {
		// Extract the file name from the path
		fileName := filepath.Base(oldConfigFile)
		// Construct the backup file path
		backupFile := filepath.Join("./conf/proxy-321.old/", fileName)

		// Copy the file to the backup directory
		err := updateutil.CopyFile(oldConfigFile, backupFile)
		if err != nil {
			return err
		}
	}

	// Read the config into the old struct
	for _, oldConfigFile := range oldConfigFiles {
		configContent, err := os.ReadFile(oldConfigFile)
		if err != nil {
			log.Println("Unable to read config file "+filepath.Base(oldConfigFile), err.Error())
			continue
		}

		thisOldConfigStruct := ProxyEndpointv321{}
		err = json.Unmarshal(configContent, &thisOldConfigStruct)
		if err != nil {
			log.Println("Unable to parse file "+filepath.Base(oldConfigFile), err.Error())
			continue
		}

		// Convert the old struct to the new struct
		thisNewConfigStruct := convertV321ToV322(thisOldConfigStruct)

		// Write the new config to file
		newConfigContent, err := json.MarshalIndent(thisNewConfigStruct, "", "    ")
		if err != nil {
			log.Println("Unable to marshal new config "+filepath.Base(oldConfigFile), err.Error())
			continue
		}

		err = os.WriteFile(oldConfigFile, newConfigContent, 0664)
		if err != nil {
			log.Println("Unable to write new config "+filepath.Base(oldConfigFile), err.Error())
			continue
		}
	}

	return nil
}

func convertV321ToV322(thisOldConfigStruct ProxyEndpointv321) ProxyEndpointv322 {
	// Merge both Authelia and Authentik into the Forward Auth provider, and remove the old provider configs
	if thisOldConfigStruct.AuthenticationProvider == nil {
		//Configs before v3.1.7 with no authentication provider
		// Set the default authentication provider
		thisOldConfigStruct.AuthenticationProvider = &AuthenticationProvider{
			AuthMethod:              AuthMethodNone, // Default to no authentication
			BasicAuthCredentials:    []*BasicAuthCredentials{},
			BasicAuthExceptionRules: []*BasicAuthExceptionRule{},
			BasicAuthGroupIDs:       []string{},
			AutheliaURL:             "",
			UseHTTPS:                false,
		}
	} else {
		//Override the old authentication provider with the new one
		if thisOldConfigStruct.AuthenticationProvider.AuthMethod == AuthMethodAuthelia {
			thisOldConfigStruct.AuthenticationProvider.AuthMethod = 2
		} else if thisOldConfigStruct.AuthenticationProvider.AuthMethod == AuthMethodAuthentik {
			thisOldConfigStruct.AuthenticationProvider.AuthMethod = 2
		}

	}

	if thisOldConfigStruct.AuthenticationProvider.BasicAuthGroupIDs == nil {
		//Create an empty basic auth group IDs array if it does not exist
		thisOldConfigStruct.AuthenticationProvider.BasicAuthGroupIDs = []string{}
	}

	newAuthenticationProvider := AuthenticationProviderV322{
		AuthMethod: AuthMethodNone, // Default to no authentication
		//Fill in the empty arrays
		BasicAuthCredentials:              []*BasicAuthCredentials{},
		BasicAuthExceptionRules:           []*BasicAuthExceptionRule{},
		BasicAuthGroupIDs:                 []string{},
		ForwardAuthURL:                    "",
		ForwardAuthResponseHeaders:        []string{},
		ForwardAuthResponseClientHeaders:  []string{},
		ForwardAuthRequestHeaders:         []string{},
		ForwardAuthRequestExcludedCookies: []string{},
	}

	// In theory the old config should have a matching itoa value that
	// can be converted to the new config
	js, err := json.Marshal(thisOldConfigStruct.AuthenticationProvider)
	if err != nil {
		fmt.Println("Unable to marshal authentication provider "+thisOldConfigStruct.RootOrMatchingDomain, err.Error())
		fmt.Println("Using default authentication provider")
	}

	err = json.Unmarshal(js, &newAuthenticationProvider)
	if err != nil {
		fmt.Println("Unable to unmarshal authentication provider "+thisOldConfigStruct.RootOrMatchingDomain, err.Error())
		fmt.Println("Using default authentication provider")
	} else {
		fmt.Println("Authentication provider for " + thisOldConfigStruct.RootOrMatchingDomain + " updated")
	}

	// Fill in any null values in the old config struct
	// these are non-upgrader requires values that updates between v3.1.5 to v3.2.1
	// will be in null state if not set by the user
	if thisOldConfigStruct.VirtualDirectories == nil {
		//Create an empty virtual directories array if it does not exist
		thisOldConfigStruct.VirtualDirectories = []*VirtualDirectoryEndpoint{}
	}

	if thisOldConfigStruct.HeaderRewriteRules == nil {
		//Create an empty header rewrite rules array if it does not exist
		thisOldConfigStruct.HeaderRewriteRules = &HeaderRewriteRules{
			UserDefinedHeaders:           []*rewrite.UserDefinedHeader{},
			RequestHostOverwrite:         "",
			HSTSMaxAge:                   0,
			EnablePermissionPolicyHeader: false,
			PermissionPolicy:             permissionpolicy.GetDefaultPermissionPolicy(),
			DisableHopByHopHeaderRemoval: false,
		}
	}

	if thisOldConfigStruct.Tags == nil {
		//Create an empty tags array if it does not exist
		thisOldConfigStruct.Tags = []string{}
	}

	if thisOldConfigStruct.MatchingDomainAlias == nil {
		//Create an empty matching domain alias array if it does not exist
		thisOldConfigStruct.MatchingDomainAlias = []string{}
	}

	// Update the config struct
	thisNewConfigStruct := ProxyEndpointv322{
		ProxyType:                    thisOldConfigStruct.ProxyType,
		RootOrMatchingDomain:         thisOldConfigStruct.RootOrMatchingDomain,
		MatchingDomainAlias:          thisOldConfigStruct.MatchingDomainAlias,
		ActiveOrigins:                thisOldConfigStruct.ActiveOrigins,
		InactiveOrigins:              thisOldConfigStruct.InactiveOrigins,
		UseStickySession:             thisOldConfigStruct.UseStickySession,
		UseActiveLoadBalance:         thisOldConfigStruct.UseActiveLoadBalance,
		Disabled:                     thisOldConfigStruct.Disabled,
		BypassGlobalTLS:              thisOldConfigStruct.BypassGlobalTLS,
		VirtualDirectories:           thisOldConfigStruct.VirtualDirectories,
		HeaderRewriteRules:           thisOldConfigStruct.HeaderRewriteRules,
		EnableWebsocketCustomHeaders: thisOldConfigStruct.EnableWebsocketCustomHeaders,
		RequireRateLimit:             thisOldConfigStruct.RequireRateLimit,
		RateLimit:                    thisOldConfigStruct.RateLimit,
		DisableUptimeMonitor:         thisOldConfigStruct.DisableUptimeMonitor,
		AccessFilterUUID:             thisOldConfigStruct.AccessFilterUUID,
		DefaultSiteOption:            thisOldConfigStruct.DefaultSiteOption,
		DefaultSiteValue:             thisOldConfigStruct.DefaultSiteValue,
		Tags:                         thisOldConfigStruct.Tags,
	}

	// Set the new authentication provider
	thisNewConfigStruct.AuthenticationProvider = &newAuthenticationProvider

	return thisNewConfigStruct
}
