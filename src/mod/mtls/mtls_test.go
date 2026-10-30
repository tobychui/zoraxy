package mtls

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"
)

type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
	pem  []byte
}

func newTestCA(t *testing.T, cn string) *testCA {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	return &testCA{cert, key, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})}
}

func (ca *testCA) issueClient(t *testing.T, cn string, expired bool) *x509.Certificate {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	notAfter := time.Now().Add(time.Hour)
	if expired {
		notAfter = time.Now().Add(-time.Minute)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: cn},
		NotBefore:    time.Now().Add(-2 * time.Hour),
		NotAfter:     notAfter,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, &key.PublicKey, ca.key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	return cert
}

func TestAddListRemoveCA(t *testing.T) {
	m, err := NewManager(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	ca := newTestCA(t, "Home CA")

	gen := m.Generation()
	if err := m.AddCA("Home CA", ca.pem); err != nil {
		t.Fatal(err)
	}
	if m.Generation() == gen {
		t.Error("generation should change after adding a CA")
	}
	if err := m.AddCA("Home CA", ca.pem); err == nil {
		t.Error("adding a duplicate name should fail")
	}

	list := m.ListCAs()
	if len(list) != 1 || list[0].Name != "Home CA" || !list[0].IsCA || list[0].Expired {
		t.Fatalf("unexpected list: %+v", list)
	}
	if !m.CAExists("Home CA") || m.CAExists("missing") {
		t.Error("CAExists mismatch")
	}

	if err := m.RemoveCA("Home CA"); err != nil {
		t.Fatal(err)
	}
	if len(m.ListCAs()) != 0 {
		t.Error("CA should be gone")
	}
}

func TestAddCARejectsBadInput(t *testing.T) {
	m, _ := NewManager(t.TempDir(), nil)
	ca := newTestCA(t, "CA")

	for _, name := range []string{"", "../evil", "a/b", `a\b`, ".hidden"} {
		if err := m.AddCA(name, ca.pem); err == nil {
			t.Errorf("name %q should be rejected", name)
		}
	}
	if err := m.AddCA("junk", []byte("not a certificate")); err == nil {
		t.Error("non PEM input should be rejected")
	}

	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	keyDer, _ := x509.MarshalECPrivateKey(key)
	withKey := append(append([]byte{}, ca.pem...), pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDer})...)
	if err := m.AddCA("withkey", withKey); err == nil {
		t.Error("a bundle containing a private key should be rejected")
	}
	if len(m.ListCAs()) != 0 {
		t.Error("nothing should have been stored")
	}
}

func TestVerifyPeer(t *testing.T) {
	m, _ := NewManager(t.TempDir(), nil)
	trusted := newTestCA(t, "Trusted")
	rogue := newTestCA(t, "Rogue")
	if err := m.AddCA("trusted", trusted.pem); err != nil {
		t.Fatal(err)
	}
	pool := m.GetPool([]string{"trusted"})

	good := trusted.issueClient(t, "alice", false)
	if VerifyPeer(&tls.ConnectionState{PeerCertificates: []*x509.Certificate{good}}, pool) == nil {
		t.Error("certificate from the trusted CA should verify")
	}
	if VerifyPeer(&tls.ConnectionState{PeerCertificates: []*x509.Certificate{rogue.issueClient(t, "mallory", false)}}, pool) != nil {
		t.Error("certificate from another CA must not verify")
	}
	if VerifyPeer(&tls.ConnectionState{PeerCertificates: []*x509.Certificate{trusted.issueClient(t, "old", true)}}, pool) != nil {
		t.Error("expired certificate must not verify")
	}
	if VerifyPeer(&tls.ConnectionState{}, pool) != nil {
		t.Error("no certificate must not verify")
	}
	if VerifyPeer(&tls.ConnectionState{PeerCertificates: []*x509.Certificate{good}}, m.GetPool([]string{"unknown"})) != nil {
		t.Error("an empty pool must trust nothing")
	}
}
