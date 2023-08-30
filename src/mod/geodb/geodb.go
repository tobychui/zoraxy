package geodb

import (
	_ "embed"
	"log"
	"net"
	"net/http"

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
	geotrie          *trie
	geotrieIpv6      *trie
	//geoipCache sync.Map
	sysdb  *database.Database
	option *StoreOptions
}

type StoreOptions struct {
	AllowSlowIpv4LookUp bool
	AllowSloeIpv6Lookup bool
}

type CountryInfo struct {
	CountryIsoCode string
	ContinetCode   string
}

func NewGeoDb(sysdb *database.Database, option *StoreOptions) (*Store, error) {
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

	var ipv4Trie *trie
	if !option.AllowSlowIpv4LookUp {
		ipv4Trie = constrctTrieTree(parsedGeoData)
	}

	var ipv6Trie *trie
	if !option.AllowSloeIpv6Lookup {
		ipv6Trie = constrctTrieTree(parsedGeoDataIpv6)
	}

	return &Store{
		BlacklistEnabled: blacklistEnabled,
		WhitelistEnabled: whitelistEnabled,
		geodb:            parsedGeoData,
		geotrie:          ipv4Trie,
		geodbIpv6:        parsedGeoDataIpv6,
		geotrieIpv6:      ipv6Trie,
		sysdb:            sysdb,
		option:           option,
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

// A helper function that check both blacklist and whitelist for access
// for both geoIP and ip / CIDR ranges
func (s *Store) AllowIpAccess(ipaddr string) bool {
	if s.IsBlacklisted(ipaddr) {
		return false
	}

	return s.IsWhitelisted(ipaddr)
}

func (s *Store) AllowConnectionAccess(conn net.Conn) bool {
	if addr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		return s.AllowIpAccess(addr.IP.String())
	}
	return true
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
