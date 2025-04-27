package dynamicproxy

import (
	"errors"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// IpTable is a rate limiter implementation using sync.Map with atomic int64
type RequestCountPerIpTable struct {
	table sync.Map
}

// Increment the count of requests for a given IP
func (t *RequestCountPerIpTable) Increment(ip string) {
	v, _ := t.table.LoadOrStore(ip, new(int64))
	atomic.AddInt64(v.(*int64), 1)
}

// Check if the IP is in the table and if it is, check if the count is less than the limit
func (t *RequestCountPerIpTable) Exceeded(ip string, limit int64) bool {
	v, ok := t.table.Load(ip)
	if !ok {
		return false
	}
	count := atomic.LoadInt64(v.(*int64))
	return count >= limit
}

// Get the count of requests for a given IP
func (t *RequestCountPerIpTable) GetCount(ip string) int64 {
	v, ok := t.table.Load(ip)
	if !ok {
		return 0
	}
	return atomic.LoadInt64(v.(*int64))
}

// Clear the IP table
func (t *RequestCountPerIpTable) Clear() {
	t.table.Range(func(key, value interface{}) bool {
		t.table.Delete(key)
		return true
	})
}

func (h *ProxyHandler) handleRateLimitRouting(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint) error {
	err := h.Parent.handleRateLimit(w, r, pe)
	if err != nil {
		h.Parent.logRequest(r, false, 429, "ratelimit", r.URL.Hostname(), "")
	}
	return err
}

func (router *Router) handleRateLimit(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint) error {
	//Get the real client-ip from request header
	clientIP := r.RemoteAddr
	if r.Header.Get("X-Real-Ip") == "" {
		CF_Connecting_IP := r.Header.Get("CF-Connecting-IP")
		Fastly_Client_IP := r.Header.Get("Fastly-Client-IP")
		if CF_Connecting_IP != "" {
			//Use CF Connecting IP
			clientIP = CF_Connecting_IP
		} else if Fastly_Client_IP != "" {
			//Use Fastly Client IP
			clientIP = Fastly_Client_IP
		} else {
			ips := strings.Split(clientIP, ",")
			if len(ips) > 0 {
				clientIP = strings.TrimSpace(ips[0])
			}
		}
	}

	ip, _, err := net.SplitHostPort(clientIP)
	if err != nil {
		//Default allow passthrough on error
		return nil
	}

	router.rateLimitCounter.Increment(ip)

	if router.rateLimitCounter.Exceeded(ip, int64(pe.RateLimit)) {
		w.WriteHeader(429)
		return errors.New("rate limit exceeded")
	}

	// log.Println("Rate limit check", ip, ipTable.GetCount(ip))

	return nil
}

// Start the ticker routine for reseting the rate limit counter every seconds
func (r *Router) startRateLimterCounterResetTicker() error {
	if r.rateLimterStop != nil {
		return errors.New("another rate limiter ticker already running")
	}
	tickerStopChan := make(chan bool)
	r.rateLimterStop = tickerStopChan

	counterResetTicker := time.NewTicker(1 * time.Second)
	go func() {
		for {
			select {
			case <-tickerStopChan:
				r.rateLimterStop = nil
				return
			case <-counterResetTicker.C:
				r.rateLimitCounter.Clear()
			}
		}
	}()

	return nil
}
