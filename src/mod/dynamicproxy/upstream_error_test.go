package dynamicproxy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"syscall"
	"testing"
)

type timeoutError struct{}

func (timeoutError) Error() string   { return "i/o timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

func TestUpstreamErrorStatusCode(t *testing.T) {
	refused := &net.OpError{Op: "dial", Net: "tcp", Err: syscall.ECONNREFUSED}
	tests := []struct {
		name            string
		proxyStatusCode int
		err             error
		want            int
	}{
		{"connection refused", http.StatusBadGateway, refused, http.StatusBadGateway},
		{"wrapped connection refused", http.StatusBadGateway, fmt.Errorf("proxy: %w", refused), http.StatusBadGateway},
		{"dial or response header timeout", http.StatusBadGateway, &net.OpError{Op: "dial", Net: "tcp", Err: timeoutError{}}, http.StatusGatewayTimeout},
		{"wrapped timeout", http.StatusBadGateway, fmt.Errorf("proxy: %w", timeoutError{}), http.StatusGatewayTimeout},
		{"context deadline", http.StatusBadGateway, context.DeadlineExceeded, http.StatusGatewayTimeout},
		{"tunnel deadline reported by dpcore", http.StatusGatewayTimeout, errors.New("set deadline failed"), http.StatusGatewayTimeout},
		{"other proxy errors keep the legacy code", http.StatusInternalServerError, errors.New("hijack failed"), 521},
		{"invalid upgrade keeps the legacy code", http.StatusBadRequest, errors.New("client requested an invalid upgrade protocol"), 521},
	}
	for _, tt := range tests {
		if got := upstreamErrorStatusCode(tt.proxyStatusCode, tt.err); got != tt.want {
			t.Errorf("%s: got %d, want %d", tt.name, got, tt.want)
		}
	}
}
