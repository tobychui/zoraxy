package streamproxy

import (
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	proxyproto "github.com/pires/go-proxyproto"
)

func isValidIP(ip string) bool {
	parsedIP := net.ParseIP(ip)
	return parsedIP != nil
}

func isValidPort(port string) bool {
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return false
	}

	if portInt < 1 || portInt > 65535 {
		return false
	}

	return true
}

func (c *ProxyRelayInstance) connCopy(conn1 net.Conn, conn2 net.Conn, wg *sync.WaitGroup, accumulator *atomic.Int64) {
	n, err := io.Copy(conn1, conn2)
	if err != nil {
		return
	}
	accumulator.Add(n) //Add to accumulator
	conn1.Close()
	c.LogMsg("[←] close the connect at local:["+conn1.LocalAddr().String()+"] and remote:["+conn1.RemoteAddr().String()+"]", nil)
	//conn2.Close()
	//c.LogMsg("[←] close the connect at local:["+conn2.LocalAddr().String()+"] and remote:["+conn2.RemoteAddr().String()+"]", nil)
	wg.Done()
}

func WriteProxyProtocolHeader(dst net.Conn, src net.Conn, version ProxyProtocolVersion) error {
	clientAddr, ok1 := src.RemoteAddr().(*net.TCPAddr)
	proxyAddr, ok2 := src.LocalAddr().(*net.TCPAddr)
	if !ok1 || !ok2 {
		return errors.New("invalid TCP address for proxy protocol")
	}

	header := proxyproto.Header{
		Version:           byte(convertProxyProtocolVersionToInt(version)),
		Command:           proxyproto.PROXY,
		TransportProtocol: proxyproto.TCPv4,
		SourceAddr:        clientAddr,
		DestinationAddr:   proxyAddr,
	}

	_, err := header.WriteTo(dst)
	return err
}

func (c *ProxyRelayInstance) forward(conn1 net.Conn, conn2 net.Conn, aTob *atomic.Int64, bToa *atomic.Int64) {
	msg := fmt.Sprintf("[+] start transmit. [%s],[%s] <-> [%s],[%s]",
		conn1.LocalAddr().String(), conn1.RemoteAddr().String(),
		conn2.LocalAddr().String(), conn2.RemoteAddr().String())
	c.LogMsg(msg, nil)

	var wg sync.WaitGroup
	wg.Add(2)
	go c.connCopy(conn1, conn2, &wg, aTob)
	go c.connCopy(conn2, conn1, &wg, bToa)
	wg.Wait()
}

func (c *ProxyRelayInstance) accept(listener net.Listener) (net.Conn, error) {
	conn, err := listener.Accept()
	if err != nil {
		return nil, err
	}

	// Check if connection in blacklist or whitelist
	if addr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		if !c.parent.Options.AccessControlHandler(conn) {
			time.Sleep(300 * time.Millisecond)
			conn.Close()
			c.LogMsg("[x] Connection from "+addr.IP.String()+" rejected by access control policy", nil)
			return nil, errors.New("Connection from " + addr.IP.String() + " rejected by access control policy")
		}
	}

	c.LogMsg("[√] accept a new client. remote address:["+conn.RemoteAddr().String()+"], local address:["+conn.LocalAddr().String()+"]", nil)
	return conn, nil
}

func startListener(address string) (net.Listener, error) {
	log.Println("[+]", "try to start server on:["+address+"]")
	server, err := net.Listen("tcp", address)
	if err != nil {
		return nil, errors.New("listen address [" + address + "] faild")
	}
	log.Println("[√]", "start listen at address:["+address+"]")
	return server, nil
}

/*
	Forwarder Functions
*/

/*
portA -> server
server -> portB
*/
func (c *ProxyRelayInstance) Port2host(allowPort string, targetAddress string, stopChan chan bool) error {
	listenerStartingAddr := allowPort
	if isValidPort(allowPort) {
		//number only, e.g. 8080
		listenerStartingAddr = "0.0.0.0:" + allowPort
	} else if strings.HasPrefix(allowPort, ":") && isValidPort(allowPort[1:]) {
		//port number starting with :, e.g. :8080
		listenerStartingAddr = "0.0.0.0" + allowPort
	}

	server, err := startListener(listenerStartingAddr)
	if err != nil {
		return err
	}

	targetAddress = strings.TrimSpace(targetAddress)

	//Start stop handler
	go func() {
		<-stopChan
		c.LogMsg("[x] Received stop signal. Exiting Port to Host forwarder", nil)
		server.Close()
	}()

	//Start blocking loop for accepting connections
	for {
		conn, err := c.accept(server)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				//Terminate by stop chan. Exit listener loop
				return nil
			}
			//Connection error. Retry
			continue
		}

		go func(targetAddress string) {
			c.LogMsg("[+] start connect host:["+targetAddress+"]", nil)
			target, err := net.Dial("tcp", targetAddress)
			if err != nil {
				// temporarily unavailable, don't use fatal.
				c.LogMsg("[x] connect target address ["+targetAddress+"] failed. retry in "+strconv.Itoa(c.Timeout)+" seconds.", nil)
				conn.Close()
				c.LogMsg("[←] close the connect at local:["+conn.LocalAddr().String()+"] and remote:["+conn.RemoteAddr().String()+"]", nil)
				time.Sleep(time.Duration(c.Timeout) * time.Second)
				return
			}
			c.LogMsg("[→] connect target address ["+targetAddress+"] success.", nil)

			if c.ProxyProtocolVersion != ProxyProtocolDisabled {
				c.LogMsg("[+] write proxy protocol header to target address ["+targetAddress+"]", nil)
				err = WriteProxyProtocolHeader(target, conn, c.ProxyProtocolVersion)
				if err != nil {
					c.LogMsg("[x] Write proxy protocol header failed: "+err.Error(), nil)
					target.Close()
					conn.Close()
					c.LogMsg("[←] close the connect at local:["+conn.LocalAddr().String()+"] and remote:["+conn.RemoteAddr().String()+"]", nil)
					time.Sleep(time.Duration(c.Timeout) * time.Second)
					return
				}
			}

			c.forward(target, conn, &c.aTobAccumulatedByteTransfer, &c.bToaAccumulatedByteTransfer)
		}(targetAddress)
	}
}
