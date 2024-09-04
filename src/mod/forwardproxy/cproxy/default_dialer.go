package cproxy

import (
	"net"
	"time"
)

type defaultDialer struct {
	timeout time.Duration
	logger  logger
}

func newDialer(config *configuration) *defaultDialer {
	return &defaultDialer{timeout: config.DialTimeout, logger: config.Logger}
}

func (this *defaultDialer) Dial(address string) Socket {
	if socket, err := net.DialTimeout("tcp", address, this.timeout); err == nil {
		return socket
	} else {
		this.logger.Printf("Unable to establish connection to [%s]: %s", address, err)
	}

	return nil
}
