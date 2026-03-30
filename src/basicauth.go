package main

import (
	"net/http"
	"sort"
	"strings"

	"imuslab.com/zoraxy/mod/auth"
	"imuslab.com/zoraxy/mod/dynamicproxy"
	"imuslab.com/zoraxy/mod/utils"
)

func migrateLegacyBasicAuthConfigs() {
	if basicAuthManager == nil || dynamicProxyRouter == nil || dynamicProxyRouter.ProxyEndpoints == nil {
		return
	}

	dynamicProxyRouter.ProxyEndpoints.Range(func(key, value interface{}) bool {
		endpoint, ok := value.(*dynamicproxy.ProxyEndpoint)
		if !ok || endpoint == nil || endpoint.AuthenticationProvider == nil {
			return true
		}
		if endpoint.ProxyType != dynamicproxy.ProxyTypeHost || endpoint.AuthenticationProvider.AuthMethod != dynamicproxy.AuthMethodBasic {
			return true
		}
		if len(endpoint.AuthenticationProvider.BasicAuthCredentials) == 0 || len(endpoint.AuthenticationProvider.BasicAuthGroupIDs) > 0 {
			return true
		}

		for _, credential := range endpoint.AuthenticationProvider.BasicAuthCredentials {
			if credential == nil {
				continue
			}
			if err := basicAuthManager.ImportLegacyCredential(credential.Username, credential.PasswordHash, "default"); err != nil {
				if SystemWideLogger != nil {
					SystemWideLogger.PrintAndLog("basic-auth", "Unable to auto-migrate legacy credentials for "+endpoint.RootOrMatchingDomain, err)
				}
				return true
			}
		}

		endpoint.AuthenticationProvider.BasicAuthGroupIDs = []string{"default"}
		endpoint.AuthenticationProvider.BasicAuthCredentials = []*dynamicproxy.BasicAuthCredentials{}
		if err := SaveReverseProxyConfig(endpoint); err != nil {
			if SystemWideLogger != nil {
				SystemWideLogger.PrintAndLog("basic-auth", "Unable to persist migrated basic auth config for "+endpoint.RootOrMatchingDomain, err)
			}
			return true
		}
		endpoint.UpdateToRuntime()
		if SystemWideLogger != nil {
			SystemWideLogger.PrintAndLog("basic-auth", "Migrated legacy host-local basic auth credentials for "+endpoint.RootOrMatchingDomain+" into the default group", nil)
		}

		return true
	})
}

func RegisterBasicAuthAPIs(authRouter *auth.RouterDef) {
	if basicAuthManager == nil {
		return
	}

	authRouter.HandleFunc("/api/basicauth/groups/list", basicAuthManager.HandleGroupList)
	authRouter.HandleFunc("/api/basicauth/groups/create", basicAuthManager.HandleGroupCreate)
	authRouter.HandleFunc("/api/basicauth/groups/update", basicAuthManager.HandleGroupUpdate)
	authRouter.HandleFunc("/api/basicauth/groups/delete", basicAuthManager.HandleGroupDelete)
	authRouter.HandleFunc("/api/basicauth/users/list", basicAuthManager.HandleUserList)
	authRouter.HandleFunc("/api/basicauth/users/create", basicAuthManager.HandleUserCreate)
	authRouter.HandleFunc("/api/basicauth/users/update", basicAuthManager.HandleUserUpdate)
	authRouter.HandleFunc("/api/basicauth/users/delete", basicAuthManager.HandleUserDelete)
}

func resolveBasicAuthGroupNames(groupIDs []string) []string {
	if basicAuthManager == nil {
		return append([]string{}, groupIDs...)
	}

	groupSummaries := basicAuthManager.ListGroups()
	groupNames := map[string]string{}
	for _, group := range groupSummaries {
		groupNames[group.ID] = group.Name
	}

	results := make([]string, 0, len(groupIDs))
	for _, groupID := range groupIDs {
		groupID = strings.TrimSpace(groupID)
		if groupID == "" {
			continue
		}
		if groupName, ok := groupNames[groupID]; ok {
			results = append(results, groupName)
		} else {
			results = append(results, groupID)
		}
	}

	sort.Strings(results)
	return results
}

func requireConfiguredBasicAuthGroups(groupIDs []string) []string {
	if len(groupIDs) == 0 {
		return []string{"default"}
	}
	return groupIDs
}

func handleBasicAuthGroupOptions(w http.ResponseWriter, r *http.Request) {
	if basicAuthManager == nil {
		utils.SendJSONResponse(w, "[]")
		return
	}
	basicAuthManager.HandleGroupList(w, r)
}
