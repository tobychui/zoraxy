package upnpc

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"gitlab.com/NebulousLabs/go-upnp"
)

/*
	uPNP Module

	This module handles uPNP Connections to the gateway router and create a port forward entry
	for the host system at the given port (set with -port paramter)
*/

type UPnPClient struct {
	Connection    *upnp.IGD //UPnP conenction object
	ExternalIP    string    //Storage of external IP address
	RequiredPorts []int     //All the required ports will be recored
	PolicyNames   sync.Map  //Name for the required port nubmer
}

// NewUPNPClient creates a new UPnPClient object
func NewUPNPClient() (*UPnPClient, error) {
	//Create uPNP forwarding in the NAT router
	fmt.Println("Discovering UPnP router in Local Area Network...")
	d, err := upnp.Discover()
	if err != nil {
		return &UPnPClient{}, err
	}

	// discover external IP
	ip, err := d.ExternalIP()
	if err != nil {
		return &UPnPClient{}, err
	}

	//Create the final obejcts
	newUPnPObject := &UPnPClient{
		Connection:    d,
		ExternalIP:    ip,
		RequiredPorts: []int{},
	}

	return newUPnPObject, nil
}

// ForwardPort forwards a port to the host
func (u *UPnPClient) ForwardPort(portNumber int, ruleName string) error {
	fmt.Println("UPnP forwarding new port: ", portNumber, "for "+ruleName+" service")

	//Check if port already forwarded
	_, ok := u.PolicyNames.Load(portNumber)
	if ok {
		//Port already forward. Ignore this request
		return errors.New("port already forwarded")
	}

	// forward a port
	err := u.Connection.Forward(uint16(portNumber), ruleName)
	if err != nil {
		return err
	}

	u.RequiredPorts = append(u.RequiredPorts, portNumber)
	u.PolicyNames.Store(portNumber, ruleName)
	return nil
}

// ClosePort closes the port forwarding
func (u *UPnPClient) ClosePort(portNumber int) error {
	//Check if port is opened
	portOpened := false
	newRequiredPort := []int{}
	for _, thisPort := range u.RequiredPorts {
		if thisPort != portNumber {
			newRequiredPort = append(newRequiredPort, thisPort)
		} else {
			portOpened = true
		}
	}

	if portOpened {
		//Update the port list
		u.RequiredPorts = newRequiredPort

		// Close the port
		fmt.Println("Closing UPnP Port Forward: ", portNumber)
		err := u.Connection.Clear(uint16(portNumber))

		//Delete the name registry
		u.PolicyNames.Delete(portNumber)

		if err != nil {
			fmt.Println(err)
			return err
		}
	}
	return nil
}

// Renew forward rules, prevent router lease time from flushing the Upnp config
func (u *UPnPClient) RenewForwardRules() {
	if u.Connection == nil {
		//UPnP router gone
		return
	}
	portsToRenew := u.RequiredPorts
	for _, thisPort := range portsToRenew {
		ruleName, ok := u.PolicyNames.Load(thisPort)
		if !ok {
			continue
		}
		u.ClosePort(thisPort)
		time.Sleep(100 * time.Millisecond)
		u.ForwardPort(thisPort, ruleName.(string))
	}
	fmt.Println("UPnP Port Forward rule renew completed")
}

func (u *UPnPClient) Close() error {
	//Shutdown the default UPnP Object
	if u != nil {
		for _, portNumber := range u.RequiredPorts {
			err := u.Connection.Clear(uint16(portNumber))
			if err != nil {
				return err
			}
		}
	}
	return nil
}
