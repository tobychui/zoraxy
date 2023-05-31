package geodb

import (
	_ "embed"
	"log"
	"net/http"
	"strings"

	"imuslab.com/zoraxy/mod/database"
)

//go:embed geoipv4.csv
var geoipv4 []byte //Geodb dataset for ipv4

//go:embed geoipv6.csv
var geoipv6 []byte //Geodb dataset for ipv6

type Store struct {
	BlacklistEnabled bool
	WhitelistEnabled bool
	geodb            [][]string //Parsed geodb list
	geodbIpv6        [][]string //Parsed geodb list for ipv6

	geotrie     *trie
	geotrieIpv6 *trie

	//geoipCache sync.Map

	sysdb *database.Database
}

type CountryInfo struct {
	CountryIsoCode string
	ContinetCode   string
}

func NewGeoDb(sysdb *database.Database) (*Store, error) {
	parsedGeoData, err := parseCSV(geoipv4)
	if err != nil {
		return nil, err
	}

	parsedGeoDataIpv6, err := parseCSV(geoipv6)
	if err != nil {
		return nil, err
	}

	blacklistEnabled := false
	whitelistEnabled := false
	if sysdb != nil {
		err = sysdb.NewTable("blacklist-cn")
		if err != nil {
			return nil, err
		}

		err = sysdb.NewTable("blacklist-ip")
		if err != nil {
			return nil, err
		}

		err = sysdb.NewTable("whitelist-cn")
		if err != nil {
			return nil, err
		}

		err = sysdb.NewTable("whitelist-ip")
		if err != nil {
			return nil, err
		}

		err = sysdb.NewTable("blackwhitelist")
		if err != nil {
			return nil, err
		}

		sysdb.Read("blackwhitelist", "blacklistEnabled", &blacklistEnabled)
		sysdb.Read("blackwhitelist", "whitelistEnabled", &whitelistEnabled)
	} else {
		log.Println("Database pointer set to nil: Entering debug mode")
	}

	return &Store{
		BlacklistEnabled: blacklistEnabled,
		WhitelistEnabled: whitelistEnabled,
		geodb:            parsedGeoData,
		geotrie:          constrctTrieTree(parsedGeoData),
		geodbIpv6:        parsedGeoDataIpv6,
		geotrieIpv6:      constrctTrieTree(parsedGeoDataIpv6),
		sysdb:            sysdb,
	}, nil
}

func (s *Store) ToggleBlacklist(enabled bool) {
	s.sysdb.Write("blackwhitelist", "blacklistEnabled", enabled)
	s.BlacklistEnabled = enabled
}

func (s *Store) ToggleWhitelist(enabled bool) {
	s.sysdb.Write("blackwhitelist", "whitelistEnabled", enabled)
	s.WhitelistEnabled = enabled
}

func (s *Store) ResolveCountryCodeFromIP(ipstring string) (*CountryInfo, error) {
	cc := s.search(ipstring)
	return &CountryInfo{
		CountryIsoCode: cc,
		ContinetCode:   "",
	}, nil
}

func (s *Store) Close() {

}

/*
	Country code based black / white list
*/

func (s *Store) AddCountryCodeToBlackList(countryCode string) {
	countryCode = strings.ToLower(countryCode)
	s.sysdb.Write("blacklist-cn", countryCode, true)
}

func (s *Store) RemoveCountryCodeFromBlackList(countryCode string) {
	countryCode = strings.ToLower(countryCode)
	s.sysdb.Delete("blacklist-cn", countryCode)
}

func (s *Store) AddCountryCodeToWhitelist(countryCode string) {
	countryCode = strings.ToLower(countryCode)
	s.sysdb.Write("whitelist-cn", countryCode, true)
}

func (s *Store) RemoveCountryCodeFromWhitelist(countryCode string) {
	countryCode = strings.ToLower(countryCode)
	s.sysdb.Delete("whitelist-cn", countryCode)
}

func (s *Store) IsCountryCodeBlacklisted(countryCode string) bool {
	countryCode = strings.ToLower(countryCode)
	var isBlacklisted bool = false
	s.sysdb.Read("blacklist-cn", countryCode, &isBlacklisted)
	return isBlacklisted
}

func (s *Store) IsCountryCodeWhitelisted(countryCode string) bool {
	countryCode = strings.ToLower(countryCode)
	var isWhitelisted bool = false
	s.sysdb.Read("whitelist-cn", countryCode, &isWhitelisted)
	return isWhitelisted
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

/*
	IP based black / whitelist
*/

func (s *Store) AddIPToBlackList(ipAddr string) {
	s.sysdb.Write("blacklist-ip", ipAddr, true)
}

func (s *Store) RemoveIPFromBlackList(ipAddr string) {
	s.sysdb.Delete("blacklist-ip", ipAddr)
}

func (s *Store) AddIPToWhiteList(ipAddr string) {
	s.sysdb.Write("whitelist-ip", ipAddr, true)
}

func (s *Store) RemoveIPFromWhiteList(ipAddr string) {
	s.sysdb.Delete("whitelist-ip", ipAddr)
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

func (s *Store) IsIPWhitelisted(ipAddr string) bool {
	var isBlacklisted bool = false
	s.sysdb.Read("whitelist-ip", ipAddr, &isBlacklisted)
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

/*
Check if a IP address is blacklisted, in either country or IP blacklist
IsBlacklisted default return is false (allow access)
*/
func (s *Store) IsBlacklisted(ipAddr string) bool {
	if !s.BlacklistEnabled {
		//Blacklist not enabled. Always return false
		return false
	}

	if ipAddr == "" {
		//Unable to get the target IP address
		return false
	}

	countryCode, err := s.ResolveCountryCodeFromIP(ipAddr)
	if err != nil {
		return false
	}

	if s.IsCountryCodeBlacklisted(countryCode.CountryIsoCode) {
		return true
	}

	if s.IsIPBlacklisted(ipAddr) {
		return true
	}

	return false
}

/*
IsWhitelisted check if a given IP address is in the current
server's white list.

Note that the Whitelist default result is true even
when encountered error
*/
func (s *Store) IsWhitelisted(ipAddr string) bool {
	if !s.WhitelistEnabled {
		//Whitelist not enabled. Always return true (allow access)
		return true
	}

	if ipAddr == "" {
		//Unable to get the target IP address, assume ok
		return true
	}

	countryCode, err := s.ResolveCountryCodeFromIP(ipAddr)
	if err != nil {
		return true
	}

	if s.IsCountryCodeWhitelisted(countryCode.CountryIsoCode) {
		return true
	}

	if s.IsIPWhitelisted(ipAddr) {
		return true
	}

	return false
}

func (s *Store) GetRequesterCountryISOCode(r *http.Request) string {
	ipAddr := GetRequesterIP(r)
	if ipAddr == "" {
		return ""
	}
	countryCode, err := s.ResolveCountryCodeFromIP(ipAddr)
	if err != nil {
		return ""
	}

	return countryCode.CountryIsoCode
}
