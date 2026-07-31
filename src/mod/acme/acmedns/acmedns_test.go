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
		"azure",
	}

	for _, provider := range providers {
		strcture, err := acmedns.GetProviderConfigStructure(provider)
		if err != nil {
			panic(err)
		}

		fmt.Println(strcture)
	}

}
