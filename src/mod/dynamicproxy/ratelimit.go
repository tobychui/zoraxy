package dynamicproxy

import (
	"errors"
	"log"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// IpTable is a rate limiter implementation using sync.Map with atomic int64
type IpTable struct {
	table sync.Map
}

// Increment the count of requests for a given IP
func (t *IpTable) Increment(ip string) {
	v, _ := t.table.LoadOrStore(ip, new(int64))
	atomic.AddInt64(v.(*int64), 1)
}

// Check if the IP is in the table and if it is, check if the count is less than the limit
func (t *IpTable) Exceeded(ip string, limit int64) bool {
	v, ok := t.table.Load(ip)
	if !ok {
		return false
	}
	count := atomic.LoadInt64(v.(*int64))
	return count >= limit
}

// Get the count of requests for a given IP
func (t *IpTable) GetCount(ip string) int64 {
	v, ok := t.table.Load(ip)
	if !ok {
		return 0
	}
	return atomic.LoadInt64(v.(*int64))
}

// Clear the IP table
func (t *IpTable) Clear() {
	t.table.Range(func(key, value interface{}) bool {
		t.table.Delete(key)
		return true
	})
}

var ipTable = IpTable{}

func handleRateLimit(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint) error {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		w.WriteHeader(500)
		log.Println("Error resolving remote address", r.RemoteAddr, err)
		return errors.New("internal server error")
	}

	ipTable.Increment(ip)

	if ipTable.Exceeded(ip, 10) {
		w.WriteHeader(429)
		return errors.New("rate limit exceeded")
	}

	// log.Println("Rate limit check", ip, ipTable.GetCount(ip))

	return nil
}

func InitRateLimit() {
	for {
		ipTable.Clear()
		time.Sleep(time.Second)
	}
}
