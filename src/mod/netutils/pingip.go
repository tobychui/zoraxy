package netutils

import (
	"fmt"
	"net"
	"time"
)

// TCP ping
func TCPPing(ipOrDomain string) (time.Duration, error) {
	start := time.Now()

	conn, err := net.DialTimeout("tcp", ipOrDomain+":80", 3*time.Second)
	if err != nil {
		return 0, fmt.Errorf("failed to establish TCP connection: %v", err)
	}
	defer conn.Close()

	elapsed := time.Since(start)
	pingTime := elapsed.Round(time.Millisecond)

	return pingTime, nil
}

// UDP Ping
func UDPPing(ipOrDomain string) (time.Duration, error) {
	start := time.Now()

	conn, err := net.DialTimeout("udp", ipOrDomain+":80", 3*time.Second)
	if err != nil {
		return 0, fmt.Errorf("failed to establish UDP connection: %v", err)
	}
	defer conn.Close()

	elapsed := time.Since(start)
	pingTime := elapsed.Round(time.Millisecond)

	return pingTime, nil
}

// Traditional ICMP ping
func PingIP(ipOrDomain string) (string, time.Duration, int, error) {
	ipAddr, err := net.ResolveIPAddr("ip", ipOrDomain)
	if err != nil {
		return "", 0, 0, fmt.Errorf("failed to resolve IP address: %v", err)
	}

	ip := ipAddr.IP.String()

	start := time.Now()

	conn, err := net.Dial("ip:icmp", ip)
	if err != nil {
		return ip, 0, 0, fmt.Errorf("failed to establish ICMP connection: %v", err)
	}
	defer conn.Close()

	icmpMsg := []byte{8, 0, 0, 0, 0, 1, 0, 0}
	_, err = conn.Write(icmpMsg)
	if err != nil {
		return ip, 0, 0, fmt.Errorf("failed to send ICMP message: %v", err)
	}

	reply := make([]byte, 1500)
	err = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	if err != nil {
		return ip, 0, 0, fmt.Errorf("failed to set read deadline: %v", err)
	}

	_, err = conn.Read(reply)
	if err != nil {
		return ip, 0, 0, fmt.Errorf("failed to read ICMP reply: %v", err)
	}

	elapsed := time.Since(start)
	pingTime := elapsed.Round(time.Millisecond)

	ttl := int(reply[8])

	return ip, pingTime, ttl, nil
}
