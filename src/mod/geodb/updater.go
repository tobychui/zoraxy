package geodb

/*
	updater.go

	This script handles the download of the latest geoip dataset from the
	upstream CDN, either triggered manually by the administrator or by the
	scheduled (weekly) updater.
*/

import (
	"errors"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"imuslab.com/zoraxy/mod/utils"
)

const (
	ipv4UpdateSource = "https://cdn.jsdelivr.net/npm/@ip-location-db/geo-whois-asn-country/geo-whois-asn-country-ipv4.csv"
	ipv6UpdateSource = "https://cdn.jsdelivr.net/npm/@ip-location-db/geo-whois-asn-country/geo-whois-asn-country-ipv6.csv"

	//GeoDBUpdateInterval is the base interval between two scheduled updates
	GeoDBUpdateInterval = 7 * 24 * time.Hour

	/*
		GeoDBUpdateJitter is the maximum random offset added on top of the base
		update interval. As the schedule is anchored on the process start time
		and further randomized here, nodes started by a cron job at a fixed time
		will still spread their requests out instead of hitting the upstream CDN
		all at the same moment
	*/
	GeoDBUpdateJitter = 30 * time.Minute

	//GeoDBUpdateRetryInterval is the (jittered) wait time before retrying a
	//failed scheduled update instead of waiting for another full week
	GeoDBUpdateRetryInterval = 1 * time.Hour

	//geoDBDownloadTimeout is the timeout for downloading one database file
	geoDBDownloadTimeout = 5 * time.Minute

	//geoDBDownloadSizeLimit is the maximum accepted size of one database file
	geoDBDownloadSizeLimit = 128 << 20 // 128MB

	//Database table and keys for persisting the updater settings
	geodbUpdaterTable  = "geodb"
	geodbAutoUpdateKey = "autoupdate"
	geodbLastUpdateKey = "lastupdate"
)

// UpdaterStatus is the current state of the geoip database and its updater,
// returned to the web UI for rendering the update section
type UpdaterStatus struct {
	AutoUpdateEnabled bool   `json:"autoUpdateEnabled"`
	Updating          bool   `json:"updating"`          //An update is currently in progress
	LastUpdateTime    int64  `json:"lastUpdateTime"`    //Unix timestamp, 0 if never updated
	NextUpdateTime    int64  `json:"nextUpdateTime"`    //Unix timestamp, 0 if no update is scheduled
	LastUpdateError   string `json:"lastUpdateError"`   //Error message of the last failed update, if any
	UsingExternalDB   bool   `json:"usingExternalDB"`   //Using the downloaded dataset instead of the embedded one
	DatabaseTime      int64  `json:"databaseTime"`      //Unix timestamp of the downloaded dataset, 0 if embedded
	IPv4EntryCount    int    `json:"ipv4EntryCount"`    //Number of loaded IPv4 ranges
	IPv6EntryCount    int    `json:"ipv6EntryCount"`    //Number of loaded IPv6 ranges
	UpdateIntervalSec int64  `json:"updateIntervalSec"` //Base interval between two scheduled updates
}

// Updater handles the manual and scheduled update of the geoip database
type Updater struct {
	store           *Store
	autoUpdate      atomic.Bool   //Scheduled update is enabled
	updating        atomic.Bool   //An update is in progress
	lastUpdateTime  atomic.Int64  //Unix timestamp of the last successful update
	nextUpdateTime  atomic.Int64  //Unix timestamp of the next scheduled update
	lastUpdateError atomic.Value  //string, error message of the last failed update
	stopChan        chan struct{} //Stop channel for the scheduler
	updateMutex     sync.Mutex    //Ensure only one update is running at a time
	schedulerMutex  sync.Mutex    //Guard the start / stop of the scheduler
}

// newUpdater create a geoip database updater for the given store and restore
// the previously saved updater settings, if any
func newUpdater(store *Store) *Updater {
	thisUpdater := &Updater{
		store: store,
	}
	thisUpdater.lastUpdateError.Store("")

	if store.sysdb == nil {
		//No database backend (e.g. unit tests). Manual update only
		return thisUpdater
	}

	store.sysdb.NewTable(geodbUpdaterTable)

	lastUpdateTime := int64(0)
	store.sysdb.Read(geodbUpdaterTable, geodbLastUpdateKey, &lastUpdateTime)
	thisUpdater.lastUpdateTime.Store(lastUpdateTime)

	autoUpdate := false
	store.sysdb.Read(geodbUpdaterTable, geodbAutoUpdateKey, &autoUpdate)
	if autoUpdate {
		thisUpdater.startScheduler()
	}

	return thisUpdater
}

// SetGeoDBAutoUpdate enable or disable the scheduled geoip database update.
// When enabled, the first update is scheduled one week (plus a random offset)
// from now instead of a fixed wall clock time
func (s *Store) SetGeoDBAutoUpdate(enabled bool) error {
	if s.updater == nil {
		return errors.New("geoip database updater not initialized")
	}
	return s.updater.SetAutoUpdate(enabled)
}

// UpdateGeoDBNow download the latest geoip database and reload it into the
// store. This is a blocking call that returns when the update is completed
func (s *Store) UpdateGeoDBNow() error {
	if s.updater == nil {
		return errors.New("geoip database updater not initialized")
	}
	return s.updater.UpdateNow()
}

// GetUpdaterStatus return the current geoip database and updater status
func (s *Store) GetUpdaterStatus() *UpdaterStatus {
	status := &UpdaterStatus{
		UpdateIntervalSec: int64(GeoDBUpdateInterval.Seconds()),
	}

	if dataset := s.dataset.Load(); dataset != nil {
		status.UsingExternalDB = dataset.usingExternalDb
		status.IPv4EntryCount = len(dataset.geodb)
		status.IPv6EntryCount = len(dataset.geodbIpv6)
	}

	//Use the modification time of the downloaded dataset as its version
	if fileInfo, err := os.Stat(s.ExternalDatabaseFilepath(false)); err == nil {
		status.DatabaseTime = fileInfo.ModTime().Unix()
	}

	if s.updater != nil {
		status.AutoUpdateEnabled = s.updater.autoUpdate.Load()
		status.Updating = s.updater.updating.Load()
		status.LastUpdateTime = s.updater.lastUpdateTime.Load()
		status.NextUpdateTime = s.updater.nextUpdateTime.Load()
		if lastError, ok := s.updater.lastUpdateError.Load().(string); ok {
			status.LastUpdateError = lastError
		}
	}

	return status
}

// SetAutoUpdate enable or disable the scheduled update and persist the setting
func (u *Updater) SetAutoUpdate(enabled bool) error {
	if enabled {
		u.startScheduler()
	} else {
		u.stopScheduler()
	}

	if u.store.sysdb != nil {
		return u.store.sysdb.Write(geodbUpdaterTable, geodbAutoUpdateKey, enabled)
	}
	return nil
}

// UpdateNow download the latest geoip database and hot reload it into the store
func (u *Updater) UpdateNow() error {
	if !u.updateMutex.TryLock() {
		return errors.New("a geoip database update is already in progress")
	}
	defer u.updateMutex.Unlock()

	u.updating.Store(true)
	defer u.updating.Store(false)

	err := DownloadGeoDBUpdateToPath(u.store.option.ExternalGeoDBFolder)
	if err != nil {
		u.lastUpdateError.Store(err.Error())
		return err
	}

	//Apply the newly downloaded dataset without restarting Zoraxy
	if err = u.store.ReloadGeoDB(); err != nil {
		u.lastUpdateError.Store(err.Error())
		return err
	}

	u.lastUpdateError.Store("")
	u.lastUpdateTime.Store(time.Now().Unix())
	if u.store.sysdb != nil {
		u.store.sysdb.Write(geodbUpdaterTable, geodbLastUpdateKey, u.lastUpdateTime.Load())
	}
	u.store.log("GeoIP database updated and reloaded", nil)
	return nil
}

// Close stop the scheduler, if it is running
func (u *Updater) Close() {
	u.stopScheduler()
}

// startScheduler start the scheduled update worker if it is not already running
func (u *Updater) startScheduler() {
	u.schedulerMutex.Lock()
	defer u.schedulerMutex.Unlock()
	if u.autoUpdate.Load() {
		//Scheduler already running
		return
	}

	u.autoUpdate.Store(true)
	stopChan := make(chan struct{})
	u.stopChan = stopChan
	go u.scheduleWorker(stopChan)
}

// stopScheduler stop the scheduled update worker if it is running
func (u *Updater) stopScheduler() {
	u.schedulerMutex.Lock()
	defer u.schedulerMutex.Unlock()
	if !u.autoUpdate.Load() {
		//Scheduler not running
		return
	}

	u.autoUpdate.Store(false)
	close(u.stopChan)
	u.stopChan = nil
	u.nextUpdateTime.Store(0)
}

/*
scheduleWorker wait until the next scheduled update time and update the
database when the timer is up.

The wait time is one week plus a random offset counted from the moment the
scheduler was started, so the database is refreshed weekly relative to the
system start time instead of at a fixed hour of the day.
*/
func (u *Updater) scheduleWorker(stopChan chan struct{}) {
	waitDuration := jitteredInterval(GeoDBUpdateInterval)
	for {
		u.nextUpdateTime.Store(time.Now().Add(waitDuration).Unix())
		timer := time.NewTimer(waitDuration)
		select {
		case <-stopChan:
			timer.Stop()
			return
		case <-timer.C:
			if err := u.UpdateNow(); err != nil {
				//Update failed. Retry sooner instead of waiting another week
				u.store.log("Scheduled GeoIP database update failed", err)
				waitDuration = jitteredInterval(GeoDBUpdateRetryInterval)
			} else {
				waitDuration = jitteredInterval(GeoDBUpdateInterval)
			}
		}
	}
}

// jitteredInterval return the given interval with a random offset of up to
// GeoDBUpdateJitter added on top of it
func jitteredInterval(baseInterval time.Duration) time.Duration {
	return baseInterval + time.Duration(rand.Int63n(int64(GeoDBUpdateJitter)))
}

// DownloadGeoDBUpdateToPath download the latest geoip dataset into the given
// folder. The downloaded files are validated before they replace the current
// dataset, so a failed or corrupted download will not break the existing one
func DownloadGeoDBUpdateToPath(externalGeoDBStoragePath string) error {
	if externalGeoDBStoragePath == "" {
		externalGeoDBStoragePath = DefaultExternalGeoDBFolder
	}

	//Create the storage path if not exist
	if !utils.FileExists(externalGeoDBStoragePath) {
		if err := os.MkdirAll(externalGeoDBStoragePath, 0755); err != nil {
			return err
		}
	}

	err := downloadAndValidateGeoDBCsv(ipv4UpdateSource, externalGeoDBStoragePath+"/"+externalIpv4Filename)
	if err != nil {
		return errors.New("IPv4 database update failed: " + err.Error())
	}

	err = downloadAndValidateGeoDBCsv(ipv6UpdateSource, externalGeoDBStoragePath+"/"+externalIpv6Filename)
	if err != nil {
		return errors.New("IPv6 database update failed: " + err.Error())
	}

	return nil
}

// DownloadGeoDBUpdate download the latest geodb update, used by the
// -update_geoip maintenance flag
func DownloadGeoDBUpdate(externalGeoDBStoragePath string) {
	log.Println("Downloading GeoIP database update...")
	if err := DownloadGeoDBUpdateToPath(externalGeoDBStoragePath); err != nil {
		log.Println(err)
		return
	}

	log.Println("GeoDB update stored at: " + externalGeoDBStoragePath)
	log.Println("Exiting...")
}

// Utility functions

// downloadAndValidateGeoDBCsv download a geoip csv from the given url, validate
// its content and atomically replace the file at savepath with it
func downloadAndValidateGeoDBCsv(url string, savepath string) error {
	client := &http.Client{Timeout: geoDBDownloadTimeout}
	resp, err := client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.New("unexpected response from update server: " + resp.Status)
	}

	fileContent, err := io.ReadAll(io.LimitReader(resp.Body, geoDBDownloadSizeLimit))
	if err != nil {
		return err
	}

	//Make sure the downloaded file is a valid geoip csv before replacing the
	//current one, otherwise the geodb will fail to load on the next startup
	parsedContent, err := parseCSV(fileContent)
	if err != nil {
		return err
	}
	if len(parsedContent) == 0 || len(parsedContent[0]) < 3 {
		return errors.New("downloaded database is empty or malformed")
	}

	//Write to a temporary file first so an interrupted write will not
	//leave a partially downloaded database behind
	tmpFilepath := savepath + ".tmp"
	if err = os.WriteFile(tmpFilepath, fileContent, 0644); err != nil {
		return err
	}

	if err = os.Rename(tmpFilepath, savepath); err != nil {
		os.Remove(tmpFilepath)
		return err
	}

	return nil
}
