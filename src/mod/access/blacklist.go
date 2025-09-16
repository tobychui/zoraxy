package access

import (
	"fmt"
	"strings"

	"imuslab.com/zoraxy/mod/netutils"
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
	if ok {
		return true
	}

	//Check for CIDR
	for ipOrCIDR, _ := range IPBlacklist {
		wildcardMatch := netutils.MatchIpWildcard(ipAddr, ipOrCIDR)
		if wildcardMatch {
			return true
		}

		cidrMatch := netutils.MatchIpCIDR(ipAddr, ipOrCIDR)
		if cidrMatch {
			return true
		}
	}

	return false
}

// GetBlacklistedIPComment returns the comment for a blacklisted IP address
// Searches blacklist for a Country (if country-code provided), IP address, or CIDR that matches the IP address
// returns error if not found
func (s *AccessRule) GetBlacklistedIPComment(ipAddr string) (string, error) {
	if countryInfo, err := s.parent.Options.GeoDB.ResolveCountryCodeFromIP(ipAddr); err == nil {
		CCBlacklist := *s.BlackListContryCode
		countryCode := strings.ToLower(countryInfo.CountryIsoCode)

		if comment, ok := CCBlacklist[countryCode]; ok {
			return comment, nil
		}
	}

	IPBlacklist := *s.BlackListIP
	if comment, ok := IPBlacklist[ipAddr]; ok {
		return comment, nil
	}

	//Check for CIDR
	for ipOrCIDR, comment := range IPBlacklist {
		wildcardMatch := netutils.MatchIpWildcard(ipAddr, ipOrCIDR)
		if wildcardMatch {
			return comment, nil
		}

		cidrMatch := netutils.MatchIpCIDR(ipAddr, ipOrCIDR)
		if cidrMatch {
			return comment, nil
		}
	}

	return "", fmt.Errorf("IP %s not found in blacklist", ipAddr)
}

// GetParent returns the parent controller
func (s *AccessRule) GetParent() *Controller {
	return s.parent
}
