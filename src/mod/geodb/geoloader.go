package geodb

import (
	"bytes"
	"encoding/csv"
	"io"
	"net"
	"strings"
)

func (s *Store) search(ip string) string {
	if strings.Contains(ip, ",") {
		//This is a CF proxied request. We only need the front part
		//Example 219.71.102.145, 172.71.139.178
		ip = strings.Split(ip, ",")[0]
		ip = strings.TrimSpace(ip)
	}
	//See if there are cached country code for this ip
	/*
		ccc, ok := s.geoipCache.Load(ip)
		if ok {
			return ccc.(string)
		}
	*/

	//Search in geotrie tree
	cc := ""
	if IsIPv6(ip) {
		cc = s.geotrieIpv6.search(ip)
	} else {
		cc = s.geotrie.search(ip)
	}

	/*
		if cc != "" {
			s.geoipCache.Store(ip, cc)
		}
	*/
	return cc
}

// Construct the trie data structure for quick lookup
func constrctTrieTree(data [][]string) *trie {
	tt := newTrie()
	for _, entry := range data {
		startIp := entry[0]
		endIp := entry[1]
		cc := entry[2]
		tt.insert(startIp, cc)
		tt.insert(endIp, cc)
	}

	return tt
}

// Parse the embedded csv as ipstart, ipend and country code entries
func parseCSV(content []byte) ([][]string, error) {
	var records [][]string
	r := csv.NewReader(bytes.NewReader(content))
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

// Check if a ip string is within the range of two others
func isIPInRange(ip, start, end string) bool {
	ipAddr := net.ParseIP(ip)
	if ipAddr == nil {
		return false
	}

	startAddr := net.ParseIP(start)
	if startAddr == nil {
		return false
	}

	endAddr := net.ParseIP(end)
	if endAddr == nil {
		return false
	}

	if ipAddr.To4() == nil || startAddr.To4() == nil || endAddr.To4() == nil {
		return false
	}

	return bytes.Compare(ipAddr.To4(), startAddr.To4()) >= 0 && bytes.Compare(ipAddr.To4(), endAddr.To4()) <= 0
}
