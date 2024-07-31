package geodb

import (
	"net"
)

type trie_Node struct {
	childrens [2]*trie_Node
	cc        string
}

// Initializing the root of the trie
type trie struct {
	root *trie_Node
}

func ipToBytes(ip string) []byte {
	// Parse the IP address string into a net.IP object
	parsedIP := net.ParseIP(ip)

	// Convert the IP address to a 4-byte slice
	ipBytes := parsedIP.To4()
	if ipBytes == nil {
		//This is an IPv6 address
		ipBytes = parsedIP.To16()
	}

	return ipBytes
}

// inititlaizing a new trie
func newTrie() *trie {
	t := new(trie)
	t.root = new(trie_Node)
	return t
}

// Passing words to trie
func (t *trie) insert(ipAddr string, cc string) {
	ipBytes := ipToBytes(ipAddr)
	current := t.root
	for _, b := range ipBytes {
		//For each byte in the ip address (4 / 16 bytes)
		//each byte is 8 bit
		for j := 7; j >= 0; j-- {
			bit := int(b >> j & 1)
			if current.childrens[bit] == nil {
				current.childrens[bit] = &trie_Node{
					childrens: [2]*trie_Node{},
					cc:        cc,
				}
			}
			current = current.childrens[bit]
		}
	}
}

// isReservedIP check if the given ip address is NOT a public ip address
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
	//Check if the IP is in the reserved private range
	if parsedIP.IsPrivate() {
		return true
	}
	return false
}

// Initializing the search for word in node
func (t *trie) search(ipAddr string) string {
	if isReservedIP(ipAddr) {
		return ""
	}

	ipBytes := ipToBytes(ipAddr)
	current := t.root
	for _, b := range ipBytes {
		//For each byte in the ip address
		//each byte is 8 bit
		for j := 7; j >= 0; j-- {
			bit := int(b >> j & 1)
			if current.childrens[bit] == nil {
				return current.cc
			}
			current = current.childrens[bit]
		}
	}

	if len(current.childrens) == 0 {
		return current.cc
	}

	//Not found
	return ""
}
