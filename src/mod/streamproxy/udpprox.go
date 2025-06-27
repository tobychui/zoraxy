package streamproxy

import (
	"errors"
	"log"
	"net"
	"strings"
	"time"
)

/*
	UDP Proxy Module
*/

// Information maintained for each client/server connection
type udpClientServerConn struct {
	ClientAddr *net.UDPAddr // Address of the client
	ServerConn *net.UDPConn // UDP connection to server
}

// Generate a new connection by opening a UDP connection to the server
func createNewUDPConn(srvAddr, cliAddr *net.UDPAddr) *udpClientServerConn {
	conn := new(udpClientServerConn)
	conn.ClientAddr = cliAddr
	srvudp, err := net.DialUDP("udp", nil, srvAddr)
	if err != nil {
		return nil
	}
	conn.ServerConn = srvudp
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
func (c *ProxyRelayConfig) RunUDPConnectionRelay(conn *udpClientServerConn, lisenter *net.UDPConn) {
	var buffer [1500]byte
	for {
		// Read from server
		n, err := conn.ServerConn.Read(buffer[0:])
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return
			}
			continue
		}
		// Relay it to client
		_, err = lisenter.WriteToUDP(buffer[0:n], conn.ClientAddr)
		if err != nil {
			continue
		}

	}
}

// Close all connections that waiting for read from server
func (c *ProxyRelayConfig) CloseAllUDPConnections() {
	c.udpClientMap.Range(func(clientAddr, clientServerConn interface{}) bool {
		conn := clientServerConn.(*udpClientServerConn)
		conn.ServerConn.Close()
		return true
	})
}

func (c *ProxyRelayConfig) ForwardUDP(address1, address2 string, stopChan chan bool) error {
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
		c.aTobAccumulatedByteTransfer.Add(int64(n))
		saddr := cliaddr.String()
		rawConn, found := c.udpClientMap.Load(saddr)
		var conn *udpClientServerConn
		if !found {
			conn = createNewUDPConn(targetAddr, cliaddr)
			if conn == nil {
				continue
			}
			c.udpClientMap.Store(saddr, conn)
			log.Println("[UDP] Created new connection for client " + saddr)
			// Fire up routine to manage new connection
			go c.RunUDPConnectionRelay(conn, lisener)

		} else {
			log.Println("[UDP] Found connection for client " + saddr)
			conn = rawConn.(*udpClientServerConn)
		}

		// Relay to server
		_, err = conn.ServerConn.Write(buffer[0:n])
		if err != nil {
			continue
		}

	}
}
