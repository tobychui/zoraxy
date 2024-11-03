package ipscan

/*
	Port Scanner

	This module scan the given IP address and scan all the opened port

*/

import (
	"fmt"
	"net"
	"sync"
	"time"
)

// OpenedPort holds information about an open port and its service type
type OpenedPort struct {
	Port  int
	IsTCP bool
}

// ScanPorts scans all the opened ports on a given host IP (both IPv4 and IPv6)
func ScanPorts(host string) []*OpenedPort {
	var openPorts []*OpenedPort
	var wg sync.WaitGroup
	var mu sync.Mutex

	for port := 1; port <= 65535; port++ {
		wg.Add(1)
		go func(port int) {
			defer wg.Done()
			address := fmt.Sprintf("%s:%d", host, port)

			// Check TCP
			conn, err := net.DialTimeout("tcp", address, 5*time.Second)
			if err == nil {
				mu.Lock()
				openPorts = append(openPorts, &OpenedPort{Port: port, IsTCP: true})
				mu.Unlock()
				conn.Close()
			}
		}(port)
	}

	wg.Wait()
	return openPorts
}
