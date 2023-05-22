package tlscert

import (
	"crypto/tls"
	"crypto/x509"
	"embed"
	"encoding/pem"
	"io"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"imuslab.com/zoraxy/mod/utils"
)

type Manager struct {
	CertStore string
	verbal    bool
}

//go:embed localhost.crt localhost.key
var buildinCertStore embed.FS

func NewManager(certStore string, verbal bool) (*Manager, error) {
	if !utils.FileExists(certStore) {
		os.MkdirAll(certStore, 0775)
	}

	thisManager := Manager{
		CertStore: certStore,
		verbal:    verbal,
	}

	return &thisManager, nil
}

func (m *Manager) ListCertDomains() ([]string, error) {
	filenames, err := m.ListCerts()
	if err != nil {
		return []string{}, err
	}

	//Remove certificates where there are missing public key or private key
	filenames = getCertPairs(filenames)

	return filenames, nil
}

func (m *Manager) ListCerts() ([]string, error) {
	certs, err := ioutil.ReadDir(m.CertStore)
	if err != nil {
		return []string{}, err
	}

	filenames := make([]string, 0, len(certs))
	for _, cert := range certs {
		if !cert.IsDir() {
			filenames = append(filenames, cert.Name())
		}
	}

	return filenames, nil
}

func (m *Manager) GetCert(helloInfo *tls.ClientHelloInfo) (*tls.Certificate, error) {
	//Check if the domain corrisponding cert exists
	pubKey := "./tmp/localhost.crt"
	priKey := "./tmp/localhost.key"

	//Check if this is initial setup
	if !utils.FileExists(pubKey) {
		buildInPubKey, _ := buildinCertStore.ReadFile(filepath.Base(pubKey))
		os.WriteFile(pubKey, buildInPubKey, 0775)
	}

	if !utils.FileExists(priKey) {
		buildInPriKey, _ := buildinCertStore.ReadFile(filepath.Base(priKey))
		os.WriteFile(priKey, buildInPriKey, 0775)
	}

	if utils.FileExists(filepath.Join(m.CertStore, helloInfo.ServerName+".crt")) && utils.FileExists(filepath.Join(m.CertStore, helloInfo.ServerName+".key")) {
		pubKey = filepath.Join(m.CertStore, helloInfo.ServerName+".crt")
		priKey = filepath.Join(m.CertStore, helloInfo.ServerName+".key")

	} else {
		domainCerts, _ := m.ListCertDomains()
		cloestDomainCert := matchClosestDomainCertificate(helloInfo.ServerName, domainCerts)
		if cloestDomainCert != "" {
			//There is a matching parent domain for this subdomain. Use this instead.
			pubKey = filepath.Join(m.CertStore, cloestDomainCert+".crt")
			priKey = filepath.Join(m.CertStore, cloestDomainCert+".key")
		} else if m.DefaultCertExists() {
			//Use default.crt and default.key
			pubKey = filepath.Join(m.CertStore, "default.crt")
			priKey = filepath.Join(m.CertStore, "default.key")
			if m.verbal {
				log.Println("No matching certificate found. Serving with default")
			}
		} else {
			if m.verbal {
				log.Println("Matching certificate not found. Serving with build-in certificate. Requesting server name: ", helloInfo.ServerName)
			}
		}
	}

	//Load the cert and serve it
	cer, err := tls.LoadX509KeyPair(pubKey, priKey)
	if err != nil {
		log.Println(err)
		return nil, nil
	}

	return &cer, nil
}

// Check if both the default cert public key and private key exists
func (m *Manager) DefaultCertExists() bool {
	return utils.FileExists(filepath.Join(m.CertStore, "default.crt")) && utils.FileExists(filepath.Join(m.CertStore, "default.key"))
}

// Check if the default cert exists returning seperate results for pubkey and prikey
func (m *Manager) DefaultCertExistsSep() (bool, bool) {
	return utils.FileExists(filepath.Join(m.CertStore, "default.crt")), utils.FileExists(filepath.Join(m.CertStore, "default.key"))
}

// Delete the cert if exists
func (m *Manager) RemoveCert(domain string) error {
	pubKey := filepath.Join(m.CertStore, domain+".crt")
	priKey := filepath.Join(m.CertStore, domain+".key")
	if utils.FileExists(pubKey) {
		err := os.Remove(pubKey)
		if err != nil {
			return err
		}
	}

	if utils.FileExists(priKey) {
		err := os.Remove(priKey)
		if err != nil {
			return err
		}
	}

	return nil
}

// Check if the given file is a valid TLS file
func IsValidTLSFile(file io.Reader) bool {
	// Read the contents of the uploaded file
	contents, err := io.ReadAll(file)
	if err != nil {
		// Handle the error
		return false
	}

	// Parse the contents of the file as a PEM-encoded certificate or key
	block, _ := pem.Decode(contents)
	if block == nil {
		// The file is not a valid PEM-encoded certificate or key
		return false
	}

	// Parse the certificate or key
	if strings.Contains(block.Type, "CERTIFICATE") {
		// The file contains a certificate
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			// Handle the error
			return false
		}
		// Check if the certificate is a valid TLS/SSL certificate
		return cert.IsCA == false && cert.KeyUsage&x509.KeyUsageDigitalSignature != 0 && cert.KeyUsage&x509.KeyUsageKeyEncipherment != 0
	} else if strings.Contains(block.Type, "PRIVATE KEY") {
		// The file contains a private key
		_, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			// Handle the error
			return false
		}
		return true
	} else {
		return false
	}

}
