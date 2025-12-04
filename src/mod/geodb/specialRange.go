package geodb

/*
	specialRange.go

	This script implement the reserved / special IP range checking
	using radix tree for efficient CIDR matching
*/
import (
	"net"
	"strings"
	"sync"
)

// reservedIPNode represents a node in the radix tree for reserved IP ranges
type reservedIPNode struct {
	children  [2]*reservedIPNode
	zoneName  string
	prefixLen int // Length of the prefix at this node
}

// reservedIPRadixTree is a radix tree for efficient CIDR matching
type reservedIPRadixTree struct {
	root *reservedIPNode
}

// reservedCIDRsToZoneName maps reserved CIDR ranges to their zone names
var reservedCIDRsToZoneName = map[string]string{
	"10.0.0.0/8":      "Private",
	"172.16.0.0/12":   "Private",
	"192.168.0.0/16":  "Private",
	"100.64.0.0/10":   "CarrierNAT",
	"127.0.0.0/8":     "Loopback",
	"169.254.0.0/16":  "LinkLocal",
	"224.0.0.0/4":     "Multicast",
	"240.0.0.0/4":     "Reserved",
	"192.0.2.0/24":    "Documentation",
	"198.51.100.0/24": "Documentation",
	"203.0.113.0/24":  "Documentation",
	"198.18.0.0/15":   "Interconnect",
	"::/128":          "Unspecified",
	"::1/128":         "Loopback",
	"::ffff:0:0/96":   "IPv4Mapped",
	"fe80::/10":       "LinkLocal",
	"fc00::/7":        "UniqueLocal",
	"ff00::/8":        "Multicast",
	"2001:db8::/32":   "Documentation",
	"2002::/16":       "6to4",
	"2001::/32":       "Teredo",
	"2001:20::/28":    "ORCHID",
	"2001:2::/48":     "Benchmarking",
	"100::/64":        "Discard",
	"64:ff9b::/96":    "Translation",
	"64:ff9b:1::/48":  "Translation",
}

var (
	reservedIPv4Tree     *reservedIPRadixTree
	reservedIPv6Tree     *reservedIPRadixTree
	reservedTreeInitOnce sync.Once
)

// initReservedIPTrees initializes the radix trees for reserved IP ranges
func initReservedIPTrees() {
	reservedTreeInitOnce.Do(func() {
		reservedIPv4Tree = &reservedIPRadixTree{root: &reservedIPNode{}}
		reservedIPv6Tree = &reservedIPRadixTree{root: &reservedIPNode{}}

		// Insert all reserved CIDR ranges into appropriate trees
		for cidr, zoneName := range reservedCIDRsToZoneName {
			if strings.Contains(cidr, ":") {
				reservedIPv6Tree.insert(cidr, zoneName)
			} else {
				reservedIPv4Tree.insert(cidr, zoneName)
			}
		}
	})
}

// insert adds a CIDR range to the radix tree
func (t *reservedIPRadixTree) insert(cidr, zoneName string) {
	_, ipnet, err := net.ParseCIDR(cidr)
	if err != nil {
		return
	}

	prefixLen, _ := ipnet.Mask.Size()
	// Use the network IP from ipnet, not the parsed IP
	ipBytes := ipnet.IP.To4()
	if ipBytes == nil {
		ipBytes = ipnet.IP.To16()
	}

	if ipBytes == nil {
		return
	}

	current := t.root
	bitPos := 0

	// Traverse the tree based on the prefix
	for bitPos < prefixLen {
		byteIdx := bitPos / 8
		if byteIdx >= len(ipBytes) {
			break
		}
		bitIdx := 7 - (bitPos % 8)
		bit := int((ipBytes[byteIdx] >> bitIdx) & 1)

		if current.children[bit] == nil {
			current.children[bit] = &reservedIPNode{}
		}
		current = current.children[bit]
		bitPos++
	}

	// Mark this node with the zone name and prefix length
	current.zoneName = zoneName
	current.prefixLen = prefixLen
}

// search finds the most specific (longest prefix) match for an IP
func (t *reservedIPRadixTree) search(ip net.IP) string {
	if t == nil || t.root == nil {
		return ""
	}

	ipBytes := ip.To4()
	if ipBytes == nil {
		ipBytes = ip.To16()
	}

	current := t.root
	lastMatch := ""
	bitPos := 0
	maxBits := len(ipBytes) * 8

	// Traverse the tree following the IP's bit pattern
	for bitPos < maxBits && current != nil {
		// If this node has a zone name, it's a potential match
		if current.zoneName != "" {
			lastMatch = current.zoneName
		}

		byteIdx := bitPos / 8
		bitIdx := 7 - (bitPos % 8)
		bit := int((ipBytes[byteIdx] >> bitIdx) & 1)

		current = current.children[bit]
		bitPos++
	}

	// Check the final node
	if current != nil && current.zoneName != "" {
		lastMatch = current.zoneName
	}

	return lastMatch
}

// Check if a ip is private IP range
func isPrivateIP(ipStr string) bool {
	if ipStr == "127.0.0.1" || ipStr == "::1" {
		// local loopback
		return true
	}
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	if ip.IsPrivate() {
		return true
	}
	// Check for IPv6 link-local addresses (fe80::/10)
	if ip.To16() != nil && ip.To4() == nil {
		// IPv6 only
		if ip[0] == 0xfe && (ip[1]&0xc0) == 0x80 {
			return true
		}
	}
	return false
}

// getReservedIPZone checks if an IP is in a reserved range and returns the zone name
// Uses radix tree for O(log n) lookup complexity
func getReservedIPZone(ipStr string) string {
	// Initialize trees on first call
	initReservedIPTrees()
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return ""
	}

	// Determine if this is an IPv4 or IPv6 address string
	// We check the original string because net.ParseIP can normalize addresses
	isIPv4Str := ip.To4() != nil && !strings.Contains(ipStr, ":")

	// Search in the appropriate radix tree
	if isIPv4Str {
		return reservedIPv4Tree.search(ip)
	} else {
		return reservedIPv6Tree.search(ip)
	}
}
