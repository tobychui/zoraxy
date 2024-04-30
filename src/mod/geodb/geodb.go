package geodb

import (
	_ "embed"
	"net/http"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/netutils"
)

//go:embed geoipv4.csv
var geoipv4 []byte //Geodb dataset for ipv4

//go:embed geoipv6.csv
var geoipv6 []byte //Geodb dataset for ipv6

type Store struct {
	geodb       [][]string //Parsed geodb list
	geodbIpv6   [][]string //Parsed geodb list for ipv6
	geotrie     *trie
	geotrieIpv6 *trie
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

	var ipv4Trie *trie
	if !option.AllowSlowIpv4LookUp {
		ipv4Trie = constrctTrieTree(parsedGeoData)
	}

	var ipv6Trie *trie
	if !option.AllowSloeIpv6Lookup {
		ipv6Trie = constrctTrieTree(parsedGeoDataIpv6)
	}

	return &Store{
		geodb:       parsedGeoData,
		geotrie:     ipv4Trie,
		geodbIpv6:   parsedGeoDataIpv6,
		geotrieIpv6: ipv6Trie,
		sysdb:       sysdb,
		option:      option,
	}, nil
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

func (s *Store) GetRequesterCountryISOCode(r *http.Request) string {
	ipAddr := netutils.GetRequesterIP(r)
	if ipAddr == "" {
		return ""
	}

	if netutils.IsPrivateIP(ipAddr) {
		return "LAN"
	}

	countryCode, err := s.ResolveCountryCodeFromIP(ipAddr)
	if err != nil {
		return ""
	}

	return countryCode.CountryIsoCode
}
