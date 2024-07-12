package loadbalance

import (
	"net/http"
	"time"
)

// Return the last ping status to see if the target is online
func (m *RouteManager) IsTargetOnline(matchingDomainOrIp string) bool {
	value, ok := m.LoadBalanceMap.Load(matchingDomainOrIp)
	if !ok {
		return false
	}

	isOnline, ok := value.(bool)
	return ok && isOnline
}

// Ping a target to see if it is online
func PingTarget(targetMatchingDomainOrIp string, requireTLS bool) bool {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	url := targetMatchingDomainOrIp
	if requireTLS {
		url = "https://" + url
	} else {
		url = "http://" + url
	}

	resp, err := client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode >= 200 && resp.StatusCode <= 600
}
