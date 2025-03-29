package access

import (
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

const (
	PUBLIC_IP_CHECK_URL = "http://checkip.amazonaws.com/"
)

// Start the public IP address updater
func (c *Controller) StartPublicIPUpdater() {
	stopChan := make(chan bool)
	c.publicIpTickerStop = stopChan
	ticker := time.NewTicker(time.Duration(c.Options.PublicIpCheckInterval) * time.Second)
	go func() {
		for {
			select {
			case <-c.publicIpTickerStop:
				ticker.Stop()
				return
			case <-ticker.C:
				err := c.UpdatePublicIP()
				if err != nil {
					c.Options.Logger.PrintAndLog("access", "Unable to update public IP address", err)
				}
			}
		}
	}()

	c.publicIpTicker = ticker
}

// Stop the public IP address updater
func (c *Controller) StopPublicIPUpdater() {
	// Stop the public IP address updater
	if c.publicIpTickerStop != nil {
		c.publicIpTickerStop <- true
	}
	c.publicIpTicker = nil
	c.publicIpTickerStop = nil
}

// Update the public IP address of the server
func (c *Controller) UpdatePublicIP() error {
	req, err := http.NewRequest("GET", PUBLIC_IP_CHECK_URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/58.0.3029.110 Safari/537.3")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("sec-ch-ua", `"Chromium";v="91", " Not;A Brand";v="99", "Google Chrome";v="91"`)
	req.Header.Set("sec-ch-ua-platform", `"Windows"`)
	req.Header.Set("sec-ch-ua-mobile", "?0")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	ip, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Validate if the returned byte is a valid IP address
	pubIP := net.ParseIP(strings.TrimSpace(string(ip)))
	if pubIP == nil {
		return errors.New("invalid IP address")
	}

	c.ServerPublicIP = pubIP.String()
	c.Options.Logger.PrintAndLog("access", "Public IP address updated to: "+c.ServerPublicIP, nil)
	return nil
}

func (c *Controller) IsLoopbackRequest(ipAddr string) bool {
	loopbackIPs := []string{
		"localhost",
		"::1",
		"127.0.0.1",
	}

	// Check if the request is loopback from public IP
	if ipAddr == c.ServerPublicIP {
		return true
	}

	// Check if the request is from localhost or loopback IPv4 or 6
	for _, loopbackIP := range loopbackIPs {
		if ipAddr == loopbackIP {
			return true
		}
	}

	return false
}

// Check if the IP address is in private IP range
func (c *Controller) IsPrivateIPRange(ipAddr string) bool {
	privateIPBlocks := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"169.254.0.0/16",
		"127.0.0.0/8",
		"::1/128",
		"fc00::/7",
		"fe80::/10",
	}

	for _, cidr := range privateIPBlocks {
		_, block, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		ip := net.ParseIP(ipAddr)
		if block.Contains(ip) {
			return true
		}
	}

	return false
}
