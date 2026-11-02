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

	// Resolve and validate both paths stay within the certificate store
	// before touching the filesystem (certFile/keyFile are expected to
	// already be sanitized by the caller, e.g. via domainToFilename).
	certPath, err := safeJoin(m.CertStore, certFile)
	if err != nil {
		m.Logger.PrintAndLog("tls-router", "Rejected unsafe certificate filename: "+certFile, err)
		return err
	}
	keyPath, err := safeJoin(m.CertStore, keyFile)
	if err != nil {
		m.Logger.PrintAndLog("tls-router", "Rejected unsafe key filename: "+keyFile, err)
		return err
	}

	// Remove old certificate file if it exists
	if _, err := os.Stat(certPath); err == nil {
		os.Remove(certPath)
	}

	// Remove old key file if it exists
	if _, err := os.Stat(keyPath); err == nil {
		os.Remove(keyPath)
	}

	// Write certificate to file
	err = writeFileWithMode(m.CertStore, certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}), defaultPublicCertFileMode)
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
	err = writeFileWithMode(m.CertStore, keyFile, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: privBytes}), defaultPrivateKeyFileMode)
	if err != nil {
		m.Logger.PrintAndLog("tls-router", "Failed to write private key to file: "+keyFile, err)
		return err
	}
	m.Logger.PrintAndLog("tls-router", "Certificate and key generated: "+certFile+", "+keyFile, nil)
	return nil
}
