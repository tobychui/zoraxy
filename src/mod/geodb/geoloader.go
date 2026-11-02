package geodb

import (
	"bytes"
	"encoding/csv"
	"io"
	"strings"

	"imuslab.com/zoraxy/mod/netutils"
)

func (s *Store) search(ip string) string {
	if strings.Contains(ip, ",") {
		//This is a CF proxied request. We only need the front part
		//Example 219.71.102.145, 172.71.139.178
		ip = strings.Split(ip, ",")[0]
		ip = strings.TrimSpace(ip)
	}

	//Get the current dataset snapshot, it might be swapped out by a
	//database update while this lookup is running
	dataset := s.dataset.Load()
	if dataset == nil {
		return ""
	}

	//Search in geotrie tree
	cc := ""
	if netutils.IsIPv6(ip) {
		if dataset.geotrieIpv6 == nil {
			cc = s.slowSearchIpv6(dataset, ip)
		} else {
			cc = dataset.geotrieIpv6.search(ip)
		}
	} else {
		if dataset.geotrie == nil {
			cc = s.slowSearchIpv4(dataset, ip)
		} else {
			cc = dataset.geotrie.search(ip)
		}
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

	// Sort the ranges for binary search after all inserts
	tt.build()

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
