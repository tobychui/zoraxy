package plugins

import (
	"testing"
	"time"
)

var policyCacheSink *inspectPolicy

// Isolate a valid cache hit (production TTL is one second); no HTTP fetch,
// persistence, stale-read or lifetime behavior is changed by this benchmark.
func BenchmarkInspectPolicyCacheHit(b *testing.B) {
	c := &inspectPolicyCache{refreshed: time.Now().Add(time.Hour), current: &inspectPolicy{}}
	b.ReportAllocs()
	for b.Loop() {
		p, err := c.load(0, "/policy")
		if err != nil || p == nil {
			b.Fatal(p, err)
		}
		policyCacheSink = p
	}
}
