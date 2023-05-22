package ipscan

import (
	"bytes"
	"fmt"
	"net"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/go-ping/ping"
)

/*
	IP Scanner

	This module scan the given network range and return a list
	of nearby nodes.
*/

type DiscoveredHost struct {
	IP                string
	Ping              int
	Hostname          string
	HttpPortDetected  bool
	HttpsPortDetected bool
}

//Scan an IP range given the start and ending ip address
func ScanIpRange(start, end string) ([]*DiscoveredHost, error) {
	ipStart := net.ParseIP(start)
	ipEnd := net.ParseIP(end)
	if ipStart == nil || ipEnd == nil {
		return nil, fmt.Errorf("Invalid IP address")
	}

	if bytes.Compare(ipStart, ipEnd) > 0 {
		return nil, fmt.Errorf("Invalid IP range")
	}

	var wg sync.WaitGroup
	hosts := make([]*DiscoveredHost, 0)
	for ip := ipStart; bytes.Compare(ip, ipEnd) <= 0; inc(ip) {
		wg.Add(1)
		thisIp := ip.String()
		go func(thisIp string) {
			defer wg.Done()
			host := &DiscoveredHost{IP: thisIp}
			if err := host.CheckPing(); err != nil {
				// skip if the host is unreachable
				host.Ping = -1
				hosts = append(hosts, host)
				return
			}

			host.CheckHostname()
			host.CheckPort("http", 80, &host.HttpPortDetected)
			host.CheckPort("https", 443, &host.HttpsPortDetected)
			fmt.Println("OK", host)
			hosts = append(hosts, host)

		}(thisIp)
	}

	//Wait until all go routine done
	wg.Wait()
	sortByIP(hosts)
	return hosts, nil
}

func ScanCIDRRange(cidr string) ([]*DiscoveredHost, error) {
	_, ipNet, err := net.ParseCIDR(cidr)
	if err != nil {
		return nil, err
	}

	ip := ipNet.IP.To4()
	startIP := net.IPv4(ip[0], ip[1], ip[2], 1).String()
	endIP := net.IPv4(ip[0], ip[1], ip[2], 254).String()

	return ScanIpRange(startIP, endIP)
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}

func sortByIP(discovered []*DiscoveredHost) {
	sort.Slice(discovered, func(i, j int) bool {
		return discovered[i].IP < discovered[j].IP
	})
}

func (host *DiscoveredHost) CheckPing() error {
	// ping the host and set the ping time in milliseconds
	pinger, err := ping.NewPinger(host.IP)
	if err != nil {
		return err
	}
	pinger.Count = 4
	pinger.Timeout = time.Second
	pinger.SetPrivileged(true) // This line may help on some systems
	pinger.Run()
	stats := pinger.Statistics()
	if stats.PacketsRecv == 0 {
		return fmt.Errorf("Host unreachable for " + host.IP)
	}
	host.Ping = int(stats.AvgRtt.Milliseconds())
	return nil
}

func (host *DiscoveredHost) CheckHostname() {
	// lookup the hostname for the IP address
	names, err := net.LookupAddr(host.IP)
	fmt.Println(names, err)
	if err == nil && len(names) > 0 {
		host.Hostname = names[0]
	}
}

func (host *DiscoveredHost) CheckPort(protocol string, port int, detected *bool) {
	// try to connect to the specified port on the host
	conn, err := net.DialTimeout("tcp", net.JoinHostPort(host.IP, strconv.Itoa(port)), 1*time.Second)
	if err == nil {
		conn.Close()
		*detected = true
	}
}

func (host *DiscoveredHost) ScanPorts(startPort, endPort int) []int {
	var openPorts []int

	for port := startPort; port <= endPort; port++ {
		target := fmt.Sprintf("%s:%d", host.IP, port)
		conn, err := net.DialTimeout("tcp", target, time.Millisecond*500)
		if err == nil {
			conn.Close()
			openPorts = append(openPorts, port)
		}
	}

	return openPorts
}
