package plugins

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
)

// inspectPolicyTTL bounds how long a fetched policy is used before refreshing it.
var inspectPolicyTTL = time.Second

var requestInspectClient = &http.Client{
	Timeout: 2 * time.Second,
	Transport: &http.Transport{
		// Keep burst connections idle rather than re-dialing above Go's default
		// two-per-host pool. No active-connection cap: admission stays in the plugin.
		// Proxy remains nil so loopback IPC cannot use environment proxies.
		// Idle timeout stays below the plugin server's 30 s IdleTimeout so the
		// client drops a pooled connection before the server closes it; an
		// inspection POST is not automatically replayed after a write.
		MaxIdleConns: 256, MaxIdleConnsPerHost: 64, IdleConnTimeout: 25 * time.Second,
	},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// inspectBody preserves the original body's Close method while replaying its prefix.
type inspectBody struct {
	io.Reader
	io.Closer
}

type inspectReadError struct{ err error }

func (r inspectReadError) Read([]byte) (int, error) { return 0, r.err }

// RequestInspectTarget identifies the proxy route a request was matched to.
type RequestInspectTarget struct {
	RouteID  string   //RootOrMatchingDomain of the matched proxy endpoint
	Tags     []string //Endpoint tags selecting the inspector plugins
	ClientIP string   //Trusted-proxy resolved client IP
}

// inspectorRef is an immutable copy of the plugin state needed to call one inspector.
type inspectorRef struct {
	id          string
	port        int
	inspectPath string
	policyPath  string              //empty for legacy inspectors configured by spec fields
	policy      *inspectPolicyCache //shared across snapshots of the same plugin
	maxBodyKB   int
	failClosed  bool
	unavailable bool //Failed process; never call its possibly reassigned port
}

// inspectSnapshot maps tags to inspectors. It is never modified after publication,
// so request handling cannot observe a half-applied enable, disable or restart.
type inspectSnapshot struct {
	tags map[string][]*inspectorRef
}

// publishInspectorsLocked rebuilds the inspector selection from the current
// plugin runtime state. The caller must hold m.lifecycleMutex.
func (m *Manager) publishInspectorsLocked() {
	snapshot := &inspectSnapshot{tags: make(map[string][]*inspectorRef)}
	refs := make(map[*Plugin]*inspectorRef)
	for tag, pluginList := range m.tagPluginList {
		for _, p := range pluginList {
			if (!p.Enabled && !p.inspectFailed) || (p.AssignedPort == 0 && !p.inspectFailed) || p.Spec == nil || p.Spec.RequestInspect == "" {
				continue
			}
			ref, ok := refs[p]
			if !ok {
				ref = &inspectorRef{
					id:          p.Spec.ID,
					port:        p.AssignedPort,
					inspectPath: p.Spec.RequestInspect,
					policyPath:  p.Spec.RequestInspectPolicy,
					maxBodyKB:   p.Spec.RequestInspectMaxBody,
					failClosed:  p.Spec.RequestInspectFailClosed,
					unavailable: p.inspectFailed,
				}
				if ref.policyPath != "" {
					//Keep the last-known-good policy across restarts of the plugin
					if m.inspectPolicy == nil {
						m.inspectPolicy = make(map[string]*inspectPolicyCache)
					}
					if m.inspectPolicy[ref.id] == nil {
						m.inspectPolicy[ref.id] = m.newInspectPolicyCache(p)
					}
					ref.policy = m.inspectPolicy[ref.id]
				}
				refs[p] = ref
			}
			snapshot.tags[tag] = append(snapshot.tags[tag], ref)
		}
	}
	m.inspectors.Store(snapshot)
}

// settings returns the enforcement for a request. Legacy inspectors block with
// their spec limits. On a policy refresh error the settings of the
// last-known-good policy are returned with the error; without one, fail open.
func (p *inspectorRef) settings(routeID, host string) (inspectSettings, error) {
	if p.policyPath == "" {
		limitKB := p.maxBodyKB
		if limitKB <= 0 {
			limitKB = defaultInspectBodyKB
		} else if limitKB > maxInspectBodyKB {
			limitKB = maxInspectBodyKB
		}
		return inspectSettings{
			mode:        inspectModeBlock,
			failClosed:  p.failClosed,
			maxBody:     limitKB * 1024,
			bodyTimeout: defaultInspectTimeoutMS * time.Millisecond,
		}, p.unavailableError()
	}
	if p.unavailable {
		p.policy.mu.Lock()
		policy := p.policy.current
		p.policy.mu.Unlock()
		if policy == nil {
			return inspectSettings{mode: inspectModeReport}, p.unavailableError()
		}
		return policy.settingsFor(routeID, host), p.unavailableError()
	}
	policy, err := p.policy.load(p.port, p.policyPath)
	if policy == nil {
		return inspectSettings{mode: inspectModeReport}, err
	}
	return policy.settingsFor(routeID, host), err
}

func (p *inspectorRef) unavailableError() error {
	if p.unavailable {
		return errors.New("inspector process unavailable")
	}
	return nil
}

// InspectRequest returns true if an inspector has answered the request.
// It is kept for callers without route identity; see InspectRoute.
func (m *Manager) InspectRequest(w http.ResponseWriter, r *http.Request, tags []string, clientIP string) bool {
	handled, _ := m.InspectRoute(w, r, RequestInspectTarget{Tags: tags, ClientIP: clientIP})
	return handled
}

// InspectRoute runs the opt-in inspectors selected by the target's tags. If an
// inspector answered the request it returns true and the written status code;
// otherwise the request continues through the normal routing chain.
func (m *Manager) InspectRoute(w http.ResponseWriter, r *http.Request, target RequestInspectTarget) (bool, int) {
	snapshot := m.inspectors.Load()
	if snapshot == nil {
		return false, 0
	}
	var inspectors []*inspectorRef
	seen := make(map[*inspectorRef]bool)
	for _, tag := range target.Tags {
		for _, ref := range snapshot.tags[tag] {
			if !seen[ref] {
				inspectors = append(inspectors, ref)
				seen[ref] = true
			}
		}
	}
	if len(inspectors) == 0 {
		return false, 0
	}

	routeID := strings.ToLower(target.RouteID)
	host := canonicalInspectHost(r.Host)
	for _, ref := range inspectors {
		settings, err := ref.settings(routeID, host)
		if err != nil {
			m.logInspect("Request inspection policy unavailable: "+ref.id, err)
			if settings.mode == inspectModeBlock && settings.failClosed {
				http.Error(w, "Request inspection unavailable", http.StatusServiceUnavailable)
				return true, http.StatusServiceUnavailable
			}
			continue
		}
		if settings.mode == inspectModeOff {
			continue
		}

		result, err := ref.inspectRequest(w, r, target, settings)
		switch {
		case isInspectBodyTimeout(err):
			//The upload stalled: its remainder cannot be forwarded intact.
			m.logInspect("Request body read timed out before inspection: "+ref.id+" route="+target.RouteID, err)
			w.Header().Set("Connection", "close")
			http.Error(w, "Request body timeout", http.StatusRequestTimeout)
			return true, http.StatusRequestTimeout
		case errors.Is(err, errInspectOversize):
			http.Error(w, "Request body too large for inspection", http.StatusRequestEntityTooLarge)
			return true, http.StatusRequestEntityTooLarge
		case err != nil:
			m.logInspect("Request inspection failed: "+ref.id, err)
			if settings.mode == inspectModeBlock && settings.failClosed {
				http.Error(w, "Request inspection unavailable", http.StatusServiceUnavailable)
				return true, http.StatusServiceUnavailable
			}
		case result.Action == "deny" && settings.mode == inspectModeReport:
			m.logInspect("Request inspection would deny (report mode): "+ref.id+" route="+target.RouteID+" event="+result.EventID, nil)
		case result.Action == "deny":
			status := result.Status
			if status < 400 || status > 599 {
				status = http.StatusForbidden
			}
			if result.EventID != "" {
				m.logInspect("Request inspection denied: "+ref.id+" route="+target.RouteID+" event="+result.EventID, nil)
			}
			http.Error(w, result.Reason, status)
			return true, status
		}
	}
	return false, 0
}

func (m *Manager) logInspect(message string, err error) {
	if m.Options != nil && m.Options.Logger != nil {
		m.Log(message, err)
	} else {
		log.Printf("%s: %v", message, err)
	}
}

var (
	errInspectOversize            = errors.New("request body exceeds the inspection limit")
	errInspectDeadlineUnsupported = errors.New("request body read deadline unsupported")
)

// inspectBodyError is a failure to read the client's request body.
type inspectBodyError struct{ err error }

func (e *inspectBodyError) Error() string { return "request body read: " + e.err.Error() }
func (e *inspectBodyError) Unwrap() error { return e.err }

// isInspectBodyTimeout reports whether the client upload hit the body read deadline.
func isInspectBodyTimeout(err error) bool {
	var bodyErr *inspectBodyError
	if !errors.As(err, &bodyErr) {
		return false
	}
	var netErr net.Error
	return errors.Is(bodyErr.err, os.ErrDeadlineExceeded) || (errors.As(bodyErr.err, &netErr) && netErr.Timeout())
}

func (p *inspectorRef) inspectRequest(w http.ResponseWriter, r *http.Request, target RequestInspectTarget, settings inspectSettings) (zoraxy_plugin.RequestInspectResult, error) {
	var result zoraxy_plugin.RequestInspectResult
	payload := zoraxy_plugin.RequestInspectPayload{
		DynamicSniffForwardRequest: zoraxy_plugin.EncodeForwardRequestPayload(r, target.ClientIP),
		ProtocolVersion:            zoraxy_plugin.RequestInspectProtocolVersion,
		RouteID:                    target.RouteID,
		Body:                       []byte{},
	}
	limit := settings.maxBody
	if r.Body != nil && r.Body != http.NoBody {
		//Bound the prefix read with a connection read deadline. Without
		//deadline support the body is left untouched instead of reading it
		//unbounded or in a goroutine that could outlive the request.
		rc := http.NewResponseController(w)
		if err := rc.SetReadDeadline(time.Now().Add(settings.bodyTimeout)); err != nil {
			return result, fmt.Errorf("%w: %v", errInspectDeadlineUnsupported, err)
		}
		original := r.Body
		buf, err := io.ReadAll(io.LimitReader(original, int64(limit)+1))
		if resetErr := rc.SetReadDeadline(time.Time{}); resetErr != nil && err == nil {
			err = resetErr
		}
		replay := io.MultiReader(bytes.NewReader(buf), original)
		if err != nil {
			//Do not turn a failed upload into a successful partial request upstream.
			replay = io.MultiReader(bytes.NewReader(buf), inspectReadError{err}, original)
		}
		r.Body = &inspectBody{Reader: replay, Closer: original}
		if err != nil {
			return result, &inspectBodyError{err}
		}
		payload.BodyTruncated = len(buf) > limit
		if payload.BodyTruncated {
			if settings.rejectOversize {
				return result, errInspectOversize
			}
			buf = buf[:limit]
		}
		payload.Body = buf
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return result, err
	}
	inspectURL := url.URL{Scheme: "http", Host: "127.0.0.1:" + strconv.Itoa(p.port), Path: p.inspectPath}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, inspectURL.String(), bytes.NewReader(data))
	if err != nil {
		return result, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := requestInspectClient.Do(req)
	if err != nil {
		return result, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return result, fmt.Errorf("inspection returned HTTP %d", resp.StatusCode)
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, 4097))
	if err != nil {
		return result, err
	}
	if len(data) > 4096 {
		return result, fmt.Errorf("inspection response exceeds 4096 bytes")
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return result, err
	}
	if result.Action != "allow" && result.Action != "deny" {
		return result, fmt.Errorf("invalid inspection action %q", result.Action)
	}
	return result, nil
}
