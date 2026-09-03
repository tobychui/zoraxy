package statistic

import (
	"fmt"
	"net"
	"time"
	"unicode/utf8"
)

/*
	MaxStatKeyLength is the maximum length of a request supplied string (URL,
	Referer or User-Agent) used as a key in the daily summary maps. Anything
	longer is truncated so that a single client cannot inflate the size of the
	summary, and so the serialized summary stays a predictable size on disk.
*/
const MaxStatKeyLength = 256

// Truncate a request supplied string down to MaxStatKeyLength bytes, cutting
// on a rune boundary so the result stays valid UTF-8 for the JSON encoder
func truncateStatKey(key string) string {
	if len(key) <= MaxStatKeyLength {
		return key
	}

	truncated := key[:MaxStatKeyLength]
	for len(truncated) > 0 && !utf8.ValidString(truncated) {
		truncated = truncated[:len(truncated)-1]
	}
	return truncated
}

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
