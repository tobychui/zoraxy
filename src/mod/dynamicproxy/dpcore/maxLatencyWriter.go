package dpcore

/*

Max Latency Writer

This script implements a io writer with periodic flushing base on a ticker
Mostly based on httputil.ReverseProxy

*/

import (
	"io"
	"sync"
	"time"
)

type maxLatencyWriter struct {
	dst          io.Writer
	flush        func() error
	latency      time.Duration // non-zero; negative means to flush immediately
	mu           sync.Mutex    // protects t, flushPending, and dst.Flush
	t            *time.Timer
	flushPending bool
}

func (m *maxLatencyWriter) Write(p []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, err = m.dst.Write(p)
	if m.latency < 0 {
		//Flush immediately
		m.flush()
		return
	}

	if m.flushPending {
		//Flush in next tick cycle
		return
	}

	if m.t == nil {
		m.t = time.AfterFunc(m.latency, m.delayedFlush)
	} else {
		m.t.Reset(m.latency)
	}

	m.flushPending = true
	return

}

func (m *maxLatencyWriter) delayedFlush() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.flushPending {
		// if stop was called but AfterFunc already started this goroutine
		return
	}

	m.flush()
	m.flushPending = false
}

func (m *maxLatencyWriter) stop() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.flushPending = false
	if m.t != nil {
		m.t.Stop()
	}
}
