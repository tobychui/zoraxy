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

// dashboardBandwidthTracker keeps the NIC accumulated counter values sampled
// at startup / midnight so that "bandwidth today" can be derived from the
// current accumulated counters without a dedicated background routine.
type dashboardBandwidthTracker struct {
	sync.Mutex
	date       string //Date of the baseline in yyyy-mm-dd
	baselineRx int64  //Accumulated rx (bits) when the baseline was taken
	baselineTx int64  //Accumulated tx (bits) when the baseline was taken
}

var dashboardBandwidth = &dashboardBandwidthTracker{}

// Initiate the bandwidth baseline. Call once on startup after netstatBuffers
// is created. Note that on mid-day startups the counter only reflects the
// traffic since Zoraxy started.
func initDashboardBandwidthTracker() {
	rx, tx, err := netstatBuffers.GetNetworkInterfaceStats()
	if err != nil {
		rx, tx = 0, 0
	}
	dashboardBandwidth.Lock()
	defer dashboardBandwidth.Unlock()
	dashboardBandwidth.date = time.Now().Format("2006-01-02")
	dashboardBandwidth.baselineRx = rx
	dashboardBandwidth.baselineTx = tx
}

// getBandwidthToday returns the total rx / tx of this host in bytes since
// midnight (or since startup if Zoraxy was started today)
func getBandwidthToday() (int64, int64) {
	rx, tx, err := netstatBuffers.GetNetworkInterfaceStats()
	if err != nil {
		return 0, 0
	}

	dashboardBandwidth.Lock()
	defer dashboardBandwidth.Unlock()

	today := time.Now().Format("2006-01-02")
	if dashboardBandwidth.date != today {
		//Day rollover, reset the baseline
		dashboardBandwidth.date = today
		dashboardBandwidth.baselineRx = rx
		dashboardBandwidth.baselineTx = tx
	}

	drx := rx - dashboardBandwidth.baselineRx
	dtx := tx - dashboardBandwidth.baselineTx
	if drx < 0 || dtx < 0 {
		//NIC counter reset (e.g. interface restarted), rebase
		dashboardBandwidth.baselineRx = rx
		dashboardBandwidth.baselineTx = tx
		drx, dtx = 0, 0
	}

	//Netstat buffers count in bits, convert to bytes
	return drx / 8, dtx / 8
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
