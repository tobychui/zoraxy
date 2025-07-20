package streamproxy_test

import (
	"testing"
	"time"

	"imuslab.com/zoraxy/mod/streamproxy"
)

func TestPort2Port(t *testing.T) {
	// Create a stopChan to control the loop
	stopChan := make(chan bool)

	// Create a ProxyRelayConfig with dummy values
	config := &streamproxy.ProxyRelayInstance{
		Timeout: 1,
	}

	// Run port2port in a separate goroutine
	t.Log("Starting go routine for proxy service")
	go func() {
		err := config.Port2host("8080", "124.244.86.40:8080", stopChan)
		if err != nil {
			t.Errorf("port2port returned an error: %v", err)
		}
	}()

	// Let the goroutine run for a while
	time.Sleep(20 * time.Second)

	// Send a stop signal to stopChan
	t.Log("Sending over stop signal")
	stopChan <- true

	// Allow some time for the goroutine to exit
	time.Sleep(1 * time.Second)

	// If the goroutine is still running, it means it did not stop as expected
	if config.IsRunning() {
		t.Errorf("port2port did not stop as expected")
	}

}
