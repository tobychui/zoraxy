package geodb

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

type trie_Node struct {
	childrens [2]*trie_Node
	ends      bool
	cc        string
}

// Initializing the root of the trie
type trie struct {
	root *trie_Node
}

func ipToBitString(ip string) string {
	// Parse the IP address string into a net.IP object
	parsedIP := net.ParseIP(ip)

	// Convert the IP address to a 4-byte slice
	ipBytes := parsedIP.To4()
	if ipBytes == nil {
		//This is an IPv6 address
		ipBytes = parsedIP.To16()
	}

	// Convert each byte in the IP address to its 8-bit binary representation
	var result []string
	for _, b := range ipBytes {
		result = append(result, fmt.Sprintf("%08b", b))
	}

	// Join the binary representation of each byte with dots to form the final bit string
	return strings.Join(result, "")
}

func bitStringToIp(bitString string) string {
	// Check if the bit string represents an IPv4 or IPv6 address
	isIPv4 := len(bitString) == 32

	// Split the bit string into 8-bit segments
	segments := make([]string, 0)
	if isIPv4 {
		for i := 0; i < 4; i++ {
			segments = append(segments, bitString[i*8:(i+1)*8])
		}
	} else {
		for i := 0; i < 16; i++ {
			segments = append(segments, bitString[i*8:(i+1)*8])
		}
	}

	// Convert each segment to its decimal equivalent
	decimalSegments := make([]int, len(segments))
	for i, s := range segments {
		val, _ := strconv.ParseInt(s, 2, 64)
		decimalSegments[i] = int(val)
	}

	// Construct the IP address string based on the type (IPv4 or IPv6)
	if isIPv4 {
		return fmt.Sprintf("%d.%d.%d.%d", decimalSegments[0], decimalSegments[1], decimalSegments[2], decimalSegments[3])
	} else {
		ip := make(net.IP, net.IPv6len)
		for i := 0; i < net.IPv6len; i++ {
			ip[i] = byte(decimalSegments[i])
		}
		return ip.String()
	}
}

// inititlaizing a new trie
func newTrie() *trie {
	t := new(trie)
	t.root = new(trie_Node)
	return t
}

// Passing words to trie
func (t *trie) insert(ipAddr string, cc string) {
	word := ipToBitString(ipAddr)
	current := t.root
	for _, wr := range word {
		index := wr - '0'
		if current.childrens[index] == nil {
			current.childrens[index] = &trie_Node{
				childrens: [2]*trie_Node{},
				ends:      false,
				cc:        cc,
			}
		}
		current = current.childrens[index]
	}
	current.ends = true
}

func isReservedIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	if parsedIP == nil {
		return false
	}
	// Check if the IP address is a loopback address
	if parsedIP.IsLoopback() {
		return true
	}
	// Check if the IP address is in the link-local address range
	if parsedIP.IsLinkLocalUnicast() || parsedIP.IsLinkLocalMulticast() {
		return true
	}

	if parsedIP.IsPrivate() {
		return true
	}

	// If the IP address is not a reserved address, return false
	return false
}

// Initializing the search for word in node
func (t *trie) search(ipAddr string) string {
	if isReservedIP(ipAddr) {
		return ""
	}
	word := ipToBitString(ipAddr)
	current := t.root
	for _, wr := range word {
		index := wr - '0'
		if current.childrens[index] == nil {
			return current.cc
		}
		current = current.childrens[index]
	}
	if current.ends {
		return current.cc
	}

	//Not found
	return ""
}
