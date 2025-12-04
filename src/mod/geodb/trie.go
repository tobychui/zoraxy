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

// Initializing the search for word in node
func (t *trie) search(ipAddr string) string {
	// Check reserved IP zones first
	reservedZone := getReservedIPZone(ipAddr)
	if reservedZone != "" {
		return reservedZone
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
