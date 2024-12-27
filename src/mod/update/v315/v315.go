package v315

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"

	"imuslab.com/zoraxy/mod/update/updateutil"
)

func UpdateFrom314To315() error {
	//Load the configs
	oldConfigFiles, err := filepath.Glob("./conf/proxy/*.config")
	if err != nil {
		return err
	}

	//Backup all the files
	err = os.MkdirAll("./conf/proxy-314.old/", 0775)
	if err != nil {
		return err
	}

	for _, oldConfigFile := range oldConfigFiles {
		// Extract the file name from the path
		fileName := filepath.Base(oldConfigFile)
		// Construct the backup file path
		backupFile := filepath.Join("./conf/proxy-314.old/", fileName)

		// Copy the file to the backup directory
		err := updateutil.CopyFile(oldConfigFile, backupFile)
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

		thisOldConfigStruct := v314ProxyEndpoint{}
		err = json.Unmarshal(configContent, &thisOldConfigStruct)
		if err != nil {
			log.Println("Unable to parse file "+filepath.Base(oldConfigFile), err.Error())
			continue
		}

		//Convert the old struct to the new struct
		thisNewConfigStruct := convertV314ToV315(thisOldConfigStruct)

		//Write the new config to file
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

func convertV314ToV315(thisOldConfigStruct v314ProxyEndpoint) v315ProxyEndpoint {
	//Move old header and auth configs into struct
	newHeaderRewriteRules := HeaderRewriteRules{
		UserDefinedHeaders:           thisOldConfigStruct.UserDefinedHeaders,
		RequestHostOverwrite:         thisOldConfigStruct.RequestHostOverwrite,
		HSTSMaxAge:                   thisOldConfigStruct.HSTSMaxAge,
		EnablePermissionPolicyHeader: thisOldConfigStruct.EnablePermissionPolicyHeader,
		PermissionPolicy:             thisOldConfigStruct.PermissionPolicy,
		DisableHopByHopHeaderRemoval: thisOldConfigStruct.DisableHopByHopHeaderRemoval,
	}

	newAuthenticationProvider := AuthenticationProvider{
		RequireBasicAuth:        thisOldConfigStruct.RequireBasicAuth,
		BasicAuthCredentials:    thisOldConfigStruct.BasicAuthCredentials,
		BasicAuthExceptionRules: thisOldConfigStruct.BasicAuthExceptionRules,
	}

	//Convert proxy type int to enum
	var newConfigProxyType ProxyType
	if thisOldConfigStruct.ProxyType == 0 {
		newConfigProxyType = ProxyTypeRoot
	} else if thisOldConfigStruct.ProxyType == 1 {
		newConfigProxyType = ProxyTypeHost
	} else if thisOldConfigStruct.ProxyType == 2 {
		newConfigProxyType = ProxyTypeVdir
	}

	//Update the config struct
	thisNewConfigStruct := v315ProxyEndpoint{
		ProxyType:            newConfigProxyType,
		RootOrMatchingDomain: thisOldConfigStruct.RootOrMatchingDomain,
		MatchingDomainAlias:  thisOldConfigStruct.MatchingDomainAlias,
		ActiveOrigins:        thisOldConfigStruct.ActiveOrigins,
		InactiveOrigins:      thisOldConfigStruct.InactiveOrigins,
		UseStickySession:     thisOldConfigStruct.UseStickySession,
		UseActiveLoadBalance: thisOldConfigStruct.UseActiveLoadBalance,
		Disabled:             thisOldConfigStruct.Disabled,
		BypassGlobalTLS:      thisOldConfigStruct.BypassGlobalTLS,
		VirtualDirectories:   thisOldConfigStruct.VirtualDirectories,
		RequireRateLimit:     thisOldConfigStruct.RequireRateLimit,
		RateLimit:            thisOldConfigStruct.RateLimit,
		AccessFilterUUID:     thisOldConfigStruct.AccessFilterUUID,
		DefaultSiteOption:    thisOldConfigStruct.DefaultSiteOption,
		DefaultSiteValue:     thisOldConfigStruct.DefaultSiteValue,

		//Append the new struct into the new config
		HeaderRewriteRules:     &newHeaderRewriteRules,
		AuthenticationProvider: &newAuthenticationProvider,
	}

	return thisNewConfigStruct
}
