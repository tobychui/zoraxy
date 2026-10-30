package mtls

/*
	mTLS (mutual TLS) client CA manager

	Stores the CA certificates that Zoraxy trusts when a client presents a
	certificate on a host that has mTLS enabled, and provides helpers to
	verify a client certificate against a selected set of those CAs.
*/

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"imuslab.com/zoraxy/mod/info/logger"
)

const (
	ModeOff      = "off"      //Do not ask clients for a certificate
	ModeOptional = "optional" //Verify the certificate if the client sends one, but do not require it
	ModeRequired = "required" //Reject clients without a valid certificate
)

var validCAName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._ -]{0,63}$`)

// Summary of a stored client CA bundle, safe to return to the web UI
type CAInfo struct {
	Name        string
	Subject     string
	Issuer      string
	NotBefore   int64
	NotAfter    int64
	Expired     bool
	IsCA        bool
	CertCount   int
	Fingerprint string //SHA-256 of the first certificate in the bundle
}

type Manager struct {
	dir    string
	logger *logger.Logger

	mu         sync.RWMutex
	generation uint64 //Bumped whenever the CA store changes, use to invalidate cached TLS configs
	poolCache  map[string]*x509.CertPool
}

func NewManager(dir string, logger *logger.Logger) (*Manager, error) {
	if err := os.MkdirAll(dir, 0775); err != nil {
		return nil, err
	}
	return &Manager{
		dir:       dir,
		logger:    logger,
		poolCache: map[string]*x509.CertPool{},
	}, nil
}

// Generation returns a counter that changes every time a CA is added or removed
func (m *Manager) Generation() uint64 {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.generation
}

func (m *Manager) caPath(name string) (string, error) {
	name = strings.TrimSpace(name)
	if !validCAName.MatchString(name) || strings.Contains(name, "..") {
		return "", errors.New("invalid CA name: use letters, numbers, space, dot, dash or underscore (max 64 characters)")
	}
	return filepath.Join(m.dir, name+".pem"), nil
}

// parseCertificates extracts every certificate from a PEM bundle. Private keys are refused
// so that a user cannot accidentally store a secret in the trust store.
func parseCertificates(pemData []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	rest := pemData
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if strings.Contains(block.Type, "PRIVATE KEY") {
			return nil, errors.New("the file contains a private key. Only upload the public certificate")
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, errors.New("invalid certificate: " + err.Error())
		}
		certs = append(certs, cert)
	}
	if len(certs) == 0 {
		return nil, errors.New("no PEM encoded certificate found")
	}
	return certs, nil
}

// AddCA stores a PEM bundle of trusted client CA certificates under the given name
func (m *Manager) AddCA(name string, pemData []byte) error {
	path, err := m.caPath(name)
	if err != nil {
		return err
	}
	certs, err := parseCertificates(pemData)
	if err != nil {
		return err
	}

	//Re-encode so only the certificate blocks are persisted
	var out []byte
	for _, c := range certs {
		out = append(out, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: c.Raw})...)
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if _, err := os.Stat(path); err == nil {
		return errors.New("a CA with this name already exists")
	}
	if err := os.WriteFile(path, out, 0644); err != nil {
		return err
	}
	m.invalidateLocked()
	return nil
}

// RemoveCA deletes a stored CA bundle
func (m *Manager) RemoveCA(name string) error {
	path, err := m.caPath(name)
	if err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if err := os.Remove(path); err != nil {
		return err
	}
	m.invalidateLocked()
	return nil
}

func (m *Manager) invalidateLocked() {
	m.generation++
	m.poolCache = map[string]*x509.CertPool{}
}

// ListCAs returns a summary of every stored CA bundle
func (m *Manager) ListCAs() []*CAInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := []*CAInfo{}
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return result
	}
	now := time.Now()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pem") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(m.dir, entry.Name()))
		if err != nil {
			continue
		}
		certs, err := parseCertificates(data)
		if err != nil {
			continue
		}
		first := certs[0]
		sum := sha256.Sum256(first.Raw)
		result = append(result, &CAInfo{
			Name:        strings.TrimSuffix(entry.Name(), ".pem"),
			Subject:     first.Subject.String(),
			Issuer:      first.Issuer.String(),
			NotBefore:   first.NotBefore.Unix(),
			NotAfter:    first.NotAfter.Unix(),
			Expired:     now.After(first.NotAfter),
			IsCA:        first.IsCA,
			CertCount:   len(certs),
			Fingerprint: hex.EncodeToString(sum[:]),
		})
	}
	return result
}

// CAExists checks if a CA bundle with the given name is stored
func (m *Manager) CAExists(name string) bool {
	path, err := m.caPath(name)
	if err != nil {
		return false
	}
	_, err = os.Stat(path)
	return err == nil
}

// GetPool builds a certificate pool from the named CA bundles. Unknown names are
// skipped, so an empty result trusts nothing and every client will be rejected.
func (m *Manager) GetPool(names []string) *x509.CertPool {
	sorted := append([]string{}, names...)
	sort.Strings(sorted)
	key := strings.Join(sorted, "\n")

	m.mu.RLock()
	cached, ok := m.poolCache[key]
	m.mu.RUnlock()
	if ok {
		return cached
	}

	pool := x509.NewCertPool()
	for _, name := range sorted {
		path, err := m.caPath(name)
		if err != nil {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		certs, err := parseCertificates(data)
		if err != nil {
			continue
		}
		for _, c := range certs {
			pool.AddCert(c)
		}
	}

	m.mu.Lock()
	m.poolCache[key] = pool
	m.mu.Unlock()
	return pool
}

// VerifyPeer verifies the client certificate presented on a TLS connection against the
// pool and returns the leaf certificate, or nil if there is none or it is not trusted.
func VerifyPeer(state *tls.ConnectionState, pool *x509.CertPool) *x509.Certificate {
	if state == nil || len(state.PeerCertificates) == 0 || pool == nil {
		return nil
	}
	leaf := state.PeerCertificates[0]
	intermediates := x509.NewCertPool()
	for _, c := range state.PeerCertificates[1:] {
		intermediates.AddCert(c)
	}
	_, err := leaf.Verify(x509.VerifyOptions{
		Roots:         pool,
		Intermediates: intermediates,
		KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	})
	if err != nil {
		return nil
	}
	return leaf
}

// Fingerprint returns the hex encoded SHA-256 fingerprint of a certificate
func Fingerprint(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.Raw)
	return hex.EncodeToString(sum[:])
}
