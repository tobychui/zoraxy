package main

import (
	"testing"
	"time"

	gonet "github.com/shirou/gopsutil/v4/net"
)

/*
	Tests for the status page "Bandwidth (Today)" accumulator.

	These exercise dashboardBandwidthTracker.accumulate() with synthetic
	interface counter readings, covering the cases that previously wiped the
	running total back to zero.
*/

func nicReading(name string, rx uint64, tx uint64) gonet.IOCountersStat {
	return gonet.IOCountersStat{
		Name:      name,
		BytesRecv: rx,
		BytesSent: tx,
	}
}

func newTestBandwidthTracker(day time.Time) *dashboardBandwidthTracker {
	return &dashboardBandwidthTracker{
		day:      day.Format(DASHBOARD_DAY_LAYOUT),
		lastSeen: map[string]nicCounter{},
	}
}

func assertTotals(t *testing.T, tracker *dashboardBandwidthTracker, wantRx int64, wantTx int64, context string) {
	t.Helper()
	if tracker.rxToday != wantRx || tracker.txToday != wantTx {
		t.Fatalf("%s: expected rx=%d tx=%d, got rx=%d tx=%d",
			context, wantRx, wantTx, tracker.rxToday, tracker.txToday)
	}
}

// The first reading of an interface must only record a baseline, otherwise the
// counter history from before Zoraxy started shows up as a one-off spike.
func TestBandwidthFirstSampleOnlyBaselines(t *testing.T) {
	now := time.Date(2026, 7, 25, 10, 0, 0, 0, time.Local)
	tracker := newTestBandwidthTracker(now)

	tracker.accumulate([]gonet.IOCountersStat{nicReading("eth0", 900000, 400000)}, now)
	assertTotals(t, tracker, 0, 0, "after first sample")

	tracker.accumulate([]gonet.IOCountersStat{nicReading("eth0", 900500, 400300)}, now.Add(5*time.Second))
	assertTotals(t, tracker, 500, 300, "after second sample")
}

// Regression test: an interface disappearing (container teardown, VPN
// reconnect, adapter reset) makes the aggregated "all interfaces" counter drop.
// The previous implementation subtracted a stored baseline from that aggregate,
// saw a negative delta and rebased to zero, losing the whole day. Per-interface
// accounting must keep the running total instead.
func TestBandwidthSurvivesDisappearingInterface(t *testing.T) {
	now := time.Date(2026, 7, 25, 10, 0, 0, 0, time.Local)
	tracker := newTestBandwidthTracker(now)

	//Baseline two interfaces
	tracker.accumulate([]gonet.IOCountersStat{
		nicReading("eth0", 1000, 1000),
		nicReading("veth7a2", 5000, 5000),
	}, now)
	assertTotals(t, tracker, 0, 0, "after baseline")

	//Both interfaces move forward: +1000 rx/tx each
	tracker.accumulate([]gonet.IOCountersStat{
		nicReading("eth0", 2000, 2000),
		nicReading("veth7a2", 6000, 6000),
	}, now.Add(5*time.Second))
	assertTotals(t, tracker, 2000, 2000, "after both interfaces advanced")

	//veth7a2 disappears. The aggregate counter drops from 8000 to 2500, but
	//only eth0's own +500 should be counted and the total must be preserved.
	tracker.accumulate([]gonet.IOCountersStat{
		nicReading("eth0", 2500, 2500),
	}, now.Add(10*time.Second))
	assertTotals(t, tracker, 2500, 2500, "after interface disappeared")
}

// A counter moving backwards on a still-present interface means that
// interface's counter was reset. That window must be skipped rather than
// subtracted from the daily total.
func TestBandwidthIgnoresCounterReset(t *testing.T) {
	now := time.Date(2026, 7, 25, 10, 0, 0, 0, time.Local)
	tracker := newTestBandwidthTracker(now)

	tracker.accumulate([]gonet.IOCountersStat{nicReading("eth0", 10000, 10000)}, now)
	tracker.accumulate([]gonet.IOCountersStat{nicReading("eth0", 12000, 12000)}, now.Add(5*time.Second))
	assertTotals(t, tracker, 2000, 2000, "before counter reset")

	//Counter reset back to near zero
	tracker.accumulate([]gonet.IOCountersStat{nicReading("eth0", 50, 50)}, now.Add(10*time.Second))
	assertTotals(t, tracker, 2000, 2000, "after counter reset")

	//Counting resumes from the new baseline
	tracker.accumulate([]gonet.IOCountersStat{nicReading("eth0", 350, 250)}, now.Add(15*time.Second))
	assertTotals(t, tracker, 2300, 2200, "after resuming from reset baseline")
}

// A newly attached interface must be baselined, not counted from zero.
func TestBandwidthBaselinesNewInterface(t *testing.T) {
	now := time.Date(2026, 7, 25, 10, 0, 0, 0, time.Local)
	tracker := newTestBandwidthTracker(now)

	tracker.accumulate([]gonet.IOCountersStat{nicReading("eth0", 1000, 1000)}, now)

	//A second interface shows up already carrying a large counter value
	tracker.accumulate([]gonet.IOCountersStat{
		nicReading("eth0", 1100, 1100),
		nicReading("tun0", 999999, 999999),
	}, now.Add(5*time.Second))
	assertTotals(t, tracker, 100, 100, "when a new interface appears")

	//From here on tun0 contributes normally
	tracker.accumulate([]gonet.IOCountersStat{
		nicReading("eth0", 1100, 1100),
		nicReading("tun0", 1000099, 1000099),
	}, now.Add(10*time.Second))
	assertTotals(t, tracker, 200, 200, "after the new interface advanced")
}

// The totals must survive an arbitrarily long gap within the same local day,
// and reset only once the local calendar day changes.
func TestBandwidthResetsOnlyAtLocalMidnight(t *testing.T) {
	now := time.Date(2026, 7, 25, 6, 0, 0, 0, time.Local)
	tracker := newTestBandwidthTracker(now)

	tracker.accumulate([]gonet.IOCountersStat{nicReading("eth0", 1000, 1000)}, now)
	tracker.accumulate([]gonet.IOCountersStat{nicReading("eth0", 4000, 3000)}, now.Add(5*time.Second))
	assertTotals(t, tracker, 3000, 2000, "morning total")

	//Many hours later, still the same local day: the total must be kept
	lateSameDay := time.Date(2026, 7, 25, 23, 59, 30, 0, time.Local)
	tracker.accumulate([]gonet.IOCountersStat{nicReading("eth0", 4500, 3500)}, lateSameDay)
	assertTotals(t, tracker, 3500, 2500, "just before midnight")

	//Just after local midnight the totals reset. Counter values are unchanged
	//here so the reset is asserted without a straddling delta.
	justAfterMidnight := time.Date(2026, 7, 26, 0, 0, 5, 0, time.Local)
	tracker.accumulate([]gonet.IOCountersStat{nicReading("eth0", 4500, 3500)}, justAfterMidnight)
	assertTotals(t, tracker, 0, 0, "just after midnight")

	if tracker.day != "2026-07-26" {
		t.Fatalf("expected tracker day to roll over to 2026-07-26, got %q", tracker.day)
	}

	//The new day accumulates from the counter values carried across midnight
	tracker.accumulate([]gonet.IOCountersStat{nicReading("eth0", 4700, 3600)}, justAfterMidnight.Add(5*time.Second))
	assertTotals(t, tracker, 200, 100, "after midnight rollover")
}
