package geodb_test

import (
	"testing"

	"imuslab.com/zoraxy/mod/geodb"
)

/*
func TestTrieConstruct(t *testing.T) {
	tt := geodb.NewTrie()
	data := [][]string{
		{"1.0.16.0", "1.0.31.255", "JP"},
		{"1.0.32.0", "1.0.63.255", "CN"},
		{"1.0.64.0", "1.0.127.255", "JP"},
		{"1.0.128.0", "1.0.255.255", "TH"},
		{"1.1.0.0", "1.1.0.255", "CN"},
		{"1.1.1.0", "1.1.1.255", "AU"},
		{"1.1.2.0", "1.1.63.255", "CN"},
		{"1.1.64.0", "1.1.127.255", "JP"},
		{"1.1.128.0", "1.1.255.255", "TH"},
		{"1.2.0.0", "1.2.2.255", "CN"},
		{"1.2.3.0", "1.2.3.255", "AU"},
	}

	for _, entry := range data {
		startIp := entry[0]
		endIp := entry[1]
		cc := entry[2]
		tt.Insert(startIp, cc)
		tt.Insert(endIp, cc)
	}

	t.Log(tt.Search("1.0.16.20"), "== JP")  //JP
	t.Log(tt.Search("1.2.0.122"), "== CN")  //CN
	t.Log(tt.Search("1.2.1.0"), "== CN")    //CN
	t.Log(tt.Search("1.0.65.243"), "== JP") //JP
	t.Log(tt.Search("1.0.62.243"), "== CN") //CN
}
*/

func TestResolveCountryCodeFromIP(t *testing.T) {
	// Create a new store
	store, err := geodb.NewGeoDb(nil, &geodb.StoreOptions{
		false,
		true,
		0,
	})
	if err != nil {
		t.Errorf("error creating store: %v", err)
		return
	}

	// Test an IP address that should return a valid country code
	knownIpCountryMap := [][]string{
		{"3.224.220.101", "US"},
		{"176.113.115.113", "RU"},
		{"65.21.233.213", "FI"},
		{"94.23.207.193", "FR"},
		{"77.131.21.232", "FR"},
	}

	for _, testcase := range knownIpCountryMap {
		ip := testcase[0]
		expected := testcase[1]
		info, err := store.ResolveCountryCodeFromIP(ip)
		if err != nil {
			t.Errorf("error resolving country code for IP %s: %v", ip, err)
			return
		}
		if info.CountryIsoCode != expected {
			t.Errorf("expected country code %s, but got %s for IP %s", expected, info.CountryIsoCode, ip)
		}
	}

	// Test an IP address that should return an empty country code
	ip := "127.0.0.1"
	expected := ""
	info, err := store.ResolveCountryCodeFromIP(ip)
	if err != nil {
		t.Errorf("error resolving country code for IP %s: %v", ip, err)
		return
	}
	if info.CountryIsoCode != expected {
		t.Errorf("expected country code %s, but got %s for IP %s", expected, info.CountryIsoCode, ip)
	}
}
