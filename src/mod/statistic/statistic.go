package statistic

import (
	"path/filepath"
	"strings"
	"sync"
	"time"

	"imuslab.com/zoraxy/mod/database"
)

/*
	Statistic Package

	This packet is designed to collection information
	and store them for future analysis
*/

// Faststat, a interval summary for all collected data and avoid
// looping through every data everytime a overview is needed
type DailySummary struct {
	TotalRequest int64 //Total request of the day
	ErrorRequest int64 //Invalid request of the day, including error or not found
	ValidRequest int64 //Valid request of the day
	//Type counters
	ForwardTypes    *sync.Map //Map that hold the forward types
	RequestOrigin   *sync.Map //Map that hold [country ISO code]: visitor counter
	RequestClientIp *sync.Map //Map that hold all unique request IPs
	Referer         *sync.Map //Map that store where the user was refered from
	UserAgent       *sync.Map //Map that store the useragent of the request
	RequestURL      *sync.Map //Request URL of the request object
}

type RequestInfo struct {
	IpAddr                        string
	RequestOriginalCountryISOCode string
	Succ                          bool
	StatusCode                    int
	ForwardType                   string
	Referer                       string
	UserAgent                     string
	RequestURL                    string
	Target                        string
}

type CollectorOption struct {
	Database *database.Database
}

type Collector struct {
	rtdataStopChan chan bool
	DailySummary   *DailySummary
	Option         *CollectorOption
}

func NewStatisticCollector(option CollectorOption) (*Collector, error) {
	option.Database.NewTable("stats")

	//Create the collector object
	thisCollector := Collector{
		DailySummary: newDailySummary(),
		Option:       &option,
	}

	//Load the stat if exists for today
	//This will exists if the program was forcefully restarted
	year, month, day := time.Now().Date()
	summary := thisCollector.LoadSummaryOfDay(year, month, day)
	if summary != nil {
		thisCollector.DailySummary = summary
	}

	//Schedule the realtime statistic clearing at midnight everyday
	rtstatStopChan := thisCollector.ScheduleResetRealtimeStats()
	thisCollector.rtdataStopChan = rtstatStopChan

	return &thisCollector, nil
}

// Write the current in-memory summary to database file
func (c *Collector) SaveSummaryOfDay() {
	//When it is called in 0:00am, make sure it is stored as yesterday key
	t := time.Now().Add(-30 * time.Second)
	summaryKey := t.Format("2006_01_02")
	saveData := DailySummaryToExport(*c.DailySummary)
	c.Option.Database.Write("stats", summaryKey, saveData)
}

// Load the summary of a day given
func (c *Collector) LoadSummaryOfDay(year int, month time.Month, day int) *DailySummary {
	date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Local)
	summaryKey := date.Format("2006_01_02")
	targetSummaryExport := DailySummaryExport{}
	c.Option.Database.Read("stats", summaryKey, &targetSummaryExport)
	targetSummary := DailySummaryExportToSummary(targetSummaryExport)
	return &targetSummary
}

// This function gives the current slot in the 288- 5 minutes interval of the day
func (c *Collector) GetCurrentRealtimeStatIntervalId() int {
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local).Unix()
	secondsSinceStartOfDay := now.Unix() - startOfDay
	interval := secondsSinceStartOfDay / (5 * 60)
	return int(interval)
}

func (c *Collector) Close() {
	//Stop the ticker
	c.rtdataStopChan <- true

	//Write the buffered data into database
	c.SaveSummaryOfDay()

}

// Main function to record all the inbound traffics
// Note that this function run in go routine and might have concurrent R/W issue
// Please make sure there is no racing paramters in this function
func (c *Collector) RecordRequest(ri RequestInfo) {
	go func() {
		c.DailySummary.TotalRequest++
		if ri.Succ {
			c.DailySummary.ValidRequest++
		} else {
			c.DailySummary.ErrorRequest++
		}

		//Store the request info into correct types of maps
		ft, ok := c.DailySummary.ForwardTypes.Load(ri.ForwardType)
		if !ok {
			c.DailySummary.ForwardTypes.Store(ri.ForwardType, 1)
		} else {
			c.DailySummary.ForwardTypes.Store(ri.ForwardType, ft.(int)+1)
		}

		originISO := strings.ToLower(ri.RequestOriginalCountryISOCode)
		fo, ok := c.DailySummary.RequestOrigin.Load(originISO)
		if !ok {
			c.DailySummary.RequestOrigin.Store(originISO, 1)
		} else {
			c.DailySummary.RequestOrigin.Store(originISO, fo.(int)+1)
		}

		//Filter out CF forwarded requests
		if strings.Contains(ri.IpAddr, ",") {
			ips := strings.Split(strings.TrimSpace(ri.IpAddr), ",")
			if len(ips) >= 1 {
				ri.IpAddr = ips[0]
			}
		}

		fi, ok := c.DailySummary.RequestClientIp.Load(ri.IpAddr)
		if !ok {
			c.DailySummary.RequestClientIp.Store(ri.IpAddr, 1)
		} else {
			c.DailySummary.RequestClientIp.Store(ri.IpAddr, fi.(int)+1)
		}

		//Record the referer
		rf, ok := c.DailySummary.Referer.Load(ri.Referer)
		if !ok {
			c.DailySummary.Referer.Store(ri.Referer, 1)
		} else {
			c.DailySummary.Referer.Store(ri.Referer, rf.(int)+1)
		}

		//Record the UserAgent
		ua, ok := c.DailySummary.UserAgent.Load(ri.UserAgent)
		if !ok {
			c.DailySummary.UserAgent.Store(ri.UserAgent, 1)
		} else {
			c.DailySummary.UserAgent.Store(ri.UserAgent, ua.(int)+1)
		}

		//ADD MORE HERE IF NEEDED

		//Record request URL, if it is a page
		ext := filepath.Ext(ri.RequestURL)

		if ext != "" && !isWebPageExtension(ext) {
			return
		}

		ru, ok := c.DailySummary.RequestURL.Load(ri.RequestURL)
		if !ok {
			c.DailySummary.RequestURL.Store(ri.RequestURL, 1)
		} else {
			c.DailySummary.RequestURL.Store(ri.RequestURL, ru.(int)+1)
		}
	}()
}

// nightly task
func (c *Collector) ScheduleResetRealtimeStats() chan bool {
	doneCh := make(chan bool)

	go func() {
		defer close(doneCh)

		for {
			// calculate duration until next midnight
			now := time.Now()

			// Get midnight of the next day in the local time zone
			midnight := time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, now.Location())

			// Calculate the duration until midnight
			duration := midnight.Sub(now)
			select {
			case <-time.After(duration):
				// store daily summary to database and reset summary
				c.SaveSummaryOfDay()
				c.DailySummary = newDailySummary()
			case <-doneCh:
				// stop the routine
				return
			}
		}
	}()

	return doneCh
}

func newDailySummary() *DailySummary {
	return &DailySummary{
		TotalRequest:    0,
		ErrorRequest:    0,
		ValidRequest:    0,
		ForwardTypes:    &sync.Map{},
		RequestOrigin:   &sync.Map{},
		RequestClientIp: &sync.Map{},
		Referer:         &sync.Map{},
		UserAgent:       &sync.Map{},
		RequestURL:      &sync.Map{},
	}
}
