package streamproxy

import (
	"bytes"
	"errors"
	"log"
	"net"
	"os"
	"strings"
	"time"

	proxyproto "github.com/pires/go-proxyproto"
)

/*
	UDP Proxy Module
*/

// defaultUDPTimeout is the idle-session timeout used for UDP proxy sessions
const defaultUDPTimeout = 60 * time.Second

// Information maintained for each client/server connection
type udpClientServerConn struct {
	ClientAddr *net.UDPAddr  // Address of the client
	ServerConn *net.UDPConn  // UDP connection to server
	timeout    time.Duration // Idle timeout for this session; 0 disables expiry
}

// udpIdleTimeout returns the timeout value of this rule
func (c *ProxyRelayInstance) udpIdleTimeout() time.Duration {
	if c.Timeout > 0 {
		return time.Duration(c.Timeout) * time.Second
	}
	return defaultUDPTimeout
}

// Generate a new connection by opening a UDP connection to the server
func createNewUDPConn(srvAddr, cliAddr *net.UDPAddr, timeout time.Duration) *udpClientServerConn {
	conn := new(udpClientServerConn)
	conn.ClientAddr = cliAddr
	srvudp, err := net.DialUDP("udp", nil, srvAddr)
	if err != nil {
		return nil
	}
	conn.ServerConn = srvudp
	conn.timeout = timeout
	return conn
}

// Start listener, return inbound lisener and proxy target UDP address
func initUDPConnections(listenAddr string, targetAddress string) (*net.UDPConn, *net.UDPAddr, error) {
	// Set up Proxy
	saddr, err := net.ResolveUDPAddr("udp", listenAddr)
	if err != nil {
		return nil, nil, err
	}
	inboundConn, err := net.ListenUDP("udp", saddr)
	if err != nil {
		return nil, nil, err
	}

	log.Println("[UDP] Proxy listening on " + listenAddr)

	outboundConn, err := net.ResolveUDPAddr("udp", targetAddress)
	if err != nil {
		return nil, nil, err
	}

	return inboundConn, outboundConn, nil
}

// Go routine which manages connection from server to single client
func (c *ProxyRelayInstance) RunUDPConnectionRelay(conn *udpClientServerConn, lisenter *net.UDPConn) {
	// Fix for #1207:
	// Release the per-client socket and remove the map entry on start
	saddr := conn.ClientAddr.String()
	defer func() {
		conn.ServerConn.Close()
		// Only delete if the map still points to *this* connection so we never
		// clobber a freshly recreated session for the same client address.
		c.udpClientMap.CompareAndDelete(saddr, conn)
	}()

	var buffer [1500]byte
	for {
		// Refresh the idle deadline before each read
		if conn.timeout > 0 {
			conn.ServerConn.SetReadDeadline(time.Now().Add(conn.timeout))
		}

		// Read from server
		n, err := conn.ServerConn.Read(buffer[0:])
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				// Socket closed by proxy shutdown or session teardown
				return
			}
			if errors.Is(err, os.ErrDeadlineExceeded) {
				// Session was idle for longer than the timeout window
				c.LogMsg("[UDP] Idle session for client "+saddr+" expired after "+conn.timeout.String(), nil)
				return
			}

			c.LogMsg("[UDP] Read error for client "+saddr+", closing session: "+err.Error(), nil)
			return
		}
		// Relay it to client
		_, err = lisenter.WriteToUDP(buffer[0:n], conn.ClientAddr)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				// Inbound listener gone (proxy stopping)
				return
			}
			continue
		}

	}
}

// Close all connections that waiting for read from server
func (c *ProxyRelayInstance) CloseAllUDPConnections() {
	c.udpClientMap.Range(func(clientAddr, clientServerConn interface{}) bool {
		conn := clientServerConn.(*udpClientServerConn)
		conn.ServerConn.Close()
		return true
	})
}

// Write Proxy Protocol v2 header to UDP connection
func WriteProxyProtocolHeaderUDP(conn *net.UDPConn, srcAddr, dstAddr *net.UDPAddr) error {
	header := proxyproto.Header{
		Version:           byte(convertProxyProtocolVersionToInt(ProxyProtocolV2)),
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.UDPv4,
		SourceAddr:        srcAddr,
		DestinationAddr:   dstAddr,
	}
	var buf bytes.Buffer
	_, err := header.WriteTo(&buf)
	if err != nil {
		return err
	}
	_, err = conn.Write(buf.Bytes())
	return err
}

func (c *ProxyRelayInstance) ForwardUDP(address1, address2 string, stopChan chan bool) error {
	//By default the incoming listen Address is int
	//We need to add the loopback address into it
	if isValidPort(address1) {
		//Port number only. Missing the : in front
		address1 = ":" + address1
	}
	if strings.HasPrefix(address1, ":") {
		//Prepend 0.0.0.0 to the address
		address1 = "0.0.0.0" + address1
	}

	lisener, targetAddr, err := initUDPConnections(address1, address2)
	if err != nil {
		return err
	}

	go func() {
		//Stop channel receiver
		for {
			select {
			case <-stopChan:
				//Stop signal received
				//Stop server -> client forwarder
				c.CloseAllUDPConnections()
				//Stop client -> server forwarder
				//Force close, will terminate ReadFromUDP for inbound listener
				lisener.Close()
				return
			default:
				time.Sleep(100 * time.Millisecond)
			}
		}

	}()

	var buffer [1500]byte
	for {
		n, cliaddr, err := lisener.ReadFromUDP(buffer[0:])
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				//Proxy stopped
				return nil
			}
			continue
		}
		*c.aTobAccumulatedByteTransfer += int64(n)
		saddr := cliaddr.String()
		rawConn, found := c.udpClientMap.Load(saddr)
		var conn *udpClientServerConn
		if !found {
			// Check if this new client is allowed by access control policy
			accessRule := c.getEffectiveAccessRule()
			if accessRule != nil && !accessRule.AllowIpAccess(cliaddr.IP.String()) {
				c.LogMsg("[UDP] Connection from "+cliaddr.IP.String()+" rejected by access control policy", nil)
				continue
			}

			conn = createNewUDPConn(targetAddr, cliaddr, c.udpIdleTimeout())
			if conn == nil {
				continue
			}
			c.udpClientMap.Store(saddr, conn)
			c.LogMsg("[UDP] Created new connection for client "+saddr, nil)
			// Fire up routine to manage new connection
			go c.RunUDPConnectionRelay(conn, lisener)

			// Send Proxy Protocol header if enabled
			if c.ProxyProtocolVersion == ProxyProtocolV2 {
				_ = WriteProxyProtocolHeaderUDP(conn.ServerConn, cliaddr, targetAddr)
			}
		} else {
			c.LogMsg("[UDP] Found connection for client "+saddr, nil)
			conn = rawConn.(*udpClientServerConn)
		}

		// The client just sent a packet, update idle deadline
		if conn.timeout > 0 {
			conn.ServerConn.SetReadDeadline(time.Now().Add(conn.timeout))
		}

		// Relay to server
		_, err = conn.ServerConn.Write(buffer[0:n])
		if err != nil {
			continue
		}

	}
}
