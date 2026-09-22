package dynamicproxy_test

import (
	"crypto/tls"
	"reflect"
	"testing"

	"imuslab.com/zoraxy/mod/dynamicproxy"
)

func TestApplyTlsCipherProfile(t *testing.T) {
	intermediateExpected := []uint16{
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
		tls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
		tls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
	}

	tests := []struct {
		name             string
		profile          string
		minVersion       uint16
		wantMinVersion   uint16
		wantCipherSuites []uint16
	}{
		{
			name:           "default profile keeps Go defaults",
			profile:        dynamicproxy.TlsCipherProfileDefault,
			minVersion:     tls.VersionTLS12,
			wantMinVersion: tls.VersionTLS12,
		},
		{
			name:           "unknown profile keeps Go defaults",
			profile:        "bogus",
			minVersion:     tls.VersionTLS12,
			wantMinVersion: tls.VersionTLS12,
		},
		{
			name:             "intermediate restricts TLS 1.2 suites to AEAD",
			profile:          dynamicproxy.TlsCipherProfileIntermediate,
			minVersion:       tls.VersionTLS12,
			wantMinVersion:   tls.VersionTLS12,
			wantCipherSuites: intermediateExpected,
		},
		{
			name:           "modern requires TLS 1.3",
			profile:        dynamicproxy.TlsCipherProfileModern,
			minVersion:     tls.VersionTLS12,
			wantMinVersion: tls.VersionTLS13,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &tls.Config{MinVersion: tt.minVersion}
			dynamicproxy.ApplyTlsCipherProfile(config, tt.profile)

			if config.MinVersion != tt.wantMinVersion {
				t.Errorf("MinVersion = 0x%04x, want 0x%04x", config.MinVersion, tt.wantMinVersion)
			}

			if tt.wantCipherSuites == nil {
				if config.CipherSuites != nil {
					t.Errorf("CipherSuites = %v, want nil (Go defaults)", config.CipherSuites)
				}
			} else if !reflect.DeepEqual(config.CipherSuites, tt.wantCipherSuites) {
				t.Errorf("CipherSuites = %v, want %v", config.CipherSuites, tt.wantCipherSuites)
			}
		})
	}
}

func TestIntermediateProfileExcludesWeakSuites(t *testing.T) {
	config := &tls.Config{MinVersion: tls.VersionTLS12}
	dynamicproxy.ApplyTlsCipherProfile(config, dynamicproxy.TlsCipherProfileIntermediate)

	excluded := map[uint16]string{
		0xC013: "TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA",
		0xC014: "TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA",
		0xC009: "TLS_ECDHE_ECDSA_WITH_AES_128_CBC_SHA",
		0xC00A: "TLS_ECDHE_ECDSA_WITH_AES_256_CBC_SHA",
		0x009C: "TLS_RSA_WITH_AES_128_GCM_SHA256",
		0x009D: "TLS_RSA_WITH_AES_256_GCM_SHA384",
	}

	for _, suite := range config.CipherSuites {
		if name, ok := excluded[suite]; ok {
			t.Errorf("intermediate profile must not include weak suite %s (0x%04x)", name, suite)
		}
	}
}
