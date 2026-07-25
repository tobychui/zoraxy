package main

import (
	"encoding/json"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/shirou/gopsutil/v4/disk"
	gonet "github.com/shirou/gopsutil/v4/net"
	"imuslab.com/zoraxy/mod/dynamicproxy"
	"imuslab.com/zoraxy/mod/info/usageinfo"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	statuspage.go

	This script handles the data endpoints for the status page dashboard,
	providing aggregated overview information (uptime, bandwidth, active
	connections and traffic counters) and host system resource usage
*/

/* Bandwidth (Today) tracker */

const (
	//Traffic accumulation sample interval
	DASHBOARD_BANDWIDTH_SAMPLE_INTERVAL = 5 * time.Second
	//Auto save interval
	DASHBOARD_BANDWIDTH_SAVE_INTERVAL = 60 * time.Second
	//Database table and key used to persist the running totals
	DASHBOARD_DB_TABLE      = "dashboard"
	DASHBOARD_BANDWIDTH_KEY = "bandwidth_today"
	//Local calendar day layout used to detect the midnight rollover
	DASHBOARD_DAY_LAYOUT = "2006-01-02"
)

// nicCounter is the raw cumulative byte counter of a single interface
type nicCounter struct {
	rx uint64
	tx uint64
}

// dashboardBandwidthRecord is the on-disk form of the running totals
type dashboardBandwidthRecord struct {
	Date string //Local calendar day the totals below belong to
	Rx   int64
	Tx   int64
}

/*
dashboardBandwidthTracker accumulates the host-wide network traffic of the
current local day for the status page.

Note that the daily total is deliberately NOT derived by subtracting a stored
baseline from the aggregated "all interfaces" counter. That aggregate is the
sum over whichever interfaces happen to exist at sample time, so it is not
monotonic: whenever an interface goes away (container teardown, VPN or
ZeroTier reconnect, adapter reset) the aggregate drops and the subtraction
underflows, which previously wiped the whole day's total back to zero.

Instead every interface is tracked individually and only forward movement is
accumulated, so a disappearing interface simply contributes nothing and the
running total is preserved. The totals are reset only when the local calendar
day changes.
*/
type dashboardBandwidthTracker struct {
	mu       sync.Mutex
	day      string                //Local calendar day that rxToday / txToday belong to
	rxToday  int64                 //Bytes received since local midnight
	txToday  int64                 //Bytes sent since local midnight
	lastSeen map[string]nicCounter //Last raw cumulative counter of each interface
	dirty    bool                  //True if the totals changed since the last save
	stopChan chan bool
}

var dashboardBandwidth = &dashboardBandwidthTracker{
	lastSeen: map[string]nicCounter{},
}

// Initiate the bandwidth tracker and start its background sampler.
// Call once on startup, after the system database is ready.
func initDashboardBandwidthTracker() {
	sysdb.NewTable(DASHBOARD_DB_TABLE)

	today := time.Now().Format(DASHBOARD_DAY_LAYOUT)

	dashboardBandwidth.mu.Lock()
	dashboardBandwidth.day = today
	dashboardBandwidth.lastSeen = map[string]nicCounter{}

	//Restore the running total if the stored record belongs to today. A record
	//from an earlier day is ignored so the new day starts from zero.
	record := dashboardBandwidthRecord{}
	if err := sysdb.Read(DASHBOARD_DB_TABLE, DASHBOARD_BANDWIDTH_KEY, &record); err == nil && record.Date == today {
		dashboardBandwidth.rxToday = record.Rx
		dashboardBandwidth.txToday = record.Tx
	}
	dashboardBandwidth.mu.Unlock()

	//Prime the per-interface snapshot. Every interface is unknown at this
	//point so this first sample only records the baselines and adds nothing.
	dashboardBandwidth.sample()

	dashboardBandwidth.start()
}

// start launches the background sampling and autosave routine
func (t *dashboardBandwidthTracker) start() {
	t.stopChan = make(chan bool)
	go func(stopChan chan bool) {
		sampleTicker := time.NewTicker(DASHBOARD_BANDWIDTH_SAMPLE_INTERVAL)
		saveTicker := time.NewTicker(DASHBOARD_BANDWIDTH_SAVE_INTERVAL)
		defer sampleTicker.Stop()
		defer saveTicker.Stop()

		for {
			select {
			case <-stopChan:
				return
			case <-sampleTicker.C:
				t.sample()
			case <-saveTicker.C:
				t.save()
			}
		}
	}(t.stopChan)
}

// Close stops the background sampler and flushes the totals to the database
func (t *dashboardBandwidthTracker) Close() {
	t.mu.Lock()
	stopChan := t.stopChan
	t.stopChan = nil
	t.mu.Unlock()

	if stopChan != nil {
		close(stopChan)
	}
	t.save()
}

// sample reads the current per-interface counters and folds them into the
// running totals
func (t *dashboardBandwidthTracker) sample() {
	counters, err := gonet.IOCounters(true)
	if err != nil {
		//Skip this window. The counters are cumulative, so the next successful
		//sample still accounts for the traffic that happened in between.
		return
	}
	t.accumulate(counters, time.Now())
}

// accumulate folds one set of per-interface counter readings into the running
// totals. Split out from sample() so the logic can be tested with synthetic
// counter readings.
func (t *dashboardBandwidthTracker) accumulate(counters []gonet.IOCountersStat, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.rolloverIfNeeded(now)

	current := make(map[string]nicCounter, len(counters))
	var deltaRx, deltaTx int64
	for _, counter := range counters {
		reading := nicCounter{rx: counter.BytesRecv, tx: counter.BytesSent}
		current[counter.Name] = reading

		previous, seenBefore := t.lastSeen[counter.Name]
		if !seenBefore {
			//Newly appeared interface. Only record the baseline, as counting
			//its whole history would show up as a one-off spike.
			continue
		}

		//Only count forward movement. A counter going backwards means that
		//interface's counter was reset, so that window is skipped instead of
		//being subtracted from the daily total.
		if reading.rx > previous.rx {
			deltaRx += int64(reading.rx - previous.rx)
		}
		if reading.tx > previous.tx {
			deltaTx += int64(reading.tx - previous.tx)
		}
	}

	//Interfaces that disappeared are simply absent from current, so they
	//contribute nothing instead of underflowing the total.
	t.lastSeen = current

	if deltaRx > 0 || deltaTx > 0 {
		t.rxToday += deltaRx
		t.txToday += deltaTx
		t.dirty = true
	}
}

// rolloverIfNeeded zeroes the running totals when the local calendar day has
// changed. The caller must hold t.mu.
func (t *dashboardBandwidthTracker) rolloverIfNeeded(now time.Time) {
	today := now.Format(DASHBOARD_DAY_LAYOUT)
	if t.day == today {
		return
	}

	t.day = today
	t.rxToday = 0
	t.txToday = 0
	t.dirty = true
}

// save flushes the running totals to the database if they have changed
func (t *dashboardBandwidthTracker) save() {
	if sysdb == nil {
		return
	}

	t.mu.Lock()
	if !t.dirty {
		t.mu.Unlock()
		return
	}
	record := dashboardBandwidthRecord{
		Date: t.day,
		Rx:   t.rxToday,
		Tx:   t.txToday,
	}
	t.dirty = false
	t.mu.Unlock()

	if err := sysdb.Write(DASHBOARD_DB_TABLE, DASHBOARD_BANDWIDTH_KEY, record); err != nil {
		//Keep the dirty flag so the next tick retries the write
		t.mu.Lock()
		t.dirty = true
		t.mu.Unlock()
	}
}

// getBandwidthToday returns the total rx / tx of this host in bytes since
// local midnight
func getBandwidthToday() (int64, int64) {
	dashboardBandwidth.mu.Lock()
	defer dashboardBandwidth.mu.Unlock()

	//Defensive rollover, in case the sampler has not ticked across midnight yet
	dashboardBandwidth.rolloverIfNeeded(time.Now())

	return dashboardBandwidth.rxToday, dashboardBandwidth.txToday
}

// countActiveProxyConnections counts the number of established TCP
// connections on the reverse proxy listening ports
func countActiveProxyConnections() int {
	if dynamicProxyRouter == nil || !dynamicProxyRouter.Running {
		return 0
	}

	listeningPorts := map[uint32]bool{
		uint32(dynamicProxyRouter.Option.Port): true,
	}
	if dynamicProxyRouter.Option.ListenOnPort80 {
		listeningPorts[80] = true
	}

	conns, err := gonet.Connections("tcp")
	if err != nil {
		return -1
	}

	count := 0
	for _, conn := range conns {
		if conn.Status == "ESTABLISHED" && listeningPorts[conn.Laddr.Port] {
			count++
		}
	}
	return count
}

// countSyncMapEntries returns the number of entries inside a sync.Map
func countSyncMapEntries(m *sync.Map) int {
	count := 0
	if m == nil {
		return 0
	}
	m.Range(func(_, _ interface{}) bool {
		count++
		return true
	})
	return count
}

// HandleDashboardOverview return the aggregated overview data used by the
// hero banner and summary cards on the status page dashboard
func HandleDashboardOverview(w http.ResponseWriter, r *http.Request) {
	type YesterdaySummary struct {
		TotalRequest   int64
		ErrorRequest   int64
		ValidRequest   int64
		UniqueVisitors int
	}
	type Overview struct {
		SystemUptime        int64 //Seconds since Zoraxy started
		ProxyRunning        bool
		TotalRequestToday   int64
		ErrorRequestToday   int64
		ValidRequestToday   int64
		UniqueVisitorsToday int
		BandwidthRxToday    int64 //Bytes received today (host wide)
		BandwidthTxToday    int64 //Bytes sent today (host wide)
		ActiveConnections   int   //Established TCP connections on proxy ports, -1 if unavailable
		ProxyHostCount      int   //Number of configured proxy rules
		UpstreamCount       int   //Number of active upstream servers over all proxy rules
		Yesterday           YesterdaySummary
	}

	//Count proxy hosts and upstreams
	proxyHostCount := 0
	upstreamCount := 0
	if dynamicProxyRouter != nil {
		dynamicProxyRouter.ProxyEndpoints.Range(func(_, value interface{}) bool {
			ep := value.(*dynamicproxy.ProxyEndpoint)
			proxyHostCount++
			upstreamCount += len(ep.ActiveOrigins)
			return true
		})
	}

	//Load yesterday summary for comparison values
	yesterday := time.Now().AddDate(0, 0, -1)
	yesterdaySummary := statisticCollector.LoadSummaryOfDay(yesterday.Year(), yesterday.Month(), yesterday.Day())

	rxToday, txToday := getBandwidthToday()

	d := statisticCollector.DailySummary
	result := Overview{
		SystemUptime:        time.Now().Unix() - bootTime,
		ProxyRunning:        dynamicProxyRouter != nil && dynamicProxyRouter.Running,
		TotalRequestToday:   d.TotalRequest,
		ErrorRequestToday:   d.ErrorRequest,
		ValidRequestToday:   d.ValidRequest,
		UniqueVisitorsToday: countSyncMapEntries(d.RequestClientIp),
		BandwidthRxToday:    rxToday,
		BandwidthTxToday:    txToday,
		ActiveConnections:   countActiveProxyConnections(),
		ProxyHostCount:      proxyHostCount,
		UpstreamCount:       upstreamCount,
		Yesterday: YesterdaySummary{
			TotalRequest:   yesterdaySummary.TotalRequest,
			ErrorRequest:   yesterdaySummary.ErrorRequest,
			ValidRequest:   yesterdaySummary.ValidRequest,
			UniqueVisitors: countSyncMapEntries(yesterdaySummary.RequestClientIp),
		},
	}

	js, _ := json.Marshal(result)
	utils.SendJSONResponse(w, string(js))
}

// HandleSystemResourceUsage return the current host CPU / RAM / disk usage
// for the System Resources card on the status page dashboard
func HandleSystemResourceUsage(w http.ResponseWriter, r *http.Request) {
	type SystemResource struct {
		CPUUsage  float64 //CPU usage in percentage
		UsedRAM   string  //Used RAM in human readable format
		TotalRAM  string  //Total RAM in human readable format
		RAMUsage  float64 //RAM usage in percentage
		DiskUsed  uint64  //Used disk space in bytes
		DiskTotal uint64  //Total disk space in bytes
		DiskUsage float64 //Disk usage in percentage
		DiskPath  string  //The volume the usage is measured on
		HostOS    string
		HostArch  string
		HostName  string
		Ready     bool //False before the first background sample is ready
	}

	cpuUsage, usedRAM, totalRAM, ramUsage, ready := usageinfo.GetCachedStats()

	result := SystemResource{
		CPUUsage: cpuUsage,
		UsedRAM:  usedRAM,
		TotalRAM: totalRAM,
		RAMUsage: ramUsage,
		HostOS:   runtime.GOOS,
		HostArch: runtime.GOARCH,
		Ready:    ready,
	}

	if hostname, err := os.Hostname(); err == nil {
		result.HostName = hostname
	}

	//Disk usage of the volume Zoraxy is running (and storing config) on
	if wd, err := os.Getwd(); err == nil {
		if du, err := disk.Usage(wd); err == nil {
			result.DiskUsed = du.Used
			result.DiskTotal = du.Total
			result.DiskUsage = du.UsedPercent
			result.DiskPath = du.Path
		}
	}

	js, _ := json.Marshal(result)
	utils.SendJSONResponse(w, string(js))
}
