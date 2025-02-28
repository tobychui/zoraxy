package ganserv

import (
	"net"
)

//Generate all ip address from a CIDR
func GetAllAddressFromCIDR(cidr string) ([]string, error) {
	ip, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	var ips []string
	for ip := ip.Mask(ipnet.Mask); ipnet.Contains(ip); inc(ip) {
		ips = append(ips, ip.String())
	}
	// remove network address and broadcast address
	return ips[1 : len(ips)-1], nil
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func isValidIPAddr(ipAddr string) bool {
	ip := net.ParseIP(ipAddr)
	if ip == nil {
		return false
	}

	return true
}

func ipWithinCIDR(ipAddr string, cidr string) bool {
	// Parse the CIDR string
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}

	// Parse the IP address
	ip := net.ParseIP(ipAddr)
	if ip == nil {
		return false
	}

	// Check if the IP address is in the CIDR range
	return ipNet.Contains(ip)
}
