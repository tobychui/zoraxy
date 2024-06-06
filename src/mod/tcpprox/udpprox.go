package tcpprox

import (
	"log"
	"net"
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

	log.Println("Proxy serving on port %s\n", listenAddr)

	outboundConn, err := net.ResolveUDPAddr("udp", targetAddress)
	if err != nil {
		return nil, nil, err
	}

	return inboundConn, outboundConn, nil
}

func (c *ProxyRelayConfig) ForwardUDP(address1, address2 string, stopChan chan bool) error {
	lisener, targetAddr, err := initUDPConnections(address1, address2)
	if err != nil {
		return err
	}

	var buffer [1500]byte
	for {
		n, cliaddr, err := lisener.ReadFromUDP(buffer[0:])
		if err != nil {
			continue
		}
		c.aTobAccumulatedByteTransfer.Add(int64(n))
		saddr := cliaddr.String()
		dlock()
		conn, found := ClientDict[saddr]
		if !found {
			conn = createNewUDPConn(targetAddr, cliaddr)
			if conn == nil {
				dunlock()
				continue
			}
			ClientDict[saddr] = conn
			dunlock()
			Vlogf(2, "Created new connection for client %s\n", saddr)
			// Fire up routine to manage new connection
			go RunConnection(conn)
		} else {
			Vlogf(5, "Found connection for client %s\n", saddr)
			dunlock()
		}
		// Relay to server
		_, err = conn.ServerConn.Write(buffer[0:n])
		if checkreport(1, err) {
			continue
		}
	}
}
