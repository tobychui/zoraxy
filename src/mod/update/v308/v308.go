package v308

import (
	"encoding/json"
	"io"
	"log"
	"os"
	"path/filepath"
)

/*

	v3.0.7 update to v3.0.8

	This update function
*/

// Update proxy config files from v3.0.7 to v3.0.8
func UpdateFrom307To308() error {

	//Load the configs
	oldConfigFiles, err := filepath.Glob("./conf/proxy/*.config")
	if err != nil {
		return err
	}

	//Backup all the files
	err = os.MkdirAll("./conf/proxy.old/", 0775)
	if err != nil {
		return err
	}

	for _, oldConfigFile := range oldConfigFiles {
		// Extract the file name from the path
		fileName := filepath.Base(oldConfigFile)
		// Construct the backup file path
		backupFile := filepath.Join("./conf/proxy.old/", fileName)

		// Copy the file to the backup directory
		err := copyFile(oldConfigFile, backupFile)
		if err != nil {
			return err
		}
	}

	//read the config into the old struct
	for _, oldConfigFile := range oldConfigFiles {
		configContent, err := os.ReadFile(oldConfigFile)
		if err != nil {
			log.Println("Unable to read config file "+filepath.Base(oldConfigFile), err.Error())
			continue
		}

		thisOldConfigStruct := v307ProxyEndpoint{}
		err = json.Unmarshal(configContent, &thisOldConfigStruct)
		if err != nil {
			log.Println("Unable to parse file "+filepath.Base(oldConfigFile), err.Error())
			continue
		}

		//Convert the old config to new config
		newProxyStructure := convertV307ToV308(thisOldConfigStruct)
		js, _ := json.MarshalIndent(newProxyStructure, "", "    ")
		err = os.WriteFile(oldConfigFile, js, 0775)
		if err != nil {
			log.Println(err.Error())
			continue
		}
	}

	return nil
}

func convertV307ToV308(old v307ProxyEndpoint) v308ProxyEndpoint {
	// Create a new v308ProxyEndpoint instance

	matchingDomainsSlice := old.MatchingDomainAlias
	if matchingDomainsSlice == nil {
		matchingDomainsSlice = []string{}
	}

	newEndpoint := v308ProxyEndpoint{
		ProxyType:            old.ProxyType,
		RootOrMatchingDomain: old.RootOrMatchingDomain,
		MatchingDomainAlias:  matchingDomainsSlice,
		ActiveOrigins: []*v308Upstream{{ // Mapping Domain field to v308Upstream struct
			OriginIpOrDomain:         old.Domain,
			RequireTLS:               old.RequireTLS,
			SkipCertValidations:      old.SkipCertValidations,
			SkipWebSocketOriginCheck: old.SkipWebSocketOriginCheck,
			Weight:                   1,
			MaxConn:                  0,
		}},
		InactiveOrigins:              []*v308Upstream{},
		UseStickySession:             false,
		Disabled:                     old.Disabled,
		BypassGlobalTLS:              old.BypassGlobalTLS,
		VirtualDirectories:           old.VirtualDirectories,
		UserDefinedHeaders:           old.UserDefinedHeaders,
		HSTSMaxAge:                   old.HSTSMaxAge,
		EnablePermissionPolicyHeader: old.EnablePermissionPolicyHeader,
		PermissionPolicy:             old.PermissionPolicy,
		RequireBasicAuth:             old.RequireBasicAuth,
		BasicAuthCredentials:         old.BasicAuthCredentials,
		BasicAuthExceptionRules:      old.BasicAuthExceptionRules,
		RequireRateLimit:             old.RequireRateLimit,
		RateLimit:                    old.RateLimit,
		AccessFilterUUID:             old.AccessFilterUUID,
		DefaultSiteOption:            old.DefaultSiteOption,
		DefaultSiteValue:             old.DefaultSiteValue,
	}

	return newEndpoint
}

// Helper function to copy files
func copyFile(src, dst string) error {
	sourceFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sourceFile.Close()

	destinationFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer destinationFile.Close()

	_, err = io.Copy(destinationFile, sourceFile)
	return err
}
