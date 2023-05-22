package ganserv

import (
	"fmt"
	"math/rand"
	"net"
	"time"
)

//Get a random free IP from the pool
func (n *Network) GetRandomFreeIP() (net.IP, error) {
	// Get all IP addresses in the subnet
	ips, err := GetAllAddressFromCIDR(n.CIDR)
	if err != nil {
		return nil, err
	}

	// Filter out used IPs
	usedIPs := make(map[string]bool)
	for _, node := range n.Nodes {
		usedIPs[node.ManagedIP.String()] = true
	}
	availableIPs := []string{}
	for _, ip := range ips {
		if !usedIPs[ip] {
			availableIPs = append(availableIPs, ip)
		}
	}

	// Randomly choose an available IP
	if len(availableIPs) == 0 {
		return nil, fmt.Errorf("no available IP")
	}
	rand.Seed(time.Now().UnixNano())
	randIndex := rand.Intn(len(availableIPs))
	pickedFreeIP := availableIPs[randIndex]

	return net.ParseIP(pickedFreeIP), nil
}
