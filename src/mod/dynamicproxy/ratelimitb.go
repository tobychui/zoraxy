package dynamicproxy

import (
	"errors"
	"log"
	"net"
	"net/http"
	"sync"
	"time"
)

// IpTableSyncMapInt64 is a rate limiter implementation using sync.Map with int64
type IpTableSyncMapInt64 struct {
	table sync.Map
}

// Increment the count of requests for a given IP
func (t *IpTableSyncMapInt64) Increment(ip string) {
	v, _ := t.table.LoadOrStore(ip, new(int64))
	count := v.(*int64)
	*count++
}

// Check if the IP is in the table and if it is, check if the count is less than the limit
func (t *IpTableSyncMapInt64) Exceeded(ip string, limit int64) bool {
	v, ok := t.table.Load(ip)
	if !ok {
		return false
	}
	count := v.(*int64)
	return *count >= limit
}

// Get the count of requests for a given IP
func (t *IpTableSyncMapInt64) GetCount(ip string) int64 {
	v, ok := t.table.Load(ip)
	if !ok {
		return 0
	}
	count := v.(*int64)
	return *count
}

// Clear the IP table
func (t *IpTableSyncMapInt64) Clear() {
	t.table.Range(func(key, value interface{}) bool {
		t.table.Delete(key)
		return true
	})
}

var ipTableSyncMapInt64 = IpTableSyncMapInt64{}

func handleRateLimitSyncMapInt64(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint) error {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		w.WriteHeader(500)
		log.Println("Error resolving remote address", r.RemoteAddr, err)
		return errors.New("internal server error")
	}

	ipTableSyncMapInt64.Increment(ip)

	if ipTableSyncMapInt64.Exceeded(ip, 10) {
		w.WriteHeader(429)
		return errors.New("rate limit exceeded")
	}

	// log.Println("Rate limit check", ip, ipTableSyncMapInt64.GetCount(ip))

	return nil
}

func InitRateLimitSyncMapInt64() {
	for {
		ipTableSyncMapInt64.Clear()
		time.Sleep(time.Second)
	}
}
