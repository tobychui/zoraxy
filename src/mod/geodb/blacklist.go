package geodb

import "strings"

/*
	Blacklist.go

	This script store the blacklist related functions
*/

//Geo Blacklist

func (s *Store) AddCountryCodeToBlackList(countryCode string) {
	countryCode = strings.ToLower(countryCode)
	s.sysdb.Write("blacklist-cn", countryCode, true)
}

func (s *Store) RemoveCountryCodeFromBlackList(countryCode string) {
	countryCode = strings.ToLower(countryCode)
	s.sysdb.Delete("blacklist-cn", countryCode)
}

func (s *Store) IsCountryCodeBlacklisted(countryCode string) bool {
	countryCode = strings.ToLower(countryCode)
	var isBlacklisted bool = false
	s.sysdb.Read("blacklist-cn", countryCode, &isBlacklisted)
	return isBlacklisted
}

func (s *Store) GetAllBlacklistedCountryCode() []string {
	bannedCountryCodes := []string{}
	entries, err := s.sysdb.ListTable("blacklist-cn")
	if err != nil {
		return bannedCountryCodes
	}
	for _, keypairs := range entries {
		ip := string(keypairs[0])
		bannedCountryCodes = append(bannedCountryCodes, ip)
	}

	return bannedCountryCodes
}

//IP Blacklsits

func (s *Store) AddIPToBlackList(ipAddr string) {
	s.sysdb.Write("blacklist-ip", ipAddr, true)
}

func (s *Store) RemoveIPFromBlackList(ipAddr string) {
	s.sysdb.Delete("blacklist-ip", ipAddr)
}

func (s *Store) GetAllBlacklistedIp() []string {
	bannedIps := []string{}
	entries, err := s.sysdb.ListTable("blacklist-ip")
	if err != nil {
		return bannedIps
	}

	for _, keypairs := range entries {
		ip := string(keypairs[0])
		bannedIps = append(bannedIps, ip)
	}

	return bannedIps
}

func (s *Store) IsIPBlacklisted(ipAddr string) bool {
	var isBlacklisted bool = false
	s.sysdb.Read("blacklist-ip", ipAddr, &isBlacklisted)
	if isBlacklisted {
		return true
	}

	//Check for IP wildcard and CIRD rules
	AllBlacklistedIps := s.GetAllBlacklistedIp()
	for _, blacklistRule := range AllBlacklistedIps {
		wildcardMatch := MatchIpWildcard(ipAddr, blacklistRule)
		if wildcardMatch {
			return true
		}

		cidrMatch := MatchIpCIDR(ipAddr, blacklistRule)
		if cidrMatch {
			return true
		}
	}

	return false
}
