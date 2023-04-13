package domainsniff

import (
	"net"
	"time"
)

//Check if the domain is reachable and return err if not reachable
func DomainReachableWithError(domain string) error {
	timeout := 1 * time.Second
	conn, err := net.DialTimeout("tcp", domain, timeout)
	if err != nil {
		return err
	}

	conn.Close()
	return nil
}

//Check if domain reachable
func DomainReachable(domain string) bool {
	return DomainReachableWithError(domain) == nil
}
