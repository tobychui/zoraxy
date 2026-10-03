package dpcore

import (
	"errors"
	"strings"
)

// ValidateH2C rejects conflicting settings before a saved route is replaced.
func ValidateH2C(target string, useH2C, requireTLS, forceHTTP11 bool) error {
	if !useH2C {
		return nil
	}
	if requireTLS || strings.HasPrefix(strings.ToLower(strings.TrimSpace(target)), "https://") {
		return errors.New("h2c is HTTP/2 without TLS; disable Require TLS or disable h2c")
	}
	if forceHTTP11 {
		return errors.New("disable Force HTTP/1.1 before enabling h2c")
	}
	return nil
}
