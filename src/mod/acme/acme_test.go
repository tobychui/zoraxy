package acme_test

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"testing"
	"time"

	"imuslab.com/zoraxy/mod/acme"
)

// Test if the issuer extraction is working
func TestExtractIssuerNameFromPEM(t *testing.T) {
	// Generate a test CA certificate with "Let's Encrypt" as the issuer organization
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Let's Encrypt"},
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	caDERBytes, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}
	caCert, err := x509.ParseCertificate(caDERBytes)
	if err != nil {
		t.Fatal(err)
	}

	// Generate a leaf certificate signed by the CA
	leafKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	leafTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "test.example.com"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	leafDERBytes, err := x509.CreateCertificate(rand.Reader, leafTemplate, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		t.Fatal(err)
	}

	// Write the leaf certificate to a temporary PEM file
	tmpFile, err := os.CreateTemp("", "test-cert-*.pem")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpFile.Name())
	if err := pem.Encode(tmpFile, &pem.Block{Type: "CERTIFICATE", Bytes: leafDERBytes}); err != nil {
		t.Fatal(err)
	}
	tmpFile.Close()

	expectedIssuer := "Let's Encrypt"
	issuerName, err := acme.ExtractIssuerNameFromPEM(tmpFile.Name())
	if err != nil {
		t.Errorf("Error extracting issuer name: %v", err)
	}

	if issuerName != expectedIssuer {
		t.Errorf("Unexpected issuer name. Expected: %s, Got: %s", expectedIssuer, issuerName)
	}
}
