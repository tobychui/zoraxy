package tcpprox

import (
	"errors"
	"io"
	"log"
	"net"
	"strconv"
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

func isReachable(target string) bool {
	timeout := time.Duration(2 * time.Second) // Set the timeout value as per your requirement
	conn, err := net.DialTimeout("tcp", target, timeout)
	if err != nil {
		return false
	}
	defer conn.Close()
	return true
}

func connCopy(conn1 net.Conn, conn2 net.Conn, wg *sync.WaitGroup, accumulator *atomic.Int64) {
	n, err := io.Copy(conn1, conn2)
	if err != nil {
		//Add to accumulator
		accumulator.Add(n)
	}
	conn1.Close()
	log.Println("[←]", "close the connect at local:["+conn1.LocalAddr().String()+"] and remote:["+conn1.RemoteAddr().String()+"]")
	//conn2.Close()
	//log.Println("[←]", "close the connect at local:["+conn2.LocalAddr().String()+"] and remote:["+conn2.RemoteAddr().String()+"]")
	wg.Done()
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
	Config Functions
*/

// Config validator
func (c *ProxyRelayConfig) ValidateConfigs() error {
	if c.Mode == ProxyMode_Transport {
		//Port2Host: PortA int, PortB string
		if !isValidPort(c.PortA) {
			return errors.New("first address must be a valid port number")
		}

		if !isReachable(c.PortB) {
			return errors.New("second address is unreachable")
		}
		return nil

	} else if c.Mode == ProxyMode_Listen {
		//Port2Port: Both port are port number
		if !isValidPort(c.PortA) {
			return errors.New("first address is not a valid port number")
		}

		if !isValidPort(c.PortB) {
			return errors.New("second address is not a valid port number")
		}

		return nil
	} else if c.Mode == ProxyMode_Starter {
		//Host2Host: Both have to be hosts
		if !isReachable(c.PortA) {
			return errors.New("first address is unreachable")
		}

		if !isReachable(c.PortB) {
			return errors.New("second address is unreachable")
		}

		return nil
	} else {
		return errors.New("invalid mode given")
	}
}

// Start a proxy if stopped
func (c *ProxyRelayConfig) Start() error {
	if c.Running {
		return errors.New("proxy already running")
	}

	// Create a stopChan to control the loop
	stopChan := make(chan bool)
	c.stopChan = stopChan

	//Validate configs
	err := c.ValidateConfigs()
	if err != nil {
		return err
	}

	//Start the proxy service
	go func() {
		c.Running = true
		if c.Mode == ProxyMode_Transport {
			err = c.Port2host(c.PortA, c.PortB, stopChan)
		} else if c.Mode == ProxyMode_Listen {
			err = c.Port2port(c.PortA, c.PortB, stopChan)
		} else if c.Mode == ProxyMode_Starter {
			err = c.Host2host(c.PortA, c.PortB, stopChan)
		} else if c.Mode == ProxyMode_UDP {
			err = c.ForwardUDP(c.PortA, c.PortB, stopChan)
		}
		if err != nil {
			c.Running = false
			log.Println("Error starting proxy service " + c.Name + "(" + c.UUID + "): " + err.Error())
		}
	}()

	//Successfully spawned off the proxy routine
	return nil
}

// Stop a running proxy if running
func (c *ProxyRelayConfig) IsRunning() bool {
	return c.Running || c.stopChan != nil
}

// Stop a running proxy if running
func (c *ProxyRelayConfig) Stop() {
	if c.Running || c.stopChan != nil {
		c.stopChan <- true
		time.Sleep(300 * time.Millisecond)
		c.stopChan = nil
		c.Running = false
	}
}

/*
	Forwarder Functions
*/

/*
portA -> server
portB -> server
*/
func (c *ProxyRelayConfig) Port2port(port1 string, port2 string, stopChan chan bool) error {
	//Trim the Prefix of : if exists
	listen1, err := startListener("0.0.0.0:" + port1)
	if err != nil {
		return err
	}
	listen2, err := startListener("0.0.0.0:" + port2)
	if err != nil {
		return err
	}

	log.Println("[√]", "listen port:", port1, "and", port2, "success. waiting for client...")
	c.Running = true

	go func() {
		<-stopChan
		log.Println("[x]", "Received stop signal. Exiting Port to Port forwarder")
		c.Running = false
		listen1.Close()
		listen2.Close()
	}()

	for {
		conn1, err := c.accept(listen1)
		if err != nil {
			if !c.Running {
				return nil
			}
			continue
		}

		conn2, err := c.accept(listen2)
		if err != nil {
			if !c.Running {
				return nil
			}
			continue
		}

		if conn1 == nil || conn2 == nil {
			log.Println("[x]", "accept client faild. retry in ", c.Timeout, " seconds. ")
			time.Sleep(time.Duration(c.Timeout) * time.Second)
			continue
		}
		go forward(conn1, conn2, &c.aTobAccumulatedByteTransfer, &c.bToaAccumulatedByteTransfer)
	}
}

/*
portA -> server
server -> portB
*/
func (c *ProxyRelayConfig) Port2host(allowPort string, targetAddress string, stopChan chan bool) error {
	server, err := startListener("0.0.0.0:" + allowPort)
	if err != nil {
		return err
	}

	//Start stop handler
	go func() {
		<-stopChan
		log.Println("[x]", "Received stop signal. Exiting Port to Host forwarder")
		c.Running = false
		server.Close()
	}()

	//Start blocking loop for accepting connections
	for {
		conn, err := c.accept(server)
		if conn == nil || err != nil {
			if !c.Running {
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
			forward(target, conn, &c.aTobAccumulatedByteTransfer, &c.bToaAccumulatedByteTransfer)
		}(targetAddress)
	}
}

/*
server -> portA
server -> portB
*/
func (c *ProxyRelayConfig) Host2host(address1, address2 string, stopChan chan bool) error {
	c.Running = true
	go func() {
		<-stopChan
		log.Println("[x]", "Received stop signal. Exiting Host to Host forwarder")
		c.Running = false
	}()

	for c.Running {
		log.Println("[+]", "try to connect host:["+address1+"] and ["+address2+"]")
		var host1, host2 net.Conn
		var err error
		for {
			d := net.Dialer{Timeout: time.Duration(c.Timeout)}
			host1, err = d.Dial("tcp", address1)
			if err == nil {
				log.Println("[→]", "connect ["+address1+"] success.")
				break
			} else {
				log.Println("[x]", "connect target address ["+address1+"] faild. retry in ", c.Timeout, " seconds. ")
				time.Sleep(time.Duration(c.Timeout) * time.Second)
			}

			if !c.Running {
				return nil
			}
		}
		for {
			d := net.Dialer{Timeout: time.Duration(c.Timeout)}
			host2, err = d.Dial("tcp", address2)
			if err == nil {
				log.Println("[→]", "connect ["+address2+"] success.")
				break
			} else {
				log.Println("[x]", "connect target address ["+address2+"] faild. retry in ", c.Timeout, " seconds. ")
				time.Sleep(time.Duration(c.Timeout) * time.Second)
			}

			if !c.Running {
				return nil
			}
		}
		go forward(host1, host2, &c.aTobAccumulatedByteTransfer, &c.bToaAccumulatedByteTransfer)
	}

	return nil
}
