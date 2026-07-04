package statistic

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestBoundedIncrCountsExistingKeys(t *testing.T) {
	m := &sync.Map{}
	b := newBoundedCounter(0)

	for i := 0; i < 100; i++ {
		boundedIncr(m, b, "hot", 1000)
	}

	v, ok := m.Load("hot")
	if !ok {
		t.Fatalf("key 'hot' should exist")
	}
	if v.(int) != 100 {
		t.Fatalf("expected count 100, got %d", v)
	}
	if got := b.size.Load(); got != 1 {
		t.Fatalf("expected size counter 1, got %d", got)
	}
}

func TestBoundedIncrTrimsLowFrequencyEntries(t *testing.T) {
	const capN = 100
	const inserts = 1000

	m := &sync.Map{}
	b := newBoundedCounter(0)

	// 10 "hot" keys hit many times each; 990 "cold" keys hit once.
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("hot-%d", i)
		for j := 0; j < 50; j++ {
			boundedIncr(m, b, key, capN)
		}
	}
	for i := 0; i < inserts-10; i++ {
		boundedIncr(m, b, fmt.Sprintf("cold-%d", i), capN)
	}

	// After all inserts, map must have been trimmed at least once.
	// Size should be at the trim target (capN * 9/10 = 90) or below.
	got := mapLen(m)
	if got > capN {
		t.Fatalf("map size %d exceeded cap %d after trimming", got, capN)
	}

	// All 10 hot keys must survive — they have the highest counts.
	for i := 0; i < 10; i++ {
		key := fmt.Sprintf("hot-%d", i)
		if _, ok := m.Load(key); !ok {
			t.Errorf("hot key %s was evicted; high-frequency entries should be preserved", key)
		}
	}

	// Size counter and actual map size should agree (within a small slop).
	if diff := abs(int(b.size.Load()) - got); diff > 0 {
		t.Errorf("size counter %d disagrees with actual map size %d", b.size.Load(), got)
	}
}

func TestNewIncrFnDispatch(t *testing.T) {
	// maxEntries=0 selects the unbounded path (upstream behavior).
	unbounded := newIncrFn(0)
	m1 := &sync.Map{}
	b1 := newBoundedCounter(0)
	for i := 0; i < 50; i++ {
		unbounded(m1, b1, fmt.Sprintf("k%d", i))
	}
	if got := mapLen(m1); got != 50 {
		t.Errorf("unbounded path: expected 50 entries, got %d (cap should not apply)", got)
	}
	if got := b1.size.Load(); got != 0 {
		t.Errorf("unbounded path: bounded counter should stay at 0, got %d", got)
	}

	// maxEntries>0 selects the bounded path.
	bounded := newIncrFn(10)
	m2 := &sync.Map{}
	b2 := newBoundedCounter(0)
	for i := 0; i < 100; i++ {
		bounded(m2, b2, fmt.Sprintf("k%d", i))
	}
	if got := mapLen(m2); got > 10 {
		t.Errorf("bounded path with cap=10: map size %d exceeds cap", got)
	}
}

func TestUnboundedIncrMatchesUpstreamPattern(t *testing.T) {
	// unboundedIncr must reproduce Load → check → Store exactly.
	m := &sync.Map{}
	unboundedIncr(m, nil, "new-key")
	v, ok := m.Load("new-key")
	if !ok || v.(int) != 1 {
		t.Fatalf("first insert: expected count 1, got %v (ok=%v)", v, ok)
	}
	for i := 0; i < 9; i++ {
		unboundedIncr(m, nil, "new-key")
	}
	v, _ = m.Load("new-key")
	if v.(int) != 10 {
		t.Fatalf("after 10 increments: expected count 10, got %v", v)
	}
}

func TestBoundedIncrConcurrent(t *testing.T) {
	const capN = 500
	const writers = 16
	const perWriter = 2000

	m := &sync.Map{}
	b := newBoundedCounter(0)

	var wg sync.WaitGroup
	var ctr atomic.Int64
	wg.Add(writers)
	for w := 0; w < writers; w++ {
		go func(wid int) {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				k := fmt.Sprintf("w%d-k%d", wid, i)
				boundedIncr(m, b, k, capN)
				ctr.Add(1)
			}
		}(w)
	}
	wg.Wait()

	// Soft cap — concurrent inserts may briefly overshoot while a trim is in
	// flight. Allow some slop; the important invariant is that we're nowhere
	// near the unbounded writers*perWriter = 32,000.
	got := mapLen(m)
	if got > capN+writers {
		t.Fatalf("map size %d well above cap %d after %d concurrent inserts", got, capN, ctr.Load())
	}
}

func mapLen(m *sync.Map) int {
	n := 0
	m.Range(func(_, _ interface{}) bool {
		n++
		return true
	})
	return n
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
