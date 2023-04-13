package main

import (
	"encoding/json"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"imuslab.com/zoraxy/mod/upnp"
	"imuslab.com/zoraxy/mod/utils"
)

var upnpEnabled = false
var preforwardMap map[int]string

func initUpnp() error {
	go func() {
		//Let UPnP discovery run in background
		var err error
		upnpClient, err = upnp.NewUPNPClient()
		if err != nil {
			log.Println("UPnP router discover error: ", err.Error())
			return
		}

		if upnpEnabled {
			//Forward all the ports
			for port, policyName := range preforwardMap {
				upnpClient.ForwardPort(port, policyName)
				log.Println("Upnp forwarding ", port, " for "+policyName)
				time.Sleep(300 * time.Millisecond)
			}
		}
	}()

	//Check if the upnp was enabled
	sysdb.NewTable("upnp")
	sysdb.Read("upnp", "enabled", &upnpEnabled)

	//Load all the ports from database
	portsMap := map[int]string{}
	sysdb.Read("upnp", "portmap", &portsMap)
	preforwardMap = portsMap

	return nil
}

func handleUpnpDiscover(w http.ResponseWriter, r *http.Request) {
	restart, err := utils.PostPara(r, "restart")
	if err != nil {
		type UpnpInfo struct {
			ExternalIp string
			RouterIp   string
		}

		if upnpClient == nil {
			utils.SendErrorResponse(w, "No UPnP router discovered")
			return
		}

		parsedUrl, _ := url.Parse(upnpClient.Connection.Location())
		ipWithPort := parsedUrl.Host

		result := UpnpInfo{
			ExternalIp: upnpClient.ExternalIP,
			RouterIp:   ipWithPort,
		}

		//Show if there is a upnpclient
		js, _ := json.Marshal(result)
		utils.SendJSONResponse(w, string(js))
	} else {
		if restart == "true" {
			//Close the upnp client if exists
			if upnpClient != nil {
				saveForwardingPortsToDatabase()
				upnpClient.Close()
			}

			//Restart a new one
			initUpnp()

			utils.SendOK(w)
		}
	}
}

func handleToggleUPnP(w http.ResponseWriter, r *http.Request) {
	newMode, err := utils.PostPara(r, "mode")
	if err != nil {
		//Send the current mode to client side
		js, _ := json.Marshal(upnpEnabled)
		utils.SendJSONResponse(w, string(js))
	} else {
		if newMode == "true" {
			upnpEnabled = true
			sysdb.Read("upnp", "enabled", true)

			log.Println("UPnP Enabled. Forwarding all required ports")
			//Mount all Upnp requests from preforward Map
			for port, policyName := range preforwardMap {
				upnpClient.ForwardPort(port, policyName)
				log.Println("Upnp forwarding ", port, " for "+policyName)
				time.Sleep(300 * time.Millisecond)
			}

			utils.SendOK(w)
			return

		} else if newMode == "false" {
			upnpEnabled = false
			sysdb.Read("upnp", "enabled", false)
			log.Println("UPnP disabled. Closing all forwarded ports")
			//Save the current forwarded ports
			saveForwardingPortsToDatabase()

			//Unmount all Upnp request
			for _, port := range upnpClient.RequiredPorts {
				upnpClient.ClosePort(port)
				log.Println("UPnP port closed: ", port)
				time.Sleep(300 * time.Millisecond)
			}

			//done
			utils.SendOK(w)
			return
		}
	}
}

func filterRFC2141(input string) string {
	rfc2141 := regexp.MustCompile(`^[\w\-.!~*'()]*(\%[\da-fA-F]{2}[\w\-.!~*'()]*)*$`)
	var result []rune
	for _, char := range input {
		if char <= 127 && rfc2141.MatchString(string(char)) {
			result = append(result, char)
		}
	}
	return string(result)
}

func handleAddUpnpPort(w http.ResponseWriter, r *http.Request) {
	portString, err := utils.PostPara(r, "port")
	if err != nil {
		utils.SendErrorResponse(w, "invalid port given")
		return
	}

	portNumber, err := strconv.Atoi(portString)
	if err != nil {
		utils.SendErrorResponse(w, "invalid port given")
		return
	}

	policyName, err := utils.PostPara(r, "name")
	if err != nil {
		utils.SendErrorResponse(w, "invalid policy name")
		return
	}

	policyName = filterRFC2141(policyName)

	err = upnpClient.ForwardPort(portNumber, policyName)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	saveForwardingPortsToDatabase()

	utils.SendOK(w)
}

func handleRemoveUpnpPort(w http.ResponseWriter, r *http.Request) {
	portString, err := utils.PostPara(r, "port")
	if err != nil {
		utils.SendErrorResponse(w, "invalid port given")
		return
	}

	portNumber, err := strconv.Atoi(portString)
	if err != nil {
		utils.SendErrorResponse(w, "invalid port given")
		return
	}

	saveForwardingPortsToDatabase()

	upnpClient.ClosePort(portNumber)
}

func saveForwardingPortsToDatabase() {
	//Move the sync map to map[int]string
	m := make(map[int]string)
	upnpClient.PolicyNames.Range(func(key, value interface{}) bool {
		if k, ok := key.(int); ok {
			if v, ok := value.(string); ok {
				m[k] = v
			}
		}
		return true
	})

	preforwardMap = m
	sysdb.Write("upnp", "portmap", &preforwardMap)

}
