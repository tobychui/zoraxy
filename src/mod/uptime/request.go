package uptime

/*
	request.go

	HTTP request utilities and wrappers used by the uptime monitor
	to probe upstream targets.
*/

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"golang.org/x/net/publicsuffix"
)

// Get website stauts with latency given URL, return is conn succ and its latency and status code
func (m *Monitor) getWebsiteStatusWithLatency(target *Target, timeout time.Duration) (bool, int64, int) {
	start := time.Now().UnixNano() / int64(time.Millisecond)
	checkURL := buildHealthCheckURL(target.URL, target.HealthCheckURI)
	statusCode, err := m.getWebsiteStatus(checkURL, target.SkipTlsValidation, timeout, normalizeMethod(target.CheckMethod))
	end := time.Now().UnixNano() / int64(time.Millisecond)
	if err != nil {
		if m.Config.Verbal {
			m.Config.Logger.PrintAndLog(LOG_MODULE_NAME, "Ping upstream timeout. Assume offline", err)
		}

		// Check if this is the first record
		// sometime after startup the first check may fail due to network issues
		// we will log it as failed but not notify dynamic proxy to take down the upstream
		m.logMutex.RLock()
		records, ok := m.OnlineStatusLog[target.ID]
		m.logMutex.RUnlock()
		if !ok || len(records) == 0 {
			return false, 0, 0
		}

		// Otherwise assume offline
		m.Config.OnlineStateNotify(target.URL, false)
		return false, 0, 0
	}

	diff := end - start
	succ := false
	if statusCode >= 200 && statusCode < 300 {
		//OK
		succ = true
	} else if statusCode >= 300 && statusCode < 400 {
		//Redirection code
		succ = true
	} else {
		succ = false
	}
	m.Config.OnlineStateNotify(target.URL, true)
	return succ, diff, statusCode

}

func (m *Monitor) getWebsiteStatus(url string, skipTLSVerification bool, timeout time.Duration, method string) (int, error) {
	// Create a one-time use cookie jar to store cookies
	jar, err := cookiejar.New(&cookiejar.Options{PublicSuffixList: publicsuffix.List})
	if err != nil {
		return 0, err
	}

	// Need to explicitly disable keep-alives and set a short idle connection timeout to avoid hanging connections
	// See #1241 for details
	transport := &http.Transport{
		DisableKeepAlives: true,
		IdleConnTimeout:   15 * time.Second,
	}

	if skipTLSVerification {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		tlsDialer := &tls.Dialer{Config: &tls.Config{InsecureSkipVerify: true}}
		transport.DialTLSContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
			return tlsDialer.DialContext(ctx, network, addr)
		}
	}

	client := http.Client{
		Jar:       jar,
		Timeout:   timeout,
		Transport: transport,
	}

	// http.NewRequest returns a nil request when the URL (or method) is invalid,
	// see issue #1369 for more details
	req, err := http.NewRequest(method, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header = http.Header{
		"User-Agent": {UPTIME_MONITOR_USER_AGENT},
	}

	resp, err := client.Do(req)
	if err != nil {
		//Try replace the http with https and vise versa
		rewriteURL := ""
		if strings.Contains(url, "https://") {
			rewriteURL = strings.ReplaceAll(url, "https://", "http://")
		} else if strings.Contains(url, "http://") {
			rewriteURL = strings.ReplaceAll(url, "http://", "https://")
		}

		if m.Config.Verbal {
			m.Config.Logger.PrintAndLog(LOG_MODULE_NAME, fmt.Sprintf("Error pinging %s: %v, try swapping protocol to %s", url, err, rewriteURL), err)
		}

		req, reqErr := http.NewRequest(method, rewriteURL, nil)
		if reqErr != nil {
			return 0, err
		}
		req.Header = http.Header{
			"User-Agent": {UPTIME_MONITOR_USER_AGENT},
		}

		resp, err = client.Do(req)
		if err != nil {
			if strings.Contains(err.Error(), "http: server gave HTTP response to HTTPS client") {
				//Invalid downstream reverse proxy settings, but it is online
				//return SSL handshake failed
				return 525, nil
			}
			return 0, err
		}
	}
	defer resp.Body.Close()
	// Drain the response body so OS close the connection with FIN instead of RST
	io.Copy(io.Discard, resp.Body)
	status_code := resp.StatusCode
	return status_code, nil
}

// buildHealthCheckURL joins a base URL (e.g. "http://host:port") with an
// optional health-check URI path (e.g. "/identity"). If the URI is empty,
// the base URL is returned unchanged. The URI is sanitized so users can
// enter "identity", "/identity" or "/identity?token=foo" indifferently.
func buildHealthCheckURL(baseURL, uri string) string {
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return baseURL
	}
	// Strip trailing slashes from base, ensure leading slash on uri
	base := strings.TrimRight(baseURL, "/")
	if !strings.HasPrefix(uri, "/") {
		uri = "/" + uri
	}
	return base + uri
}
