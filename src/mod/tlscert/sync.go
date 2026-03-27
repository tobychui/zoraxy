package tlscert

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
)

type StoredCertificate struct {
	Name       string          `json:"name"`
	PublicKey  string          `json:"public_key"`
	PrivateKey string          `json:"private_key"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
}

// ReplaceStoredCertificatesAtPath replaces all certificates in the target cert store.
func ReplaceStoredCertificatesAtPath(certStore string, certificates []StoredCertificate) error {
	if err := os.MkdirAll(certStore, 0775); err != nil {
		return err
	}

	patterns := []string{"*.pem", "*.key", "*.json"}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(filepath.Join(certStore, pattern))
		if err != nil {
			return err
		}

		for _, match := range matches {
			if err := os.Remove(match); err != nil && !os.IsNotExist(err) {
				return err
			}
		}
	}

	sort.Slice(certificates, func(i, j int) bool {
		return certificates[i].Name < certificates[j].Name
	})

	for _, certificate := range certificates {
		certName := filepath.Base(certificate.Name)
		if certName == "." || certName == "" {
			return errors.New("invalid certificate name")
		}

		if err := os.WriteFile(filepath.Join(certStore, certName+".pem"), []byte(certificate.PublicKey), 0775); err != nil {
			return err
		}

		if err := os.WriteFile(filepath.Join(certStore, certName+".key"), []byte(certificate.PrivateKey), 0775); err != nil {
			return err
		}

		if len(certificate.Metadata) > 0 {
			if err := os.WriteFile(filepath.Join(certStore, certName+".json"), certificate.Metadata, 0775); err != nil {
				return err
			}
		}
	}

	return nil
}

// ExportStoredCertificates exports all certificate key pairs and metadata from the cert store.
func (m *Manager) ExportStoredCertificates() ([]StoredCertificate, error) {
	if m == nil {
		return nil, errors.New("tls certificate manager is not initialized")
	}

	domains, err := m.ListCertDomains()
	if err != nil {
		return nil, err
	}

	sort.Strings(domains)
	results := make([]StoredCertificate, 0, len(domains))
	for _, domain := range domains {
		pubKeyPath := filepath.Join(m.CertStore, domain+".pem")
		priKeyPath := filepath.Join(m.CertStore, domain+".key")
		metadataPath := filepath.Join(m.CertStore, domain+".json")

		pubKey, err := os.ReadFile(pubKeyPath)
		if err != nil {
			return nil, err
		}

		priKey, err := os.ReadFile(priKeyPath)
		if err != nil {
			return nil, err
		}

		var metadata json.RawMessage
		if metadataBytes, err := os.ReadFile(metadataPath); err == nil {
			metadata = metadataBytes
		}

		results = append(results, StoredCertificate{
			Name:       domain,
			PublicKey:  string(pubKey),
			PrivateKey: string(priKey),
			Metadata:   metadata,
		})
	}

	return results, nil
}

// ReplaceStoredCertificates replaces all certificates in the cert store.
func (m *Manager) ReplaceStoredCertificates(certificates []StoredCertificate) error {
	if m == nil {
		return errors.New("tls certificate manager is not initialized")
	}

	if err := ReplaceStoredCertificatesAtPath(m.CertStore, certificates); err != nil {
		return err
	}

	return m.UpdateLoadedCertList()
}
