package geodb

import (
	"math"
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
		//For each byte in the ip address
		//each byte is 8 bit
		for j := 0; j < 8; j++ {
			bitwise := (b&uint8(math.Pow(float64(2), float64(j))) > 0)
			bit := 0b0000
			if bitwise {
				bit = 0b0001
			}
			if current.childrens[bit] == nil {
				current.childrens[bit] = &trie_Node{
					childrens: [2]*trie_Node{},
					cc:        cc,
				}
			}
			current = current.childrens[bit]
		}
	}

	/*
		for i := 63; i >= 0; i-- {
			bit := (ipInt64 >> uint(i)) & 1
			if current.childrens[bit] == nil {
				current.childrens[bit] = &trie_Node{
					childrens: [2]*trie_Node{},
					cc:        cc,
				}
			}
			current = current.childrens[bit]
		}
	*/
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

	ipBytes := ipToBytes(ipAddr)
	current := t.root
	for _, b := range ipBytes {
		//For each byte in the ip address
		//each byte is 8 bit
		for j := 0; j < 8; j++ {
			bitwise := (b&uint8(math.Pow(float64(2), float64(j))) > 0)
			bit := 0b0000
			if bitwise {
				bit = 0b0001
			}
			if current.childrens[bit] == nil {
				return current.cc
			}
			current = current.childrens[bit]
		}
	}
	/*
		for i := 63; i >= 0; i-- {
			bit := (ipInt64 >> uint(i)) & 1
			if current.childrens[bit] == nil {
				return current.cc
			}
			current = current.childrens[bit]
		}
	*/
	if len(current.childrens) == 0 {
		return current.cc
	}

	//Not found
	return ""
}
