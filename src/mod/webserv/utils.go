package webserv

import (
	"net"
)

// IsPortInUse checks if a port is in use.
func IsPortInUse(port string) bool {
	listener, err := net.Listen("tcp", "localhost:"+port)
	if err != nil {
		// If there was an error, the port is in use.
		return true
	}
	defer listener.Close()

	// No error means the port is available.
	return false
}
