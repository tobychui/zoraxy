package geodb

import "strings"

/*
	Whitelist.go

	This script handles whitelist related functions
*/

//Geo Whitelist

func (s *Store) AddCountryCodeToWhitelist(countryCode string) {
	countryCode = strings.ToLower(countryCode)
	s.sysdb.Write("whitelist-cn", countryCode, true)
}

func (s *Store) RemoveCountryCodeFromWhitelist(countryCode string) {
	countryCode = strings.ToLower(countryCode)
	s.sysdb.Delete("whitelist-cn", countryCode)
}

func (s *Store) IsCountryCodeWhitelisted(countryCode string) bool {
	countryCode = strings.ToLower(countryCode)
	var isWhitelisted bool = false
	s.sysdb.Read("whitelist-cn", countryCode, &isWhitelisted)
	return isWhitelisted
}

func (s *Store) GetAllWhitelistedCountryCode() []string {
	whitelistedCountryCode := []string{}
	entries, err := s.sysdb.ListTable("whitelist-cn")
	if err != nil {
		return whitelistedCountryCode
	}
	for _, keypairs := range entries {
		ip := string(keypairs[0])
		whitelistedCountryCode = append(whitelistedCountryCode, ip)
	}

	return whitelistedCountryCode
}

//IP Whitelist

func (s *Store) AddIPToWhiteList(ipAddr string) {
	s.sysdb.Write("whitelist-ip", ipAddr, true)
}

func (s *Store) RemoveIPFromWhiteList(ipAddr string) {
	s.sysdb.Delete("whitelist-ip", ipAddr)
}

func (s *Store) IsIPWhitelisted(ipAddr string) bool {
	var isWhitelisted bool = false
	s.sysdb.Read("whitelist-ip", ipAddr, &isWhitelisted)
	if isWhitelisted {
		return true
	}

	//Check for IP wildcard and CIRD rules
	AllWhitelistedIps := s.GetAllWhitelistedIp()
	for _, whitelistRules := range AllWhitelistedIps {
		wildcardMatch := MatchIpWildcard(ipAddr, whitelistRules)
		if wildcardMatch {
			return true
		}

		cidrMatch := MatchIpCIDR(ipAddr, whitelistRules)
		if cidrMatch {
			return true
		}
	}

	return false
}

func (s *Store) GetAllWhitelistedIp() []string {
	whitelistedIp := []string{}
	entries, err := s.sysdb.ListTable("whitelist-ip")
	if err != nil {
		return whitelistedIp
	}

	for _, keypairs := range entries {
		ip := string(keypairs[0])
		whitelistedIp = append(whitelistedIp, ip)
	}

	return whitelistedIp
}
