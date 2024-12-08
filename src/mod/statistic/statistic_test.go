package statistic_test

import (
	"net"
	"os"
	"testing"
	"time"

	"math/rand"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/database/dbinc"
	"imuslab.com/zoraxy/mod/geodb"
	"imuslab.com/zoraxy/mod/statistic"
)

const test_db_path = "test_db"

func getNewDatabase() *database.Database {
	db, err := database.NewDatabase(test_db_path, dbinc.BackendLevelDB)
	if err != nil {
		panic(err)
	}
	db.NewTable("stats")
	return db
}

func clearDatabase(db *database.Database) {
	db.Close()
	os.RemoveAll(test_db_path)
}

func TestNewStatisticCollector(t *testing.T) {
	db := getNewDatabase()
	defer clearDatabase(db)
	option := statistic.CollectorOption{Database: db}
	collector, err := statistic.NewStatisticCollector(option)
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}
	if collector == nil {
		t.Fatalf("Expected collector, got nil")
	}

}

func TestSaveSummaryOfDay(t *testing.T) {
	db := getNewDatabase()
	defer clearDatabase(db)
	option := statistic.CollectorOption{Database: db}
	collector, _ := statistic.NewStatisticCollector(option)
	collector.SaveSummaryOfDay()
	// Add assertions to check if data is saved correctly
}

func TestLoadSummaryOfDay(t *testing.T) {
	db := getNewDatabase()
	defer clearDatabase(db)
	option := statistic.CollectorOption{Database: db}
	collector, _ := statistic.NewStatisticCollector(option)
	year, month, day := time.Now().Date()
	summary := collector.LoadSummaryOfDay(year, month, day)
	if summary == nil {
		t.Fatalf("Expected summary, got nil")
	}
}

func TestResetSummaryOfDay(t *testing.T) {
	db := getNewDatabase()
	defer clearDatabase(db)
	option := statistic.CollectorOption{Database: db}
	collector, _ := statistic.NewStatisticCollector(option)
	collector.ResetSummaryOfDay()
	if collector.DailySummary.TotalRequest != 0 {
		t.Fatalf("Expected TotalRequest to be 0, got %v", collector.DailySummary.TotalRequest)
	}
}

func TestGetCurrentRealtimeStatIntervalId(t *testing.T) {
	db := getNewDatabase()
	defer clearDatabase(db)
	option := statistic.CollectorOption{Database: db}
	collector, _ := statistic.NewStatisticCollector(option)
	intervalId := collector.GetCurrentRealtimeStatIntervalId()
	if intervalId < 0 || intervalId > 287 {
		t.Fatalf("Expected intervalId to be between 0 and 287, got %v", intervalId)
	}
}

func TestRecordRequest(t *testing.T) {
	db := getNewDatabase()
	defer clearDatabase(db)
	option := statistic.CollectorOption{Database: db}
	collector, _ := statistic.NewStatisticCollector(option)
	requestInfo := statistic.RequestInfo{
		IpAddr:                        "127.0.0.1",
		RequestOriginalCountryISOCode: "US",
		Succ:                          true,
		StatusCode:                    200,
		ForwardType:                   "type1",
		Referer:                       "http://example.com",
		UserAgent:                     "Mozilla/5.0",
		RequestURL:                    "/test",
		Target:                        "target1",
	}
	collector.RecordRequest(requestInfo)
	time.Sleep(1 * time.Second) // Wait for the goroutine to finish
	if collector.DailySummary.TotalRequest != 1 {
		t.Fatalf("Expected TotalRequest to be 1, got %v", collector.DailySummary.TotalRequest)
	}
}

func TestScheduleResetRealtimeStats(t *testing.T) {
	db := getNewDatabase()
	defer clearDatabase(db)
	option := statistic.CollectorOption{Database: db}
	collector, _ := statistic.NewStatisticCollector(option)
	stopChan := collector.ScheduleResetRealtimeStats()
	if stopChan == nil {
		t.Fatalf("Expected stopChan, got nil")
	}
	collector.Close()
}

func TestNewDailySummary(t *testing.T) {
	summary := statistic.NewDailySummary()
	if summary.TotalRequest != 0 {
		t.Fatalf("Expected TotalRequest to be 0, got %v", summary.TotalRequest)
	}
	if summary.ForwardTypes == nil {
		t.Fatalf("Expected ForwardTypes to be initialized, got nil")
	}
	if summary.RequestOrigin == nil {
		t.Fatalf("Expected RequestOrigin to be initialized, got nil")
	}
	if summary.RequestClientIp == nil {
		t.Fatalf("Expected RequestClientIp to be initialized, got nil")
	}
	if summary.Referer == nil {
		t.Fatalf("Expected Referer to be initialized, got nil")
	}
	if summary.UserAgent == nil {
		t.Fatalf("Expected UserAgent to be initialized, got nil")
	}
	if summary.RequestURL == nil {
		t.Fatalf("Expected RequestURL to be initialized, got nil")
	}
}

func generateTestRequestInfo(db *database.Database) statistic.RequestInfo {
	//Generate a random IPv4 address
	randomIpAddr := ""
	for {
		ip := net.IPv4(byte(rand.Intn(256)), byte(rand.Intn(256)), byte(rand.Intn(256)), byte(rand.Intn(256)))
		if !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsMulticast() && !ip.IsUnspecified() {
			randomIpAddr = ip.String()
			break
		}
	}

	//Resolve the country code for this IP
	ipLocation := "unknown"
	geoIpResolver, err := geodb.NewGeoDb(db, &geodb.StoreOptions{
		AllowSlowIpv4LookUp: false,
		AllowSlowIpv6Lookup: true, //Just to save some RAM
	})

	if err == nil {
		ipInfo, _ := geoIpResolver.ResolveCountryCodeFromIP(randomIpAddr)
		ipLocation = ipInfo.CountryIsoCode
	}

	forwardType := "host-http"
	//Generate a random forward type between "subdomain-http" and "host-https"
	if rand.Intn(2) == 1 {
		forwardType = "subdomain-http"
	}

	//Generate 5 random refers URL and pick from there
	referers := []string{"https://example.com", "https://example.org", "https://example.net", "https://example.io", "https://example.co"}
	referer := referers[rand.Intn(5)]

	return statistic.RequestInfo{
		IpAddr:                        randomIpAddr,
		RequestOriginalCountryISOCode: ipLocation,
		Succ:                          true,
		StatusCode:                    200,
		ForwardType:                   forwardType,
		Referer:                       referer,
		UserAgent:                     "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/114.0.0.0 Safari/537.36",
		RequestURL:                    "/benchmark",
		Target:                        "test.imuslab.internal",
	}
}

func BenchmarkRecordRequest(b *testing.B) {
	db := getNewDatabase()
	defer clearDatabase(db)

	option := statistic.CollectorOption{Database: db}
	collector, _ := statistic.NewStatisticCollector(option)
	var requestInfo statistic.RequestInfo = generateTestRequestInfo(db)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		collector.RecordRequest(requestInfo)
		collector.SaveSummaryOfDay()
	}

	//Write the current in-memory summary to database file
	b.StopTimer()

	//Print the generated summary
	//testSummary := collector.GetCurrentDailySummary()
	//statistic.PrintDailySummary(testSummary)
}
