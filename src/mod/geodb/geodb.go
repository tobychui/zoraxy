package geodb

import (
	_ "embed"
	"net/http"
	"os"
	"sync"
	"time"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/netutils"
	"imuslab.com/zoraxy/mod/utils"
)

//go:embed geoipv4.csv
var geoipv4 []byte //Geodb dataset for ipv4

//go:embed geoipv6.csv
var geoipv6 []byte //Geodb dataset for ipv6

type Store struct {
	geodb                    [][]string //Parsed geodb list
	geodbIpv6                [][]string //Parsed geodb list for ipv6
	geotrie                  *trie
	geotrieIpv6              *trie
	sysdb                    *database.Database
	slowLookupCacheIpv4      sync.Map     //Cache for slow lookup, ip -> cc
	slowLookupCacheIpv6      sync.Map     //Cache for slow lookup ipv6, ip -> cc
	cacheClearTicker         *time.Ticker //Ticker for clearing cache
	cacheClearTickerStopChan chan bool    //Stop channel for cache clear ticker
	option                   *StoreOptions
}

type StoreOptions struct {
	AllowSlowIpv4LookUp          bool
	AllowSlowIpv6Lookup          bool
	Logger                       *logger.Logger
	SlowLookupCacheClearInterval time.Duration //Clear slow lookup cache interval
}

type CountryInfo struct {
	CountryIsoCode string
	ContinetCode   string
}

func NewGeoDb(sysdb *database.Database, option *StoreOptions) (*Store, error) {
	//Check if external geoDB data is available
	if utils.FileExists("./conf/geodb/geoipv4.csv") {
		externalV4Db, err := os.ReadFile("./conf/geodb/geoipv4.csv")
		if err == nil {
			option.Logger.PrintAndLog("GeoDB", "External GeoDB data found, using external IPv4 GeoIP data", nil)
			geoipv4 = externalV4Db
		}
	}

	if utils.FileExists("./conf/geodb/geoipv6.csv") {
		externalV6Db, err := os.ReadFile("./conf/geodb/geoipv6.csv")
		if err == nil {
			option.Logger.PrintAndLog("GeoDB", "External GeoDB data found, using external IPv6 GeoIP data", nil)
			geoipv6 = externalV6Db
		}
	}

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
	if !option.AllowSlowIpv6Lookup {
		ipv6Trie = constrctTrieTree(parsedGeoDataIpv6)
	}

	if option.SlowLookupCacheClearInterval == 0 {
		option.SlowLookupCacheClearInterval = 30 * time.Minute
	}

	//Create a new store
	thisGeoDBStore := &Store{
		geodb:                    parsedGeoData,
		geotrie:                  ipv4Trie,
		geodbIpv6:                parsedGeoDataIpv6,
		geotrieIpv6:              ipv6Trie,
		sysdb:                    sysdb,
		slowLookupCacheIpv4:      sync.Map{},
		slowLookupCacheIpv6:      sync.Map{},
		cacheClearTicker:         time.NewTicker(option.SlowLookupCacheClearInterval),
		cacheClearTickerStopChan: make(chan bool),
		option:                   option,
	}

	//Start cache clear ticker
	if option.AllowSlowIpv4LookUp || option.AllowSlowIpv6Lookup {
		go func(store *Store) {
			for {
				select {
				case <-store.cacheClearTickerStopChan:
					return
				case <-thisGeoDBStore.cacheClearTicker.C:
					thisGeoDBStore.slowLookupCacheIpv4 = sync.Map{}
					thisGeoDBStore.slowLookupCacheIpv6 = sync.Map{}
				}
			}
		}(thisGeoDBStore)
	}

	return thisGeoDBStore, nil
}

func (s *Store) ResolveCountryCodeFromIP(ipstring string) (*CountryInfo, error) {
	cc := s.search(ipstring)
	return &CountryInfo{
		CountryIsoCode: cc,
		ContinetCode:   "",
	}, nil

}

// Close the store
func (s *Store) Close() {
	if s.option.AllowSlowIpv4LookUp || s.option.AllowSlowIpv6Lookup {
		//Stop cache clear ticker
		s.cacheClearTickerStopChan <- true
	}
}

func (s *Store) GetRequesterCountryISOCode(r *http.Request) string {
	ipAddr := netutils.GetRequesterIP(r)
	if ipAddr == "" {
		return ""
	}

	countryCode, err := s.ResolveCountryCodeFromIP(ipAddr)
	if err != nil {
		return ""
	}

	return countryCode.CountryIsoCode
}
