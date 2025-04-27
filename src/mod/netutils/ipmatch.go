package netutils

import (
	"net"
	"net/http"
	"strings"
)

/*
	MatchIP.go

	This script contains function for matching IP address, comparing
	CIDR and IPv4 / v6 validations
*/

func GetRequesterIP(r *http.Request) string {
	ip := r.Header.Get("X-Real-Ip")
	if ip == "" {
		CF_Connecting_IP := r.Header.Get("CF-Connecting-IP")
		Fastly_Client_IP := r.Header.Get("Fastly-Client-IP")
		if CF_Connecting_IP != "" {
			//Use CF Connecting IP
			return CF_Connecting_IP
		} else if Fastly_Client_IP != "" {
			//Use Fastly Client IP
			return Fastly_Client_IP
		}
		ip = r.Header.Get("X-Forwarded-For")
	}
	if ip == "" {
		ip = r.RemoteAddr
	}

	/*
		Possible shits that might be extracted by this code
		127.0.0.1:61001
		[15c4:cbb4:cc98:4291:ffc1:3a46:06a1:51a7]:61002
		127.0.0.1
		158.250.160.114,109.21.249.211
		[15c4:cbb4:cc98:4291:ffc1:3a46:06a1:51a7],109.21.249.211

		We need to extract just the first ip address
	*/
	requesterRawIp := ip
	if strings.Contains(requesterRawIp, ",") {
		//Trim off all the forwarder IPs
		requesterRawIp = strings.Split(requesterRawIp, ",")[0]
	}

	//Trim away the port number
	reqHost, _, err := net.SplitHostPort(requesterRawIp)
	if err == nil {
		requesterRawIp = reqHost
	}

	if strings.HasPrefix(requesterRawIp, "[") && strings.HasSuffix(requesterRawIp, "]") {
		//e.g. [15c4:cbb4:cc98:4291:ffc1:3a46:06a1:51a7]
		requesterRawIp = requesterRawIp[1 : len(requesterRawIp)-1]
	}

	return requesterRawIp
}

// Match the IP address with a wildcard string
func MatchIpWildcard(ipAddress, wildcard string) bool {
	// Split IP address and wildcard into octets
	ipOctets := strings.Split(ipAddress, ".")
	wildcardOctets := strings.Split(wildcard, ".")

	// Check that both have 4 octets
	if len(ipOctets) != 4 || len(wildcardOctets) != 4 {
		return false
	}

	// Check each octet to see if it matches the wildcard or is an exact match
	for i := 0; i < 4; i++ {
		if wildcardOctets[i] == "*" {
			continue
		}
		if ipOctets[i] != wildcardOctets[i] {
			return false
		}
	}

	return true
}

// Match ip address with CIDR
func MatchIpCIDR(ip string, cidr string) bool {
	// Trim away scope ID if present in IP (e.g. fe80::1%eth0)
	if i := strings.Index(ip, "%"); i != -1 {
		ip = ip[:i]
	}

	// parse the CIDR string
	_, cidrnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return false
	}

	// parse the IP address
	ipAddr := net.ParseIP(ip)

	// check if the IP address is within the CIDR range
	return cidrnet.Contains(ipAddr)
}

// Check if a ip is private IP range
func IsPrivateIP(ipStr string) bool {
	if ipStr == "127.0.0.1" || ipStr == "::1" {
		//local loopback
		return true
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	return ip.IsPrivate()
}

// Check if an Ip string is ipv6
func IsIPv6(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	return ip.To4() == nil && ip.To16() != nil
}

// Check if an Ip string is ipv6
func IsIPv4(ipStr string) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}

	return ip.To4() != nil
}
