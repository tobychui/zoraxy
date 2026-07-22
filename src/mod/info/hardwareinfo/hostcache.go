package hardwareinfo

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"time"
)

// refreshInterval controls how often drive, NIC and USB data is refreshed.
// CPU model and total RAM are sampled once and never expire.
const refreshInterval = 60 * time.Second

type hostInfoCache struct {
	mu sync.RWMutex

	// Static: primed once, never refreshed.
	cpuBytes []byte
	ramBytes []byte

	// Volatile: refreshed every refreshInterval.
	driveBytes []byte
	ifcBytes   []byte
	usbBytes   []byte
}

var hic = &hostInfoCache{}

// StartHostInfoCache primes all hardware caches in the background and
// launches a ticker to keep volatile data (drives, NICs, USB) fresh.
// Call this once at startup.
func StartHostInfoCache() {
	go hic.refresh(true) // static + volatile initial prime

	go func() {
		ticker := time.NewTicker(refreshInterval)
		defer ticker.Stop()
		for range ticker.C {
			hic.refresh(false) // volatile-only periodic refresh
		}
	}()
}

// refresh collects data by invoking the existing HTTP handlers through an
// in-process httptest recorder. When withStatic is true, CPU and RAM are
// re-sampled; otherwise only drives, NICs and USB are refreshed.
func (c *hostInfoCache) refresh(withStatic bool) {
	req, _ := http.NewRequest("GET", "/", nil)

	if withStatic {
		r1 := httptest.NewRecorder()
		GetCPUInfo(r1, req)
		r2 := httptest.NewRecorder()
		GetRamInfo(r2, req)
		c.mu.Lock()
		c.cpuBytes = r1.Body.Bytes()
		c.ramBytes = r2.Body.Bytes()
		c.mu.Unlock()
	}

	r3 := httptest.NewRecorder()
	GetDriveStat(r3, req)
	r4 := httptest.NewRecorder()
	Ifconfig(r4, req)
	r5 := httptest.NewRecorder()
	GetUSB(r5, req)

	c.mu.Lock()
	c.driveBytes = r3.Body.Bytes()
	c.ifcBytes = r4.Body.Bytes()
	c.usbBytes = r5.Body.Bytes()
	c.mu.Unlock()
}

// CachedGetCPUInfo serves CPU info from cache, falling back to a live call
// for the brief window before the first background sample completes.
func CachedGetCPUInfo(w http.ResponseWriter, r *http.Request) {
	hic.mu.RLock()
	b := hic.cpuBytes
	hic.mu.RUnlock()
	if len(b) == 0 {
		GetCPUInfo(w, r)
		return
	}
	w.Write(b)
}

// CachedGetRamInfo serves total RAM from cache.
func CachedGetRamInfo(w http.ResponseWriter, r *http.Request) {
	hic.mu.RLock()
	b := hic.ramBytes
	hic.mu.RUnlock()
	if len(b) == 0 {
		GetRamInfo(w, r)
		return
	}
	w.Write(b)
}

// CachedGetDriveStat serves drive statistics from cache.
func CachedGetDriveStat(w http.ResponseWriter, r *http.Request) {
	hic.mu.RLock()
	b := hic.driveBytes
	hic.mu.RUnlock()
	if len(b) == 0 {
		GetDriveStat(w, r)
		return
	}
	w.Write(b)
}

// CachedIfconfig serves NIC info from cache.
func CachedIfconfig(w http.ResponseWriter, r *http.Request) {
	hic.mu.RLock()
	b := hic.ifcBytes
	hic.mu.RUnlock()
	if len(b) == 0 {
		Ifconfig(w, r)
		return
	}
	w.Write(b)
}

// CachedGetUSB serves USB device info from cache.
func CachedGetUSB(w http.ResponseWriter, r *http.Request) {
	hic.mu.RLock()
	b := hic.usbBytes
	hic.mu.RUnlock()
	if len(b) == 0 {
		GetUSB(w, r)
		return
	}
	w.Write(b)
}
