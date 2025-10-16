package acme

import (
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"
)

// Get the issuer name from pem file
func ExtractIssuerNameFromPEM(pemFilePath string) (string, error) {
	// Read the PEM file
	pemData, err := os.ReadFile(pemFilePath)
	if err != nil {
		return "", err
	}

	return ExtractIssuerName(pemData)
}

// Get the DNSName in the cert
func ExtractDomains(certBytes []byte) ([]string, error) {
	domains := []string{}
	block, _ := pem.Decode(certBytes)
	if block != nil {
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return []string{}, err
		}
		for _, dnsName := range cert.DNSNames {
			if !contains(domains, dnsName) {
				domains = append(domains, dnsName)
			}
		}

		return domains, nil
	}
	return []string{}, errors.New("decode cert bytes failed")
}

func ExtractIssuerName(certBytes []byte) (string, error) {
	// Parse the PEM block
	block, _ := pem.Decode(certBytes)
	if block == nil || block.Type != "CERTIFICATE" {
		return "", fmt.Errorf("failed to decode PEM block containing certificate")
	}

	// Parse the certificate
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse certificate: %v", err)
	}

	// Check if exist incase some acme server didn't have org section
	if len(cert.Issuer.Organization) == 0 {
		return "", fmt.Errorf("cert didn't have org section exist")
	}

	// Extract the issuer name
	issuer := cert.Issuer.Organization[0]

	return issuer, nil
}

// ExtractDomainsFromPEM reads a PEM certificate file and returns all SANs
func ExtractDomainsFromPEM(pemFilePath string) ([]string, error) {

	certBytes, err := os.ReadFile(pemFilePath)
	if err != nil {
		return nil, err
	}
	domains, err := ExtractDomains(certBytes)
	if err != nil {
		return nil, err
	}
	return domains, nil
}

// Check if a cert is expired by public key
func CertIsExpired(certBytes []byte) bool {
	block, _ := pem.Decode(certBytes)
	if block != nil {
		cert, err := x509.ParseCertificate(block.Bytes)
		if err == nil {
			elapsed := time.Since(cert.NotAfter)
			if elapsed > 0 {
				// if it is expired then add it in
				// make sure it's uniqueless
				return true
			}
		}
	}
	return false
}

// CertExpireSoon check if the given cert bytes will expires within the given number of days from now
func CertExpireSoon(certBytes []byte, numberOfDays int) bool {
	block, _ := pem.Decode(certBytes)
	if block != nil {
		cert, err := x509.ParseCertificate(block.Bytes)
		if err == nil {
			expirationDate := cert.NotAfter
			threshold := time.Duration(numberOfDays) * 24 * time.Hour

			timeRemaining := time.Until(expirationDate)
			if timeRemaining <= threshold {
				return true
			}
		}
	}
	return false
}
