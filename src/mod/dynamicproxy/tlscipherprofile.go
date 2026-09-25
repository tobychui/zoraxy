package dynamicproxy

import (
	"crypto/tls"
)

/*
	tlscipherprofile.go

	TLS cipher profile handling for the reverse proxy TLS listeners.

	The profiles follow the Mozilla TLS guidelines (guideline v6.0,
	https://configurator.tlsref.org / https://data.tlsref.org/guidelines/6.0.json):

	  intermediate: TLS 1.2 + 1.3, with TLS 1.2 cipher suites restricted to
	                ECDHE + AEAD (AES-GCM / ChaCha20-Poly1305). Removes the
	                legacy CBC-SHA1 suites still enabled in Go's defaults.
	  modern:       TLS 1.3 only.

	Note: cipher suite filtering only affects TLS 1.2. TLS 1.3 suites are not
	configurable in Go, and the key exchange groups (which carry the
	post-quantum hybrids like X25519MLKEM768) are left at their Go defaults.

	Added in Zoraxy v3.3.6 by Michael Lux
*/

const (
	TlsCipherProfileDefault      = "default"
	TlsCipherProfileIntermediate = "intermediate"
	TlsCipherProfileModern       = "modern"
)

// mozillaIntermediateCipherSuites is the TLS 1.2 cipher suite list of the
// Mozilla "intermediate" recommendation (guideline v6.0), in guideline
// preference order.
var mozillaIntermediateCipherSuites = []uint16{
	tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
	tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
	tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
	tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
}

// filterSupportedCipherSuites returns the given suites restricted to those
// enabled by this Go build for TLS 1.2, keeping the input order.
func filterSupportedCipherSuites(suites []uint16) []uint16 {
	supported := make(map[uint16]bool)
	for _, cs := range tls.CipherSuites() {
		for _, v := range cs.SupportedVersions {
			if v == tls.VersionTLS12 {
				supported[cs.ID] = true
				break
			}
		}
	}

	filtered := make([]uint16, 0, len(suites))
	for _, id := range suites {
		if supported[id] {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

// ApplyTlsCipherProfile applies the given cipher profile to a TLS server
// config. Unknown profiles (including the empty default) leave the config
// at Go's built-in defaults.
func ApplyTlsCipherProfile(config *tls.Config, profile string) {
	switch profile {
	case TlsCipherProfileIntermediate:
		//Restrict the TLS 1.2 cipher suites to the Mozilla intermediate list
		config.CipherSuites = filterSupportedCipherSuites(mozillaIntermediateCipherSuites)
	case TlsCipherProfileModern:
		//Modern profile: TLS 1.3 only (TLS 1.3 suites are not configurable)
		config.MinVersion = tls.VersionTLS13
	}
}

// MinTLSVersionForProfile returns the minimum TLS version required by the
// given cipher profile, or 0 if the profile imposes no requirement. The
// intermediate profile only offers TLS 1.2 suites, so TLS 1.0/1.1 minimums
// would leave those protocol versions without any usable cipher suite;
// modern is TLS 1.3 only by definition.
func MinTLSVersionForProfile(profile string) uint16 {
	switch profile {
	case TlsCipherProfileIntermediate:
		return tls.VersionTLS12
	case TlsCipherProfileModern:
		return tls.VersionTLS13
	}
	return 0
}
