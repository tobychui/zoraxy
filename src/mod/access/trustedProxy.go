package access

import (
	"bufio"
	_ "embed"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"strings"

	"imuslab.com/zoraxy/mod/netutils"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	TrustedProxy.go

	Module to handle trusted proxy IPs
*/

//go:embed default_trusted_proxies.csv
var defaultTrustedProxiesCSV string

// Check if an IP is a trusted proxy (supports both single IPs and CIDRs)
func (c *Controller) IsTrustedProxy(ip string) bool {
	// Parse the input IP
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}

	trusted := false
	c.TrustedProxies.Range(func(key, value any) bool {
		proxy, ok := value.(TrustedProxy)
		if !ok {
			return true
		}

		// Check if it's a CIDR notation
		if strings.Contains(proxy.IP, "/") {
			_, cidr, err := net.ParseCIDR(proxy.IP)
			if err == nil && cidr.Contains(parsedIP) {
				trusted = true
				return false // stop iteration
			}
		} else {
			// Direct IP comparison
			if proxy.IP == ip {
				trusted = true
				return false // stop iteration
			}
		}
		return true
	})

	return trusted
}

// TrustedProxyExists checks if a trusted proxy entry exists by its exact IP/CIDR key
func (c *Controller) TrustedProxyExists(ip string) bool {
	_, exists := c.TrustedProxies.Load(ip)
	return exists
}

// Add a trusted proxy by IP
func (c *Controller) AddTrustedProxy(ip string, desc string) bool {
	_, exists := c.TrustedProxies.Load(ip)
	if !exists {
		c.TrustedProxies.Store(ip, TrustedProxy{IP: ip, Desc: desc})
	}
	return !exists
}

// Remove a trusted proxy by IP
func (c *Controller) RemoveTrustedProxy(ip string) bool {
	_, exists := c.TrustedProxies.Load(ip)
	if exists {
		c.TrustedProxies.Delete(ip)
	}
	return exists
}

// List all trusted proxies
func (c *Controller) ListTrustedProxies() []TrustedProxy {
	var proxies []TrustedProxy
	c.TrustedProxies.Range(func(key, value any) bool {
		proxy, ok := value.(TrustedProxy)
		if ok {
			proxies = append(proxies, proxy)
		}
		return true
	})
	return proxies
}

// Load trusted proxies from file
func (c *Controller) LoadTrustedProxies() error {
	trustedProxies := []TrustedProxy{}

	if !utils.FileExists(c.Options.TrustedProxiesFile) {
		// Load default trusted proxies from embedded CSV and create the config file
		defaultProxies := loadDefaultTrustedProxiesFromCSV()
		data, err := json.MarshalIndent(defaultProxies, "", "  ")
		if err != nil {
			return err
		}
		err = os.WriteFile(c.Options.TrustedProxiesFile, data, 0775)
		if err != nil {
			return err
		}
		// Load into memory
		for _, proxy := range defaultProxies {
			c.TrustedProxies.Store(proxy.IP, proxy)
		}
		return nil
	}

	//Load from file
	data, err := os.ReadFile(c.Options.TrustedProxiesFile)
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, &trustedProxies)
	if err != nil {
		return err
	}

	// Load into memory
	for _, proxy := range trustedProxies {
		c.TrustedProxies.Store(proxy.IP, proxy)
	}

	return nil
}

// loadDefaultTrustedProxiesFromCSV parses the embedded CSV and returns TrustedProxy slice
func loadDefaultTrustedProxiesFromCSV() []TrustedProxy {
	var proxies []TrustedProxy
	scanner := bufio.NewScanner(strings.NewReader(defaultTrustedProxiesCSV))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		proxies = append(proxies, TrustedProxy{
			IP:   line,
			Desc: "Cloudflare",
		})
	}
	return proxies
}

// Save trusted proxies to file
func (c *Controller) SaveTrustedProxies() error {
	trustedProxies := c.ListTrustedProxies()
	data, err := json.MarshalIndent(trustedProxies, "", "  ")
	if err != nil {
		return err
	}

	err = os.WriteFile(c.Options.TrustedProxiesFile, data, 0775)
	return err
}

// GetClientIP returns the client IP based on the access rule's trusted proxy config
func (s *AccessRule) GetClientIP(r *http.Request) string {
	if s.TrustProxyHeadersOnly {
		return netutils.GetRequesterIP(r)
	}
	return netutils.GetRequesterIPUntrusted(r)
}
