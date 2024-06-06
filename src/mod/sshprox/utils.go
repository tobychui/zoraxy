package sshprox

import (
	"fmt"
	"net"
	"net/url"
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

// Check if the port is used by other process or application
func isPortInUse(port int) bool {
	address := fmt.Sprintf(":%d", port)
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return true
	}
	listener.Close()
	return false
}
