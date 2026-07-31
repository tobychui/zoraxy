package acmedns_test

import (
	"fmt"
	"testing"

	"imuslab.com/zoraxy/mod/acme/acmedns"
)

// Test if the structure of ACME DNS config can be reflected from lego source code definations
func TestACMEDNSConfigStructureReflector(t *testing.T) {
	providers := []string{
		"gandi",
		"cloudflare",
		"azuredns",
	}

	for _, provider := range providers {
		structure, err := acmedns.GetProviderConfigStructure(provider)
		if err != nil {
			t.Fatalf("%s: %v", provider, err)
		}

		fmt.Println(structure)
	}
}
