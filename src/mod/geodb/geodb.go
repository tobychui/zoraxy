package geodb

import (
	"net"
	"net/http"
	"strings"

	"github.com/oschwald/geoip2-golang"
	"imuslab.com/zoraxy/mod/database"
)

type Store struct {
	Enabled bool
	geodb   *geoip2.Reader
	sysdb   *database.Database
}

type CountryInfo struct {
	CountryIsoCode string
	ContinetCode   string
}

func NewGeoDb(sysdb *database.Database, dbfile string) (*Store, error) {
	db, err := geoip2.Open(dbfile)
	if err != nil {
		return nil, err
	}

	err = sysdb.NewTable("blacklist-cn")
	if err != nil {
		return nil, err
	}

	err = sysdb.NewTable("blacklist-ip")
	if err != nil {
		return nil, err
	}

	err = sysdb.NewTable("blacklist")
	if err != nil {
		return nil, err
	}

	blacklistEnabled := false
	sysdb.Read("blacklist", "enabled", &blacklistEnabled)

	return &Store{
		Enabled: blacklistEnabled,
		geodb:   db,
		sysdb:   sysdb,
	}, nil
}

func (s *Store) ToggleBlacklist(enabled bool) {
	s.sysdb.Write("blacklist", "enabled", enabled)
	s.Enabled = enabled
}

func (s *Store) ResolveCountryCodeFromIP(ipstring string) (*CountryInfo, error) {
	// If you are using strings that may be invalid, check that ip is not nil
	ip := net.ParseIP(ipstring)
	record, err := s.geodb.City(ip)
	if err != nil {
		return nil, err
	}
	return &CountryInfo{
		record.Country.IsoCode,
		record.Continent.Code,
	}, nil
}

func (s *Store) Close() {
	s.geodb.Close()
}

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

func (s *Store) AddIPToBlackList(ipAddr string) {
	s.sysdb.Write("blacklist-ip", ipAddr, true)
}

func (s *Store) RemoveIPFromBlackList(ipAddr string) {
	s.sysdb.Delete("blacklist-ip", ipAddr)
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

//Check if a IP address is blacklisted, in either country or IP blacklist
func (s *Store) IsBlacklisted(ipAddr string) bool {
	if !s.Enabled {
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

//Utilities function
func GetRequesterIP(r *http.Request) string {
	ip := r.Header.Get("X-Forwarded-For")
	if ip == "" {
		ip = r.Header.Get("X-Real-IP")
		if ip == "" {
			ip = strings.Split(r.RemoteAddr, ":")[0]
		}
	}
	return ip
}

//Match the IP address with a wildcard string
func MatchIpWildcard(ipAddress, wildcard string) bool {
	// Split IP address and wildcard into octets
	ipOctets := strings.Split(ipAddress, ".")
	wildcardOctets := strings.Split(wildcard, ".")

	// Check that both have 4 octets
	if len(ipOctets) != 4 || len(wildcardOctets) != 4 {
		return false
	}

	// Check each octet to see if it matches the wildcard or is an exact match
	for i := 0; i < 4; i++ {
		if wildcardOctets[i] == "*" {
			continue
		}
		if ipOctets[i] != wildcardOctets[i] {
			return false
		}
	}

	return true
}

//Match ip address with CIDR
func MatchIpCIDR(ip string, cidr string) bool {
	// parse the CIDR string
	_, cidrnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}

	// parse the IP address
	ipAddr := net.ParseIP(ip)

	// check if the IP address is within the CIDR range
	return cidrnet.Contains(ipAddr)
}
