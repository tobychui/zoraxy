package cproxy

import (
	"io"
	"net"
	"net/http"
)

type (
	Filter interface {
		IsAuthorized(http.ResponseWriter, *http.Request) bool
	}

	clientConnector interface {
		Connect(http.ResponseWriter) Socket
	}
)

type (
	Dialer interface {
		Dial(string) Socket
	}

	serverConnector interface {
		Connect(Socket, string) proxy
	}

	initializer interface {
		Initialize(Socket, Socket) bool
	}

	proxy interface {
		Proxy()
	}
)

type (
	Socket interface {
		io.ReadWriteCloser
		RemoteAddr() net.Addr
	}

	tcpSocket interface {
		Socket
		CloseRead() error
		CloseWrite() error
	}
)

type (
	monitor interface {
		Measure(int)
	}
	logger interface {
		Printf(string, ...interface{})
	}
)

const (
	MeasurementHTTPRequest int = iota
	MeasurementBadMethod
	MeasurementUnauthorizedRequest
	MeasurementClientConnectionFailed
	MeasurementServerConnectionFailed
	MeasurementProxyReady
	MeasurementProxyComplete
)
