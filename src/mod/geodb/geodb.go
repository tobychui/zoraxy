package geodb

import (
	_ "embed"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/netutils"
)

//go:embed geoipv4.csv
var geoipv4 []byte //Geodb dataset for ipv4

//go:embed geoipv6.csv
var geoipv6 []byte //Geodb dataset for ipv6

const (
	//DefaultExternalGeoDBFolder is the folder storing the downloaded geodb
	//dataset, used if no ExternalGeoDBFolder is given in the store options
	DefaultExternalGeoDBFolder = "./conf/geodb"

	//Filename of the external (downloaded) geoip dataset
	externalIpv4Filename = "geoipv4.csv"
	externalIpv6Filename = "geoipv6.csv"
)

// geoDataSet is an immutable snapshot of the parsed geoip dataset. The whole
// dataset is swapped at once on database reload so an ongoing lookup will
// never see a partially updated dataset, see Store.dataset
type geoDataSet struct {
	geodb           [][]string //Parsed geodb list
	geodbIpv6       [][]string //Parsed geodb list for ipv6
	geotrie         *trie
	geotrieIpv6     *trie
	usingExternalDb bool //Whether any of the dataset is loaded from the external geodb folder
}

type Store struct {
	dataset                  atomic.Pointer[geoDataSet] //Current geoip dataset in use
	updater                  *Updater                   //Geoip database updater, see updater.go
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
	ExternalGeoDBFolder          string        //Folder for storing the downloaded geodb dataset
}

type CountryInfo struct {
	CountryIsoCode string
	ContinetCode   string
}

func NewGeoDb(sysdb *database.Database, option *StoreOptions) (*Store, error) {
	if option == nil {
		return nil, errors.New("geodb store options cannot be nil")
	}

	if option.SlowLookupCacheClearInterval == 0 {
		option.SlowLookupCacheClearInterval = 30 * time.Minute
	}

	if option.ExternalGeoDBFolder == "" {
		option.ExternalGeoDBFolder = DefaultExternalGeoDBFolder
	}

	//Create a new store
	thisGeoDBStore := &Store{
		sysdb:                    sysdb,
		slowLookupCacheIpv4:      sync.Map{},
		slowLookupCacheIpv6:      sync.Map{},
		cacheClearTicker:         time.NewTicker(option.SlowLookupCacheClearInterval),
		cacheClearTickerStopChan: make(chan bool),
		option:                   option,
	}

	//Load the geoip dataset into the store
	dataset, err := thisGeoDBStore.loadDataSet()
	if err != nil {
		return nil, err
	}
	thisGeoDBStore.dataset.Store(dataset)

	//Start cache clear ticker
	if option.AllowSlowIpv4LookUp || option.AllowSlowIpv6Lookup {
		go func(store *Store) {
			for {
				select {
				case <-store.cacheClearTickerStopChan:
					return
				case <-store.cacheClearTicker.C:
					store.slowLookupCacheIpv4.Clear()
					store.slowLookupCacheIpv6.Clear()
				}
			}
		}(thisGeoDBStore)
	}

	//Create the geoip database updater and restore its previous settings
	thisGeoDBStore.updater = newUpdater(thisGeoDBStore)

	return thisGeoDBStore, nil
}

// loadDataSet parse the geoip dataset from the external geodb folder, if exists,
// or fallback to the embedded dataset
func (s *Store) loadDataSet() (*geoDataSet, error) {
	ipv4Content := geoipv4
	ipv6Content := geoipv6
	usingExternalDb := false

	//Check if external geoDB data is available
	if externalV4Db, err := os.ReadFile(s.ExternalDatabaseFilepath(false)); err == nil && len(externalV4Db) > 0 {
		s.log("External GeoDB data found, using external IPv4 GeoIP data", nil)
		ipv4Content = externalV4Db
		usingExternalDb = true
	}

	if externalV6Db, err := os.ReadFile(s.ExternalDatabaseFilepath(true)); err == nil && len(externalV6Db) > 0 {
		s.log("External GeoDB data found, using external IPv6 GeoIP data", nil)
		ipv6Content = externalV6Db
		usingExternalDb = true
	}

	parsedGeoData, err := parseCSV(ipv4Content)
	if err != nil {
		return nil, err
	}

	parsedGeoDataIpv6, err := parseCSV(ipv6Content)
	if err != nil {
		return nil, err
	}

	var ipv4Trie *trie
	if !s.option.AllowSlowIpv4LookUp {
		ipv4Trie = constrctTrieTree(parsedGeoData)
	}

	var ipv6Trie *trie
	if !s.option.AllowSlowIpv6Lookup {
		ipv6Trie = constrctTrieTree(parsedGeoDataIpv6)
	}

	return &geoDataSet{
		geodb:           parsedGeoData,
		geodbIpv6:       parsedGeoDataIpv6,
		geotrie:         ipv4Trie,
		geotrieIpv6:     ipv6Trie,
		usingExternalDb: usingExternalDb,
	}, nil
}

// ReloadGeoDB reload the geoip dataset from the external geodb folder and hot
// swap it into the store. This is called after a database update so the new
// dataset can be used without restarting Zoraxy
func (s *Store) ReloadGeoDB() error {
	newDataSet, err := s.loadDataSet()
	if err != nil {
		return err
	}
	s.dataset.Store(newDataSet)

	//The lookup results might have changed. Drop the slow lookup caches
	s.slowLookupCacheIpv4.Clear()
	s.slowLookupCacheIpv6.Clear()
	return nil
}

// ExternalDatabaseFilepath return the path of the external (downloaded)
// geoip database file for the given IP version
func (s *Store) ExternalDatabaseFilepath(ipv6 bool) string {
	if ipv6 {
		return filepath.Join(s.option.ExternalGeoDBFolder, externalIpv6Filename)
	}
	return filepath.Join(s.option.ExternalGeoDBFolder, externalIpv4Filename)
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
	if s.updater != nil {
		s.updater.Close()
	}
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

// log write a message to the system wide logger, if one is given in the options
func (s *Store) log(message string, err error) {
	if s.option == nil || s.option.Logger == nil {
		if err != nil {
			log.Println("[GeoDB] " + message + ": " + err.Error())
		} else {
			log.Println("[GeoDB] " + message)
		}
		return
	}
	s.option.Logger.PrintAndLog("GeoDB", message, err)
}
