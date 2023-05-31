package statistic

import (
	"fmt"
	"net"
	"time"
)

func isWebPageExtension(ext string) bool {
	webPageExts := []string{".html", ".htm", ".php", ".jsp", ".aspx", ".js", ".jsx"}
	for _, e := range webPageExts {
		if e == ext {
			return true
		}
	}
	return false
}

func IsBeforeToday(dateString string) bool {
	layout := "2006_01_02"
	date, err := time.Parse(layout, dateString)
	if err != nil {
		fmt.Println("Error parsing date:", err)
		return false
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	return date.Before(today) || dateString == time.Now().Format(layout)
}

// Check if the IP string is a valid ip address
func IsValidIPAddress(ip string) bool {
	// Check if the string is a valid IPv4 address
	if parsedIP := net.ParseIP(ip); parsedIP != nil && parsedIP.To4() != nil {
		return true
	}

	// Check if the string is a valid IPv6 address
	if parsedIP := net.ParseIP(ip); parsedIP != nil && parsedIP.To16() != nil {
		return true
	}

	// If the string is neither a valid IPv4 nor IPv6 address, return false
	return false
}
