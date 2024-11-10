package sshprox

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Rewrite url based on proxy root (default site)
func RewriteURL(rooturl string, requestURL string) (*url.URL, error) {
	rewrittenURL := strings.TrimPrefix(requestURL, rooturl)
	return url.Parse(rewrittenURL)
}

// Check if the current platform support web.ssh function
func IsWebSSHSupported() bool {
	//Check if the binary exists in system/gotty/
	binary := "gotty_" + runtime.GOOS + "_" + runtime.GOARCH

	if runtime.GOOS == "windows" {
		binary = binary + ".exe"
	}

	//Check if the target gotty terminal exists
	f, err := gotty.Open("gotty/" + binary)
	if err != nil {
		return false
	}

	f.Close()
	return true
}

// Get the next free port in the list
func (m *Manager) GetNextPort() int {
	nextPort := m.StartingPort
	occupiedPort := make(map[int]bool)
	for _, instance := range m.Instances {
		occupiedPort[instance.AssignedPort] = true
	}
	for {
		if !occupiedPort[nextPort] {
			return nextPort
		}
		nextPort++
	}
}

// Check if a given domain and port is a valid ssh server
func IsSSHConnectable(ipOrDomain string, port int) bool {
	timeout := time.Second * 3
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("%s:%d", ipOrDomain, port), timeout)
	if err != nil {
		return false
	}
	defer conn.Close()

	// Send an SSH version identification string to the server to check if it's SSH
	_, err = conn.Write([]byte("SSH-2.0-Go\r\n"))
	if err != nil {
		return false
	}

	// Wait for a response from the server
	buf := make([]byte, 1024)
	_, err = conn.Read(buf)
	if err != nil {
		return false
	}

	// Check if the response starts with "SSH-2.0"
	return string(buf[:7]) == "SSH-2.0"
}

// Validate the username and remote address to prevent injection
func ValidateUsernameAndRemoteAddr(username string, remoteIpAddr string) error {
	// Validate and sanitize the username to prevent ssh injection
	validUsername := regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	if !validUsername.MatchString(username) {
		return errors.New("invalid username, only alphanumeric characters, dots, underscores and dashes are allowed")
	}

	//Check if the remoteIpAddr is a valid ipv4 or ipv6 address
	if net.ParseIP(remoteIpAddr) != nil {
		//A valid IP address do not need further validation
		return nil
	}

	// Validate and sanitize the remote domain to prevent injection
	validRemoteAddr := regexp.MustCompile(`^[a-zA-Z0-9._-]+$`)
	if !validRemoteAddr.MatchString(remoteIpAddr) {
		return errors.New("invalid remote address, only alphanumeric characters, dots, underscores and dashes are allowed")
	}

	return nil
}

// Check if the given ip or domain is a loopback address
// or resolves to a loopback address
func IsLoopbackIPOrDomain(ipOrDomain string) bool {
	if strings.EqualFold(strings.TrimSpace(ipOrDomain), "localhost") || strings.TrimSpace(ipOrDomain) == "127.0.0.1" {
		return true
	}

	//Check if the ipOrDomain resolves to a loopback address
	ips, err := net.LookupIP(ipOrDomain)
	if err != nil {
		return false
	}

	for _, ip := range ips {
		if ip.IsLoopback() {
			return true
		}
	}

	return false
}
