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

func connCopy(conn1 net.Conn, conn2 net.Conn, wg *sync.WaitGroup, accumulator *atomic.Int64) {
	n, err := io.Copy(conn1, conn2)
	if err != nil {
		return
	}
	accumulator.Add(n) //Add to accumulator
	conn1.Close()
	log.Println("[←]", "close the connect at local:["+conn1.LocalAddr().String()+"] and remote:["+conn1.RemoteAddr().String()+"]")
	//conn2.Close()
	//log.Println("[←]", "close the connect at local:["+conn2.LocalAddr().String()+"] and remote:["+conn2.RemoteAddr().String()+"]")
	wg.Done()
}

func writeProxyProtocolHeaderV1(dst net.Conn, src net.Conn) error {
	clientAddr, ok1 := src.RemoteAddr().(*net.TCPAddr)
	proxyAddr, ok2 := src.LocalAddr().(*net.TCPAddr)
	if !ok1 || !ok2 {
		return errors.New("invalid TCP address for proxy protocol")
	}

	header := fmt.Sprintf("PROXY TCP4 %s %s %d %d\r\n",
		clientAddr.IP.String(),
		proxyAddr.IP.String(),
		clientAddr.Port,
		proxyAddr.Port)

	_, err := dst.Write([]byte(header))
	return err
}

func forward(conn1 net.Conn, conn2 net.Conn, aTob *atomic.Int64, bToa *atomic.Int64) {
	log.Printf("[+] start transmit. [%s],[%s] <-> [%s],[%s] \n", conn1.LocalAddr().String(), conn1.RemoteAddr().String(), conn2.LocalAddr().String(), conn2.RemoteAddr().String())
	var wg sync.WaitGroup
	// wait tow goroutines
	wg.Add(2)
	go connCopy(conn1, conn2, &wg, aTob)
	go connCopy(conn2, conn1, &wg, bToa)
	//blocking when the wg is locked
	wg.Wait()
}

func (c *ProxyRelayConfig) accept(listener net.Listener) (net.Conn, error) {
	conn, err := listener.Accept()
	if err != nil {
		return nil, err
	}

	//Check if connection in blacklist or whitelist
	if addr, ok := conn.RemoteAddr().(*net.TCPAddr); ok {
		if !c.parent.Options.AccessControlHandler(conn) {
			time.Sleep(300 * time.Millisecond)
			conn.Close()
			log.Println("[x]", "Connection from "+addr.IP.String()+" rejected by access control policy")
			return nil, errors.New("Connection from " + addr.IP.String() + " rejected by access control policy")
		}
	}

	log.Println("[√]", "accept a new client. remote address:["+conn.RemoteAddr().String()+"], local address:["+conn.LocalAddr().String()+"]")
	return conn, err
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
func (c *ProxyRelayConfig) Port2host(allowPort string, targetAddress string, stopChan chan bool) error {
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
		log.Println("[x]", "Received stop signal. Exiting Port to Host forwarder")
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
			log.Println("[+]", "start connect host:["+targetAddress+"]")
			target, err := net.Dial("tcp", targetAddress)
			if err != nil {
				// temporarily unavailable, don't use fatal.
				log.Println("[x]", "connect target address ["+targetAddress+"] faild. retry in ", c.Timeout, "seconds. ")
				conn.Close()
				log.Println("[←]", "close the connect at local:["+conn.LocalAddr().String()+"] and remote:["+conn.RemoteAddr().String()+"]")
				time.Sleep(time.Duration(c.Timeout) * time.Second)
				return
			}
			log.Println("[→]", "connect target address ["+targetAddress+"] success.")

			if c.UseProxyProtocol {
				log.Println("[+]", "write proxy protocol header to target address ["+targetAddress+"]")
				err = writeProxyProtocolHeaderV1(target, conn)
				if err != nil {
					log.Println("[x]", "Write proxy protocol header faild: ", err)
					target.Close()
					conn.Close()
					log.Println("[←]", "close the connect at local:["+conn.LocalAddr().String()+"] and remote:["+conn.RemoteAddr().String()+"]")
					time.Sleep(time.Duration(c.Timeout) * time.Second)
					return
				}
			}

			forward(target, conn, &c.aTobAccumulatedByteTransfer, &c.bToaAccumulatedByteTransfer)
		}(targetAddress)
	}
}
