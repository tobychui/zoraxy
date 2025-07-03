package streamproxy

/*
	Instances.go

	This file contains the methods to start, stop, and manage the proxy relay instances.

*/

import (
	"errors"
	"log"
	"time"
)

func (c *ProxyRelayInstance) LogMsg(message string, originalError error) {
	if !c.EnableLogging {
		return
	}

	if originalError != nil {
		log.Println(message, "error:", originalError)
	} else {
		log.Println(message)
	}
}

// Start a proxy if stopped
func (c *ProxyRelayInstance) Start() error {
	if c.IsRunning() {
		c.Running = true
		return errors.New("proxy already running")
	}

	// Create a stopChan to control the loop
	tcpStopChan := make(chan bool)
	udpStopChan := make(chan bool)

	//Start the proxy service
	if c.UseUDP {
		c.udpStopChan = udpStopChan
		go func() {
			err := c.ForwardUDP(c.ListeningAddress, c.ProxyTargetAddr, udpStopChan)
			if err != nil {
				if !c.UseTCP {
					c.Running = false
					c.udpStopChan = nil
					c.parent.SaveConfigToDatabase()
				}
				c.parent.logf("[proto:udp] Error starting stream proxy "+c.Name+"("+c.UUID+")", err)
			}
		}()
	}

	if c.UseTCP {
		c.tcpStopChan = tcpStopChan
		go func() {
			//Default to transport mode
			err := c.Port2host(c.ListeningAddress, c.ProxyTargetAddr, tcpStopChan)
			if err != nil {
				c.Running = false
				c.tcpStopChan = nil
				c.parent.SaveConfigToDatabase()
				c.parent.logf("[proto:tcp] Error starting stream proxy "+c.Name+"("+c.UUID+")", err)
			}
		}()
	}

	//Successfully spawned off the proxy routine
	c.Running = true
	c.parent.SaveConfigToDatabase()
	return nil
}

// Return if a proxy config is running
func (c *ProxyRelayInstance) IsRunning() bool {
	return c.tcpStopChan != nil || c.udpStopChan != nil
}

// Restart a proxy config
func (c *ProxyRelayInstance) Restart() {
	if c.IsRunning() {
		c.Stop()
	}
	time.Sleep(3000 * time.Millisecond)
	c.Start()
}

// Stop a running proxy if running
func (c *ProxyRelayInstance) Stop() {
	c.parent.logf("Stopping Stream Proxy "+c.Name, nil)

	if c.udpStopChan != nil {
		c.parent.logf("Stopping UDP for "+c.Name, nil)
		c.udpStopChan <- true
		c.udpStopChan = nil
	}

	if c.tcpStopChan != nil {
		c.parent.logf("Stopping TCP for "+c.Name, nil)
		c.tcpStopChan <- true
		c.tcpStopChan = nil
	}

	c.parent.logf("Stopped Stream Proxy "+c.Name, nil)
	c.Running = false

	//Update the running status
	c.parent.SaveConfigToDatabase()
}
