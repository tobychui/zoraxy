package acme_test

import (
	"fmt"
	"testing"

	"imuslab.com/zoraxy/mod/acme"
)

// Test if the issuer extraction is working
func TestExtractIssuerNameFromPEM(t *testing.T) {
	pemFilePath := "test/stackoverflow.pem"
	expectedIssuer := "Let's Encrypt"

	issuerName, err := acme.ExtractIssuerNameFromPEM(pemFilePath)
	fmt.Println(issuerName)
	if err != nil {
		t.Errorf("Error extracting issuer name: %v", err)
	}

	if issuerName != expectedIssuer {
		t.Errorf("Unexpected issuer name. Expected: %s, Got: %s", expectedIssuer, issuerName)
	}
}
