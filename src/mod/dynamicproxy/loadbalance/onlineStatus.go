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

func (m *RouteManager) SetTargetOffline() {

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

// StartHeartbeats start pinging each server every minutes to make sure all targets are online
// Active mode only
/*
func (m *RouteManager) StartHeartbeats(pingTargets []*FallbackProxyTarget) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	fmt.Println("Heartbeat started")
	go func() {
		for {
			select {
			case <-m.onlineStatusTickerStop:
				ticker.Stop()
				return
			case <-ticker.C:
				for _, target := range pingTargets {
					go func(target *FallbackProxyTarget) {
						isOnline := PingTarget(target.MatchingDomainOrIp, target.RequireTLS)
						m.LoadBalanceMap.Store(target.MatchingDomainOrIp, isOnline)
					}(target)
				}
			}
		}
	}()
}
*/
