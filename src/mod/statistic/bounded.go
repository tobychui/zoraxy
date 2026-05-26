package statistic

import (
	"sort"
	"sync"
	"sync/atomic"
)

// maxEntriesPerStatMap is a soft cap on entries per per-dimension sync.Map in
// DailySummary. When a map exceeds this cap, its lowest-count entries are
// evicted down to ~90% of the cap. This bounds memory growth driven by
// high-cardinality request inputs (many unique client IPs, varied
// User-Agent/Referer strings, or many distinct URL paths) while preserving
// the most frequently observed entries — the data the dashboards actually
// surface.
//
// TODO: expose as a configuration option (e.g. via router.Option or a CLI
// flag) so operators running large-scale deployments can tune it. Hardcoded
// for now to keep the change surface small.
const maxEntriesPerStatMap = 20000

// boundedCounter is a sidecar to a *sync.Map[string]int that tracks its
// logical size atomically and serializes trim (eviction) operations.
type boundedCounter struct {
	size   atomic.Int64
	trimMu sync.Mutex
}

func newBoundedCounter(initialSize int) *boundedCounter {
	b := &boundedCounter{}
	b.size.Store(int64(initialSize))
	return b
}

// boundedCounters is the per-DailySummary set of size sidecars, one per
// sync.Map field in DailySummary.
type boundedCounters struct {
	ForwardTypes        *boundedCounter
	RequestOrigin       *boundedCounter
	RequestClientIp     *boundedCounter
	Referer             *boundedCounter
	UserAgent           *boundedCounter
	RequestURL          *boundedCounter
	DownstreamHostnames *boundedCounter
	UpstreamHostnames   *boundedCounter
}

func newBoundedCounters() boundedCounters {
	return boundedCounters{
		ForwardTypes:        newBoundedCounter(0),
		RequestOrigin:       newBoundedCounter(0),
		RequestClientIp:     newBoundedCounter(0),
		Referer:             newBoundedCounter(0),
		UserAgent:           newBoundedCounter(0),
		RequestURL:          newBoundedCounter(0),
		DownstreamHostnames: newBoundedCounter(0),
		UpstreamHostnames:   newBoundedCounter(0),
	}
}

// boundedIncr increments the counter for key in m, applying a soft cap of
// capN entries. New keys past the cap trigger a trim that drops the entries
// with the lowest request counts down to ~90% of capN. Safe for concurrent use.
func boundedIncr(m *sync.Map, b *boundedCounter, key string, capN int) {
	// LoadOrStore atomically inserts {key: 1} if absent; otherwise returns
	// the existing value. The new-key branch is race-free; the increment
	// branch matches the pre-existing non-atomic Load+Store pattern in
	// RecordRequest (fixing that read-modify-write race is out of scope).
	actual, loaded := m.LoadOrStore(key, 1)
	if loaded {
		m.Store(key, actual.(int)+1)
		return
	}

	newSize := b.size.Add(1)
	if int(newSize) <= capN {
		return
	}

	// Try to acquire the trim lock without blocking. If another goroutine
	// is already trimming, our insert lands; the next over-cap insert will
	// trigger a trim.
	if !b.trimMu.TryLock() {
		return
	}
	defer b.trimMu.Unlock()

	// Re-check under lock — the other trimmer may have just finished.
	if int(b.size.Load()) <= capN {
		return
	}
	evictLeastFrequent(m, b, capN)
}

// evictLeastFrequent removes the entries in m with the lowest request counts
// until ~90% of capN entries remain. Caller must hold b.trimMu.
func evictLeastFrequent(m *sync.Map, b *boundedCounter, capN int) {
	type entry struct {
		key   string
		count int
	}

	entries := make([]entry, 0, capN+capN/8)
	m.Range(func(k, v interface{}) bool {
		entries = append(entries, entry{key: k.(string), count: v.(int)})
		return true
	})

	target := capN * 9 / 10
	if len(entries) <= target {
		// Range observed fewer entries than expected (concurrent deletes
		// from another path, or the size counter overshot). Reconcile.
		b.size.Store(int64(len(entries)))
		return
	}

	// Ascending by count; evict the front (lowest-frequency) until target.
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].count < entries[j].count
	})

	evictCount := len(entries) - target
	for i := 0; i < evictCount; i++ {
		m.Delete(entries[i].key)
	}
	b.size.Add(-int64(evictCount))
}
