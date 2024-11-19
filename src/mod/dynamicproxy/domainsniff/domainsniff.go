package domainsniff

/*
	Domainsniff

	This package contain codes that perform project / domain specific behavior in Zoraxy
	If you want Zoraxy to handle a particular domain or open source project in a special way,
	you can add the checking logic here.

*/
import (
	"crypto/tls"
	"net"
	"time"
)

// Check if the domain is reachable and return err if not reachable
func DomainReachableWithError(domain string) error {
	timeout := 1 * time.Second
	conn, err := net.DialTimeout("tcp", domain, timeout)
	if err != nil {
		return err
	}

	conn.Close()
	return nil
}

// Check if a domain have TLS but it is self-signed or expired
func DomainIsSelfSigned(domain string) (bool, error) {
	//Get the certificate
	conn, err := net.Dial("tcp", domain)
	if err != nil {
		return false, err
	}
	defer conn.Close()

	//Connect with TLS using insecure skip verify
	config := &tls.Config{
		InsecureSkipVerify: true,
	}
	tlsConn := tls.Client(conn, config)
	err = tlsConn.Handshake()
	if err != nil {
		return false, err
	}

	//Check if the certificate is self-signed
	cert := tlsConn.ConnectionState().PeerCertificates[0]
	return cert.Issuer.CommonName == cert.Subject.CommonName, nil
}

// Check if domain reachable
func DomainReachable(domain string) bool {
	return DomainReachableWithError(domain) == nil
}
