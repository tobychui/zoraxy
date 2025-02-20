package loadbalance

import (
	"strconv"
	"strings"
)

// Return if the target host is online
func (m *RouteManager) IsTargetOnline(upstreamIP string) bool {
	value, ok := m.OnlineStatus.Load(upstreamIP)
	if !ok {
		// Assume online if not found, also update the map
		m.OnlineStatus.Store(upstreamIP, true)
		return true
	}

	isOnline, ok := value.(bool)
	return ok && isOnline
}

// Notify the host online state, should be called from uptime monitor
func (m *RouteManager) NotifyHostOnlineState(upstreamIP string, isOnline bool) {
	//if the upstream IP contains http or https, strip it
	upstreamIP = strings.TrimPrefix(upstreamIP, "http://")
	upstreamIP = strings.TrimPrefix(upstreamIP, "https://")

	//Check previous state and update
	if m.IsTargetOnline(upstreamIP) == isOnline {
		return
	}

	m.OnlineStatus.Store(upstreamIP, isOnline)
	m.println("Updating upstream "+upstreamIP+" online state to "+strconv.FormatBool(isOnline), nil)
}

// Set this host unreachable for a given amount of time defined in timeout
// this shall be used in passive fallback. The uptime monitor should call to NotifyHostOnlineState() instead
/*
func (m *RouteManager) NotifyHostUnreachableWithTimeout(upstreamIp string, timeout int64) {
	//if the upstream IP contains http or https, strip it
	upstreamIp = strings.TrimPrefix(upstreamIp, "http://")
	upstreamIp = strings.TrimPrefix(upstreamIp, "https://")
	if timeout <= 0 {
		//Set to the default timeout
		timeout = 60
	}

	if !m.IsTargetOnline(upstreamIp) {
		//Already offline
		return
	}

	m.OnlineStatus.Store(upstreamIp, false)
	m.println("Setting upstream "+upstreamIp+" unreachable for "+strconv.FormatInt(timeout, 10)+"s", nil)
	go func() {
		//Set the upstream back to online after the timeout
		<-time.After(time.Duration(timeout) * time.Second)
		m.NotifyHostOnlineState(upstreamIp, true)
	}()
}
*/

// FilterOfflineOrigins return only online origins from a list of origins
func (m *RouteManager) FilterOfflineOrigins(origins []*Upstream) []*Upstream {
	var onlineOrigins []*Upstream
	for _, origin := range origins {
		if m.IsTargetOnline(origin.OriginIpOrDomain) {
			onlineOrigins = append(onlineOrigins, origin)
		}
	}

	return onlineOrigins
}
