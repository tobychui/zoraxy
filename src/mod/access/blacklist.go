package access

import (
	"strings"
)

/*
	Blacklist.go

	This script store the blacklist related functions
*/

// Geo Blacklist
func (s *AccessRule) AddCountryCodeToBlackList(countryCode string, comment string) {
	countryCode = strings.ToLower(countryCode)
	newBlacklistCountryCode := deepCopy(*s.BlackListContryCode)
	newBlacklistCountryCode[countryCode] = comment
	s.BlackListContryCode = &newBlacklistCountryCode
	s.SaveChanges()
}

func (s *AccessRule) RemoveCountryCodeFromBlackList(countryCode string) {
	countryCode = strings.ToLower(countryCode)
	newBlacklistCountryCode := deepCopy(*s.BlackListContryCode)
	delete(newBlacklistCountryCode, countryCode)
	s.BlackListContryCode = &newBlacklistCountryCode
	s.SaveChanges()
}

func (s *AccessRule) IsCountryCodeBlacklisted(countryCode string) bool {
	countryCode = strings.ToLower(countryCode)
	blacklistMap := *s.BlackListContryCode
	_, ok := blacklistMap[countryCode]
	return ok
}

func (s *AccessRule) GetAllBlacklistedCountryCode() []string {
	bannedCountryCodes := []string{}
	blacklistMap := *s.BlackListContryCode
	for cc, _ := range blacklistMap {
		bannedCountryCodes = append(bannedCountryCodes, cc)
	}
	return bannedCountryCodes
}

// IP Blacklsits
func (s *AccessRule) AddIPToBlackList(ipAddr string, comment string) {
	newBlackListIP := deepCopy(*s.BlackListIP)
	newBlackListIP[ipAddr] = comment
	s.BlackListIP = &newBlackListIP
	s.SaveChanges()
}

func (s *AccessRule) RemoveIPFromBlackList(ipAddr string) {
	newBlackListIP := deepCopy(*s.BlackListIP)
	delete(newBlackListIP, ipAddr)
	s.BlackListIP = &newBlackListIP
	s.SaveChanges()
}

func (s *AccessRule) GetAllBlacklistedIp() []string {
	bannedIps := []string{}
	blacklistMap := *s.BlackListIP
	for ip, _ := range blacklistMap {
		bannedIps = append(bannedIps, ip)
	}

	return bannedIps
}

func (s *AccessRule) IsIPBlacklisted(ipAddr string) bool {
	IPBlacklist := *s.BlackListIP
	_, ok := IPBlacklist[ipAddr]
	return ok
}
