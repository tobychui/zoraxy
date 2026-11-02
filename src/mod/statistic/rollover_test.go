package statistic

import (
	"os"
	"testing"
	"time"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/database/dbinc"
)

// Saves after the midnight reset must not overwrite the previous day
func TestSaveAfterMidnightKeepsPreviousDay(t *testing.T) {
	dbPath := "test_rollover_db"
	db, err := database.NewDatabase(dbPath, dbinc.BackendLevelDB)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		db.Close()
		os.RemoveAll(dbPath)
	}()

	collector, err := NewStatisticCollector(CollectorOption{Database: db})
	if err != nil {
		t.Fatal(err)
	}

	//Pretend the current in-memory summary belongs to yesterday
	yesterday := time.Now().AddDate(0, 0, -1)
	collector.summaryKey = summaryKeyOf(yesterday)
	collector.DailySummary.TotalRequest = 183000
	collector.DailySummary.RequestClientIp.Store("1.2.3.4", 1)

	//Midnight reset: yesterday is persisted and a new summary is started
	collector.SaveSummaryOfDay()
	if collector.DailySummary.TotalRequest != 0 {
		t.Fatalf("expected a fresh summary after rollover, got %d requests", collector.DailySummary.TotalRequest)
	}

	//A few requests arrive after midnight, then the autosave ticker fires
	collector.DailySummary.TotalRequest = 11
	collector.SaveSummaryOfDay()

	y, m, d := yesterday.Date()
	if got := collector.LoadSummaryOfDay(y, m, d).TotalRequest; got != 183000 {
		t.Fatalf("yesterday's summary was overwritten, expected 183000 requests, got %d", got)
	}

	y, m, d = time.Now().Date()
	if got := collector.LoadSummaryOfDay(y, m, d).TotalRequest; got != 11 {
		t.Fatalf("expected 11 requests saved for today, got %d", got)
	}
}
