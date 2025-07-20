package tlscert

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"time"
)

// GenerateSelfSignedCertificate generates a self-signed ECDSA certificate and saves it to the specified files.
func (m *Manager) GenerateSelfSignedCertificate(cn string, sans []string, certFile string, keyFile string) error {
	// Generate private key (ECDSA P-256)
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		m.Logger.PrintAndLog("tls-router", "Failed to generate private key", err)
		return err
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName:         cn,                   // Common Name for the certificate
			Organization:       []string{"aroz.org"}, // Organization name
			OrganizationalUnit: []string{"Zoraxy"},   // Organizational Unit
			Country:            []string{"US"},       // Country code
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour), // valid for 1 year
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              sans, // Subject Alternative Names
	}

	// Create self-signed certificate
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	if err != nil {
		m.Logger.PrintAndLog("tls-router", "Failed to create certificate", err)
		return err
	}

	// Remove old certificate file if it exists
	certPath := filepath.Join(m.CertStore, certFile)
	if _, err := os.Stat(certPath); err == nil {
		os.Remove(certPath)
	}

	// Remove old key file if it exists
	keyPath := filepath.Join(m.CertStore, keyFile)
	if _, err := os.Stat(keyPath); err == nil {
		os.Remove(keyPath)
	}

	// Write certificate to file
	certOut, err := os.Create(filepath.Join(m.CertStore, certFile))
	if err != nil {
		m.Logger.PrintAndLog("tls-router", "Failed to open cert file for writing: "+certFile, err)
		return err
	}
	defer certOut.Close()
	err = pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	if err != nil {
		m.Logger.PrintAndLog("tls-router", "Failed to write certificate to file: "+certFile, err)
		return err
	}

	// Encode private key to PEM
	privBytes, err := x509.MarshalECPrivateKey(privKey)
	if err != nil {
		m.Logger.PrintAndLog("tls-router", "Unable to marshal ECDSA private key", err)
		return err
	}
	keyOut, err := os.Create(filepath.Join(m.CertStore, keyFile))
	if err != nil {
		m.Logger.PrintAndLog("tls-router", "Failed to open key file for writing: "+keyFile, err)
		return err
	}
	defer keyOut.Close()
	err = pem.Encode(keyOut, &pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes})
	if err != nil {
		m.Logger.PrintAndLog("tls-router", "Failed to write private key to file: "+keyFile, err)
		return err
	}
	m.Logger.PrintAndLog("tls-router", "Certificate and key generated: "+certFile+", "+keyFile, nil)
	return nil
}
