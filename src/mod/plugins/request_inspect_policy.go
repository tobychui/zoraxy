package plugins

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
)

/*
	Request inspection policy

	Plugins declaring IntroSpect.RequestInspectPolicy serve a complete
	control-plane policy snapshot. Zoraxy validates it, caches it for at most
	inspectPolicyTTL and enforces mode, limits and failure behavior itself,
	so a misbehaving inspector cannot turn report mode into blocking.
*/

const (
	inspectModeInherit = "inherit"
	inspectModeOff     = "off"
	inspectModeReport  = "report"
	inspectModeBlock   = "block"

	inspectPolicyMaxBytes   = 128 * 1024
	defaultInspectBodyKB    = 64
	maxInspectBodyKB        = 1024
	defaultInspectTimeoutMS = 5000
	minInspectTimeoutMS     = 100
	maxInspectTimeoutMS     = 30000
)

var requestInspectPolicyClient = &http.Client{
	Timeout:       time.Second,
	Transport:     &http.Transport{}, //Loopback IPC must not use environment HTTP proxies.
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// inspectSettings is the enforcement for one request and inspector.
type inspectSettings struct {
	mode           string
	failClosed     bool
	maxBody        int //bytes
	bodyTimeout    time.Duration
	rejectOversize bool //block mode answers 413 instead of inspecting a prefix
}

// inspectPolicy is a validated policy. It is never modified after parsing.
type inspectPolicy struct {
	zoraxy_plugin.RequestInspectPolicy
	invert map[string]bool
}

func validInspectKey(key string) bool {
	//The default-site sentinel is a route identity, not a path prefix.
	return key == "/" || (key != "" && key == strings.ToLower(key) && strings.IndexFunc(key, unicode.IsSpace) < 0 && !strings.ContainsAny(key, "/\\"))
}

func validInspectLimits(maxBodyKB, timeoutMS int) error {
	if maxBodyKB != 0 && (maxBodyKB < 1 || maxBodyKB > maxInspectBodyKB) {
		return fmt.Errorf("max_body_kb %d outside 1-%d", maxBodyKB, maxInspectBodyKB)
	}
	if timeoutMS != 0 && (timeoutMS < minInspectTimeoutMS || timeoutMS > maxInspectTimeoutMS) {
		return fmt.Errorf("body_timeout_ms %d outside %d-%d", timeoutMS, minInspectTimeoutMS, maxInspectTimeoutMS)
	}
	return nil
}

// parseInspectPolicy strictly decodes and validates a policy, filling in defaults.
func parseInspectPolicy(data []byte) (*inspectPolicy, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 || data[0] != '{' {
		return nil, errors.New("policy is not a JSON object")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	policy := &inspectPolicy{}
	if err := decoder.Decode(&policy.RequestInspectPolicy); err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return nil, errors.New("unexpected data after policy object")
	}

	switch policy.Mode {
	case "":
		policy.Mode = inspectModeReport
	case inspectModeOff, inspectModeReport, inspectModeBlock:
	default:
		return nil, fmt.Errorf("invalid mode %q", policy.Mode)
	}
	if err := validInspectLimits(policy.MaxBodyKB, policy.BodyTimeoutMS); err != nil {
		return nil, err
	}
	if policy.MaxBodyKB == 0 {
		policy.MaxBodyKB = defaultInspectBodyKB
	}
	if policy.BodyTimeoutMS == 0 {
		policy.BodyTimeoutMS = defaultInspectTimeoutMS
	}
	policy.invert = make(map[string]bool)
	for _, key := range policy.InvertFailureHosts {
		if !validInspectKey(key) {
			return nil, fmt.Errorf("invalid invert_failure_hosts entry %q", key)
		}
		policy.invert[key] = true
	}
	for key, host := range policy.Hosts {
		if !validInspectKey(key) {
			return nil, fmt.Errorf("invalid hosts key %q", key)
		}
		switch host.Mode {
		case "", inspectModeInherit, inspectModeOff, inspectModeReport, inspectModeBlock:
		default:
			return nil, fmt.Errorf("invalid mode %q for host %q", host.Mode, key)
		}
		if err := validInspectLimits(host.MaxBodyKB, host.BodyTimeoutMS); err != nil {
			return nil, fmt.Errorf("host %q: %w", key, err)
		}
	}
	return policy, nil
}

// settingsFor resolves the policy for a lowercase route ID and canonical host.
// A host entry for the route ID takes precedence over one for the host name.
func (p *inspectPolicy) settingsFor(routeID, host string) inspectSettings {
	settings := inspectSettings{
		mode:        p.Mode,
		failClosed:  p.FailClosed,
		maxBody:     p.MaxBodyKB * 1024,
		bodyTimeout: time.Duration(p.BodyTimeoutMS) * time.Millisecond,
	}
	override, ok := p.Hosts[routeID]
	if !ok {
		override, ok = p.Hosts[host]
	}
	if ok {
		if override.Mode != "" && override.Mode != inspectModeInherit {
			settings.mode = override.Mode
		}
		if override.MaxBodyKB != 0 {
			settings.maxBody = override.MaxBodyKB * 1024
		}
		if override.BodyTimeoutMS != 0 {
			settings.bodyTimeout = time.Duration(override.BodyTimeoutMS) * time.Millisecond
		}
	}
	if p.invert[routeID] || p.invert[host] {
		settings.failClosed = !settings.failClosed
	}
	settings.rejectOversize = settings.mode == inspectModeBlock
	return settings
}

// canonicalInspectHost returns the lowercase request host without port or trailing dot.
func canonicalInspectHost(hostport string) string {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.Trim(host, "[]"), ".")
	return strings.ToLower(host)
}

// inspectPolicyCache keeps the last-known-good policy of one plugin.
type inspectPolicyCache struct {
	mu        sync.Mutex
	refreshed time.Time                  //start of the last refresh attempt
	current   *inspectPolicy             //last-known-good policy, nil on a fresh install
	err       error                      //result of the last refresh attempt
	persist   func(*inspectPolicy) error //Persist only validated policies
}

// load returns the cached policy, refreshing it when older than inspectPolicyTTL.
// On a failed refresh it returns the error together with the last-known-good policy.
func (c *inspectPolicyCache) load(port int, path string) (*inspectPolicy, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := time.Now()
	if !c.refreshed.IsZero() && now.Sub(c.refreshed) < inspectPolicyTTL {
		return c.current, c.err
	}
	c.refreshed = now
	policy, err := fetchInspectPolicy(port, path)
	c.err = err
	if err == nil {
		c.current = policy
		if c.persist != nil {
			err = c.persist(policy)
			if err != nil {
				err = fmt.Errorf("persist inspection policy: %w", err)
			}
			c.err = err
		}
	}
	return c.current, err
}

func fetchInspectPolicy(port int, path string) (*inspectPolicy, error) {
	target := url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(port), Path: path}
	resp, err := requestInspectPolicyClient.Get(target.String())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("policy returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, inspectPolicyMaxBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > inspectPolicyMaxBytes {
		return nil, fmt.Errorf("policy exceeds %d bytes", inspectPolicyMaxBytes)
	}
	return parseInspectPolicy(data)
}
