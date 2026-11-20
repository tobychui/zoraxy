package geodb

import (
	"encoding/binary"
	"net"
	"sort"
)

// ipRange represents a single IP range with its country code
// Memory: 8 bytes (startIP) + 2 bytes (ccIdx) = 10 bytes per IPv4 range
// vs old trie: ~64 nodes * 40+ bytes per node = 2500+ bytes per IP
type ipRange struct {
	startIP uint64 // For IPv4: upper 32 bits unused. For IPv6: we use separate structure
	ccIdx   uint16 // Index into country code table (max 65535 countries, we have ~250)
}

// ipRangeV6 represents an IPv6 range (needs 128-bit support)
type ipRangeV6 struct {
	startIPHigh uint64 // Upper 64 bits of IPv6
	startIPLow  uint64 // Lower 64 bits of IPv6
	ccIdx       uint16
}

// trie is now a memory-efficient sorted range structure
// Uses binary search for O(log n) lookup, similar to trie's O(32) for IPv4
type trie struct {
	ranges    []ipRange   // Sorted by startIP
	rangesV6  []ipRangeV6 // Sorted by startIP for IPv6
	ccTable   []string    // Country code lookup table
	ccToIndex map[string]uint16
	isIPv6    bool
}

// newTrie creates a new memory-efficient IP range structure
func newTrie() *trie {
	return &trie{
		ranges:    make([]ipRange, 0),
		rangesV6:  make([]ipRangeV6, 0),
		ccTable:   make([]string, 0),
		ccToIndex: make(map[string]uint16),
		isIPv6:    false,
	}
}

// getOrCreateCCIndex returns the index for a country code, creating if needed
func (t *trie) getOrCreateCCIndex(cc string) uint16 {
	if idx, exists := t.ccToIndex[cc]; exists {
		return idx
	}
	idx := uint16(len(t.ccTable))
	t.ccTable = append(t.ccTable, cc)
	t.ccToIndex[cc] = idx
	return idx
}

// ipv4ToUint64 converts an IPv4 address to uint64
func ipv4ToUint64(ip net.IP) uint64 {
	ipv4 := ip.To4()
	if ipv4 == nil {
		return 0
	}
	return uint64(binary.BigEndian.Uint32(ipv4))
}

// ipv6ToUint64Pair converts an IPv6 address to two uint64 values
func ipv6ToUint64Pair(ip net.IP) (uint64, uint64) {
	ipv6 := ip.To16()
	if ipv6 == nil {
		return 0, 0
	}
	high := binary.BigEndian.Uint64(ipv6[0:8])
	low := binary.BigEndian.Uint64(ipv6[8:16])
	return high, low
}

// nextIP returns the address right after the given one. ok is false if the
// given address is the last address of its address family
func nextIP(ip net.IP) (next net.IP, ok bool) {
	if ipv4 := ip.To4(); ipv4 != nil {
		//Use the 4 bytes form so 255.255.255.255 does not overflow into
		//the IPv6 address space
		ip = ipv4
	}
	next = make(net.IP, len(ip))
	copy(next, ip)
	for i := len(next) - 1; i >= 0; i-- {
		next[i]++
		if next[i] != 0 {
			return next, true
		}
	}
	return nil, false
}

// insertRange adds an IP range with its country code. Only the start of the
// range is stored with the country code. The address right after the end of
// the range is stored with an empty (unknown) country code, so addresses in a
// gap between two ranges are not resolved to the country of the previous range
func (t *trie) insertRange(startIP string, endIP string, cc string) {
	parsedStartIP := net.ParseIP(startIP)
	parsedEndIP := net.ParseIP(endIP)
	if parsedStartIP == nil || parsedEndIP == nil {
		return
	}

	t.insertIP(parsedStartIP, cc)
	if afterEndIP, ok := nextIP(parsedEndIP); ok {
		t.insertIP(afterEndIP, "")
	}
}

// insert adds an IP address with its country code
func (t *trie) insert(ipAddr string, cc string) {
	parsedIP := net.ParseIP(ipAddr)
	if parsedIP == nil {
		return
	}
	t.insertIP(parsedIP, cc)
}

// insertIP adds a parsed IP address with its country code
func (t *trie) insertIP(parsedIP net.IP, cc string) {
	ccIdx := t.getOrCreateCCIndex(cc)

	if parsedIP.To4() != nil {
		// IPv4
		t.isIPv6 = false
		ipVal := ipv4ToUint64(parsedIP)
		t.ranges = append(t.ranges, ipRange{
			startIP: ipVal,
			ccIdx:   ccIdx,
		})
	} else {
		// IPv6
		t.isIPv6 = true
		high, low := ipv6ToUint64Pair(parsedIP)
		t.rangesV6 = append(t.rangesV6, ipRangeV6{
			startIPHigh: high,
			startIPLow:  low,
			ccIdx:       ccIdx,
		})
	}
}

// build sorts the ranges after all inserts are done
// Must be called after all inserts and before any searches
func (t *trie) build() {
	if t.isIPv6 {
		sort.Slice(t.rangesV6, func(i, j int) bool {
			if t.rangesV6[i].startIPHigh != t.rangesV6[j].startIPHigh {
				return t.rangesV6[i].startIPHigh < t.rangesV6[j].startIPHigh
			}
			return t.rangesV6[i].startIPLow < t.rangesV6[j].startIPLow
		})
	} else {
		sort.Slice(t.ranges, func(i, j int) bool {
			return t.ranges[i].startIP < t.ranges[j].startIP
		})
	}

	t.removeDuplicatedEntries()
}

// removeDuplicatedEntries merge the entries sharing the same address after
// sorting. This happens when a range starts right after the end of the
// previous one, where the start of the next range must win over the unknown
// marker added after the end of the previous range (see insertRange)
func (t *trie) removeDuplicatedEntries() {
	gapIdx, hasGapMarker := t.ccToIndex[""]
	if t.isIPv6 {
		compacted := make([]ipRangeV6, 0, len(t.rangesV6))
		for _, entry := range t.rangesV6 {
			last := len(compacted) - 1
			if last >= 0 && compacted[last].startIPHigh == entry.startIPHigh && compacted[last].startIPLow == entry.startIPLow {
				if hasGapMarker && compacted[last].ccIdx == gapIdx {
					compacted[last] = entry
				}
				continue
			}
			compacted = append(compacted, entry)
		}
		t.rangesV6 = append([]ipRangeV6(nil), compacted...)
	} else {
		compacted := make([]ipRange, 0, len(t.ranges))
		for _, entry := range t.ranges {
			last := len(compacted) - 1
			if last >= 0 && compacted[last].startIP == entry.startIP {
				if hasGapMarker && compacted[last].ccIdx == gapIdx {
					compacted[last] = entry
				}
				continue
			}
			compacted = append(compacted, entry)
		}
		t.ranges = append([]ipRange(nil), compacted...)
	}
}

// search finds the country code for an IP address using binary search
// Time complexity: O(log n) where n is number of ranges
func (t *trie) search(ipAddr string) string {
	// Check reserved IP zones first
	reservedZone := getReservedIPZone(ipAddr)
	if reservedZone != "" {
		return reservedZone
	}

	parsedIP := net.ParseIP(ipAddr)
	if parsedIP == nil {
		return ""
	}

	if parsedIP.To4() != nil {
		return t.searchIPv4(parsedIP)
	}
	return t.searchIPv6(parsedIP)
}

// searchIPv4 performs binary search for IPv4
func (t *trie) searchIPv4(ip net.IP) string {
	if len(t.ranges) == 0 {
		return ""
	}

	ipVal := ipv4ToUint64(ip)

	// Binary search to find the largest startIP <= ipVal
	idx := sort.Search(len(t.ranges), func(i int) bool {
		return t.ranges[i].startIP > ipVal
	})

	// idx is now the first element > ipVal, so we want idx-1
	if idx == 0 {
		return ""
	}

	idx--
	return t.ccTable[t.ranges[idx].ccIdx]
}

// searchIPv6 performs binary search for IPv6
func (t *trie) searchIPv6(ip net.IP) string {
	if len(t.rangesV6) == 0 {
		return ""
	}

	high, low := ipv6ToUint64Pair(ip)

	// Binary search for IPv6
	idx := sort.Search(len(t.rangesV6), func(i int) bool {
		if t.rangesV6[i].startIPHigh != high {
			return t.rangesV6[i].startIPHigh > high
		}
		return t.rangesV6[i].startIPLow > low
	})

	if idx == 0 {
		return ""
	}

	idx--
	return t.ccTable[t.rangesV6[idx].ccIdx]
}
