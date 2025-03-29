package access

import (
	"strings"

	"imuslab.com/zoraxy/mod/netutils"
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

func (s *AccessRule) AddCountryCodeToWhitelist(countryCode string, comment string) {
	countryCode = strings.ToLower(countryCode)
	newWhitelistCC := deepCopy(*s.WhiteListCountryCode)
	newWhitelistCC[countryCode] = comment
	s.WhiteListCountryCode = &newWhitelistCC
	s.SaveChanges()
}

func (s *AccessRule) RemoveCountryCodeFromWhitelist(countryCode string) {
	countryCode = strings.ToLower(countryCode)
	newWhitelistCC := deepCopy(*s.WhiteListCountryCode)
	delete(newWhitelistCC, countryCode)
	s.WhiteListCountryCode = &newWhitelistCC
	s.SaveChanges()
}

func (s *AccessRule) IsCountryCodeWhitelisted(countryCode string) bool {
	countryCode = strings.ToLower(countryCode)
	whitelistCC := *s.WhiteListCountryCode
	_, ok := whitelistCC[countryCode]
	return ok
}

func (s *AccessRule) GetAllWhitelistedCountryCode() []*WhitelistEntry {
	whitelistedCountryCode := []*WhitelistEntry{}
	whitelistCC := *s.WhiteListCountryCode
	for cc, comment := range whitelistCC {
		whitelistedCountryCode = append(whitelistedCountryCode, &WhitelistEntry{
			EntryType: EntryType_CountryCode,
			CC:        cc,
			Comment:   comment,
		})
	}
	return whitelistedCountryCode
}

//IP Whitelist

func (s *AccessRule) AddIPToWhiteList(ipAddr string, comment string) {
	newWhitelistIP := deepCopy(*s.WhiteListIP)
	newWhitelistIP[ipAddr] = comment
	s.WhiteListIP = &newWhitelistIP
	s.SaveChanges()
}

func (s *AccessRule) RemoveIPFromWhiteList(ipAddr string) {
	newWhitelistIP := deepCopy(*s.WhiteListIP)
	delete(newWhitelistIP, ipAddr)
	s.WhiteListIP = &newWhitelistIP
	s.SaveChanges()
}

func (s *AccessRule) IsIPWhitelisted(ipAddr string) bool {
	//Check for IP wildcard and CIRD rules
	WhitelistedIP := *s.WhiteListIP
	for ipOrCIDR, _ := range WhitelistedIP {
		wildcardMatch := netutils.MatchIpWildcard(ipAddr, ipOrCIDR)
		if wildcardMatch {
			return true
		}

		cidrMatch := netutils.MatchIpCIDR(ipAddr, ipOrCIDR)
		if cidrMatch {
			return true
		}
	}

	//Check for loopback match
	if s.WhitelistAllowLocalAndLoopback {
		if s.parent.IsLoopbackRequest(ipAddr) || s.parent.IsPrivateIPRange(ipAddr) {
			return true
		}
	}

	return false
}

func (s *AccessRule) GetAllWhitelistedIp() []*WhitelistEntry {
	whitelistedIp := []*WhitelistEntry{}
	currentWhitelistedIP := *s.WhiteListIP
	for ipOrCIDR, comment := range currentWhitelistedIP {
		thisEntry := WhitelistEntry{
			EntryType: EntryType_IP,
			IP:        ipOrCIDR,
			Comment:   comment,
		}
		whitelistedIp = append(whitelistedIp, &thisEntry)
	}

	return whitelistedIp
}
