package geodb

import (
	"testing"
)

func BenchmarkGetReservedIPZone(b *testing.B) {
	testCases := []string{
		"127.0.0.1",
		"10.0.0.1",
		"192.168.1.1",
		"172.16.0.1",
		"169.254.1.1",
		"224.0.0.1",
		"::1",
		"fe80::1",
		"fc00::1",
		"ff00::1",
		"3.224.220.101", // Public IP (not reserved)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		for _, ip := range testCases {
			getReservedIPZone(ip)
		}
	}
}

func BenchmarkGetReservedIPZone_PrivateIPv4(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getReservedIPZone("192.168.1.1")
	}
}

func BenchmarkGetReservedIPZone_PrivateIPv6(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getReservedIPZone("fe80::1")
	}
}

func BenchmarkGetReservedIPZone_PublicIP(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		getReservedIPZone("8.8.8.8")
	}
}
