package geodb

import (
	"encoding/json"
	"strings"
)

/*
	Whitelist.go

	This script handles whitelist related functions
*/

const (
	EntryType_CountryCode int = 0
	EntryType_IP          int = 1
)

type WhitelistEntry struct {
	EntryType int    //Entry type of whitelist, Country Code or IP
	CC        string //ISO Country Code
	IP        string //IP address or range
	Comment   string //Comment for this entry
}

//Geo Whitelist

func (s *Store) AddCountryCodeToWhitelist(countryCode string, comment string) {
	countryCode = strings.ToLower(countryCode)
	entry := WhitelistEntry{
		EntryType: EntryType_CountryCode,
		CC:        countryCode,
		Comment:   comment,
	}

	s.sysdb.Write("whitelist-cn", countryCode, entry)
}

func (s *Store) RemoveCountryCodeFromWhitelist(countryCode string) {
	countryCode = strings.ToLower(countryCode)
	s.sysdb.Delete("whitelist-cn", countryCode)
}

func (s *Store) IsCountryCodeWhitelisted(countryCode string) bool {
	countryCode = strings.ToLower(countryCode)
	return s.sysdb.KeyExists("whitelist-cn", countryCode)
}

func (s *Store) GetAllWhitelistedCountryCode() []*WhitelistEntry {
	whitelistedCountryCode := []*WhitelistEntry{}
	entries, err := s.sysdb.ListTable("whitelist-cn")
	if err != nil {
		return whitelistedCountryCode
	}
	for _, keypairs := range entries {
		thisWhitelistEntry := WhitelistEntry{}
		json.Unmarshal(keypairs[1], &thisWhitelistEntry)
		whitelistedCountryCode = append(whitelistedCountryCode, &thisWhitelistEntry)
	}

	return whitelistedCountryCode
}

//IP Whitelist

func (s *Store) AddIPToWhiteList(ipAddr string, comment string) {
	thisIpEntry := WhitelistEntry{
		EntryType: EntryType_IP,
		IP:        ipAddr,
		Comment:   comment,
	}

	s.sysdb.Write("whitelist-ip", ipAddr, thisIpEntry)
}

func (s *Store) RemoveIPFromWhiteList(ipAddr string) {
	s.sysdb.Delete("whitelist-ip", ipAddr)
}

func (s *Store) IsIPWhitelisted(ipAddr string) bool {
	isWhitelisted := s.sysdb.KeyExists("whitelist-ip", ipAddr)
	if isWhitelisted {
		//single IP whitelist entry
		return true
	}

	//Check for IP wildcard and CIRD rules
	AllWhitelistedIps := s.GetAllWhitelistedIpAsStringSlice()
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

func (s *Store) GetAllWhitelistedIp() []*WhitelistEntry {
	whitelistedIp := []*WhitelistEntry{}
	entries, err := s.sysdb.ListTable("whitelist-ip")
	if err != nil {
		return whitelistedIp
	}

	for _, keypairs := range entries {
		//ip := string(keypairs[0])
		thisEntry := WhitelistEntry{}
		json.Unmarshal(keypairs[1], &thisEntry)
		whitelistedIp = append(whitelistedIp, &thisEntry)
	}

	return whitelistedIp
}

func (s *Store) GetAllWhitelistedIpAsStringSlice() []string {
	allWhitelistedIPs := []string{}
	entries := s.GetAllWhitelistedIp()
	for _, entry := range entries {
		allWhitelistedIPs = append(allWhitelistedIPs, entry.IP)
	}

	return allWhitelistedIPs
}
