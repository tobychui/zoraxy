package geodb

import (
	"net"
	"testing"
)

// TestTrieRangeGaps make sure addresses outside of any range are not resolved
// to the country of the previous range
func TestTrieRangeGaps(t *testing.T) {
	ipv4Data := [][]string{
		{"1.0.0.0", "1.0.0.255", "AU"},
		{"1.0.1.0", "1.0.3.255", "CN"},  //Adjacent to the previous range
		{"1.0.8.0", "1.0.15.255", "CN"}, //Gap 1.0.4.0 - 1.0.7.255
		{"255.255.255.0", "255.255.255.255", "ZZ"},
	}
	ipv6Data := [][]string{
		{"2001:cf8::", "2001:cf8:ffff:ffff:ffff:ffff:ffff:ffff", "JP"},
		{"2001:d08::", "2001:d08:ffff:ffff:ffff:ffff:ffff:ffff", "MY"}, //Gap 2001:cf9:: - 2001:d07:ffff:...
		{"2001:d09::", "2001:d09:ffff:ffff:ffff:ffff:ffff:ffff", "SG"}, //Adjacent to the previous range
		{"ffff::", "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "ZZ"},
	}

	ipv4Trie := constrctTrieTree(ipv4Data)
	ipv6Trie := constrctTrieTree(ipv6Data)

	testCases := []struct {
		tree     *trie
		ip       string
		expected string
	}{
		{ipv4Trie, "0.255.255.255", ""},
		{ipv4Trie, "1.0.0.0", "AU"},
		{ipv4Trie, "1.0.0.255", "AU"},
		{ipv4Trie, "1.0.1.0", "CN"},
		{ipv4Trie, "1.0.3.255", "CN"},
		{ipv4Trie, "1.0.4.0", ""},
		{ipv4Trie, "1.0.7.255", ""},
		{ipv4Trie, "1.0.8.0", "CN"},
		{ipv4Trie, "1.0.15.255", "CN"},
		{ipv4Trie, "1.0.16.0", ""},
		{ipv4Trie, "255.255.255.255", "ZZ"},
		{ipv6Trie, "2001:cf8::1", "JP"},
		{ipv6Trie, "2001:cf8:ffff:ffff:ffff:ffff:ffff:ffff", "JP"},
		{ipv6Trie, "2001:cf9::", ""},
		{ipv6Trie, "2001:d00::1", ""},
		{ipv6Trie, "2001:d08::1", "MY"},
		{ipv6Trie, "2001:d08:ffff:ffff:ffff:ffff:ffff:ffff", "MY"},
		{ipv6Trie, "2001:d09::", "SG"},
		{ipv6Trie, "2001:d0a::", ""},
		{ipv6Trie, "ffff:ffff:ffff:ffff:ffff:ffff:ffff:ffff", "ZZ"},
	}

	for _, testCase := range testCases {
		//Search without the reserved zone check, so ranges like 240.0.0.0/4
		//can be used to test the end of the address space
		parsedIP := net.ParseIP(testCase.ip)
		result := ""
		if parsedIP.To4() != nil {
			result = testCase.tree.searchIPv4(parsedIP)
		} else {
			result = testCase.tree.searchIPv6(parsedIP)
		}
		if result != testCase.expected {
			t.Errorf("expected %q, but got %q for IP %s", testCase.expected, result, testCase.ip)
		}
	}

	//Adjacent ranges should share a single entry instead of keeping the
	//unknown marker after the end of the previous range
	if len(ipv4Trie.ranges) != 6 {
		t.Errorf("expected 6 IPv4 entries after merging, got %d", len(ipv4Trie.ranges))
	}
	if len(ipv6Trie.rangesV6) != 6 {
		t.Errorf("expected 6 IPv6 entries after merging, got %d", len(ipv6Trie.rangesV6))
	}
}
