package dynamicproxy

import (
	"errors"
	"net"
	"net/http"
	"sync"
	"time"

	"log"
)

/*
	ratelimit.go

	This file handles the ratelimit on proxy endpoints
	if RateLimit is set to true
*/

// idk what this was for
// func (h *ProxyHandler) handleRateLimitRouting(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint) error {
// 	err := handleRateLimit(w, r, pe)
// 	if err != nil {
// 		h.logRequest(r, false, 429, "host", pe.Domain)
// 	}
// 	return err
// }

type IpTable struct {
	sync.RWMutex
	table map[string]*IpTableValue
}

// Get the ip from the table
func (t *IpTable) Get(ip string) (*IpTableValue, bool) {
	t.RLock()
	defer t.RUnlock()
	v, ok := t.table[ip]
	return v, ok
}

// Clear the ip from the table
func (t *IpTable) Clear() {
	t.Lock()
	defer t.Unlock()
	t.table = make(map[string]*IpTableValue)
}

// Increment the count of requests for a given ip
// init ip in ipTable if not exists
func (t *IpTable) Increment(ip string) {
	t.Lock()
	defer t.Unlock()
	v, ok := t.table[ip]
	if !ok {
		v = &IpTableValue{Count: 0, LastHit: time.Now()}
	}
	v.Count++
	t.table[ip] = v
}

// Check if the ip is in the table and if it is, check if the count is less than the limit
func (t *IpTable) Exceeded(ip string, limit int64) bool {
	t.RLock()
	defer t.RUnlock()
	v, ok := t.table[ip]
	if !ok {
		return false
	}
	if v.Count < limit {
		return false
	}
	return true
}

// Get the count of requests for a given ip
// returns 0 if ip is not in the table
func (t *IpTable) GetCount(ip string) int64 {
	t.RLock()
	defer t.RUnlock()
	v, ok := t.table[ip]
	if !ok {
		return 0
	}
	return v.Count
}

type IpTableValue struct {
	Count   int64
	LastHit time.Time
}

var ipTable IpTable = IpTable{table: make(map[string]*IpTableValue)}

// Handle rate limit logic
// do not write to http.ResponseWriter if err return is not nil (already handled by this function)
func handleRateLimit(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint) error {
	// if len(pe.BasicAuthExceptionRules) > 0 {
	// 	//Check if the current path matches the exception rules
	// 	for _, exceptionRule := range pe.BasicAuthExceptionRules {
	// 		if strings.HasPrefix(r.RequestURI, exceptionRule.PathPrefix) {
	// 			//This path is excluded from basic auth
	// 			return nil
	// 		}
	// 	}
	// }

	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		w.WriteHeader(500)
		log.Println("Error resolving remote address", r.RemoteAddr, err)
		return errors.New("internal server error")
	}

	ipTable.Increment(ip)

	// if ipTable.Exceeded(ip, pe.RateLimit) {
	if ipTable.Exceeded(ip, 10) {
		w.WriteHeader(429)
		return errors.New("rate limit exceeded")
	}

	log.Println("Rate limit check", ip, ipTable.GetCount(ip))

	return nil
}

func InitRateLimit() {
	for {
		ipTable.Clear()
		time.Sleep(time.Second)
	}
}
