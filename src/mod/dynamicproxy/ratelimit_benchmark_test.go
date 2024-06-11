package dynamicproxy

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

func BenchmarkRateLimitSyncMapInt64(b *testing.B) {
	ipTableSyncMapInt64 = IpTableSyncMapInt64{} // Reset the table

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleRateLimitSyncMapInt64(w, r, &ProxyEndpoint{RateLimiting: 10})
	})

	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "192.168.1.1:1234"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
	}
}

func BenchmarkRateLimitSyncMapAtomicInt64(b *testing.B) {
	ipTableSyncMapAtomicInt64 = IpTableSyncMapAtomicInt64{} // Reset the table

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleRateLimitSyncMapAtomicInt64(w, r, &ProxyEndpoint{RateLimiting: 10})
	})

	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "192.168.1.1:1234"

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
	}
}

func BenchmarkRateLimitSyncMapInt64Concurrent(b *testing.B) {
	ipTableSyncMapInt64 = IpTableSyncMapInt64{} // Reset the table

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleRateLimitSyncMapInt64(w, r, &ProxyEndpoint{RateLimiting: 10})
	})

	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "192.168.1.1:1234"

	b.ResetTimer()

	var wg sync.WaitGroup
	for i := 0; i < b.N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
		}()
	}
	wg.Wait()
}

func BenchmarkRateLimitSyncMapAtomicInt64Concurrent(b *testing.B) {
	ipTableSyncMapAtomicInt64 = IpTableSyncMapAtomicInt64{} // Reset the table

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleRateLimitSyncMapAtomicInt64(w, r, &ProxyEndpoint{RateLimiting: 10})
	})

	request := httptest.NewRequest("GET", "/", nil)
	request.RemoteAddr = "192.168.1.1:1234"

	b.ResetTimer()

	var wg sync.WaitGroup
	for i := 0; i < b.N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, request)
		}()
	}
	wg.Wait()
}
