package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

/*
	API Handlers
*/

func handleUsableState(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		js, _ := json.Marshal(upnpRouterExists)
		SendJSONResponse(w, string(js))
	} else if r.Method == "POST" {
		//Try to probe the UPnP router again
		TryStartUPnPClient()
		if upnpRouterExists {
			SendOK(w)
		} else {
			SendErrorResponse(w, "UPnP router not found")
		}
	}
}

// Get or set the enable state of the plugin
func handleEnableState(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		js, _ := json.Marshal(upnpRuntimeConfig.Enabled)
		SendJSONResponse(w, string(js))
	} else if r.Method == "POST" {
		enable, err := PostBool(r, "enable")
		if err != nil {
			SendErrorResponse(w, err.Error())
			return
		}

		if !enable {
			//Close all the port forwards if UPnP client is available
			if upnpClient != nil {
				for _, record := range upnpRuntimeConfig.ForwardRules {
					err = upnpClient.ClosePort(record.PortNumber)
					if err != nil {
						SendErrorResponse(w, err.Error())
						return
					}
				}
			}
		} else {
			if upnpClient == nil {
				SendErrorResponse(w, "No UPnP router in network")
				return
			}

			//Forward all the ports if UPnP client is available
			if upnpClient != nil {
				for _, record := range upnpRuntimeConfig.ForwardRules {
					err = upnpClient.ForwardPort(record.PortNumber, record.RuleName)
					if err != nil {
						SendErrorResponse(w, err.Error())
						return
					}
				}
			}
		}

		upnpRuntimeConfig.Enabled = enable
		SaveRuntimeConfig()
	}
}

func handleForwardPortEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		port, err := PostInt(r, "port")
		if err != nil {
			SendErrorResponse(w, err.Error())
			return
		}

		oldPort, err := PostInt(r, "oldPort")
		if err != nil {
			SendErrorResponse(w, err.Error())
			return
		}

		name, err := PostPara(r, "name")
		if err != nil {
			SendErrorResponse(w, err.Error())
			return
		}

		if port < 1 || port > 65535 {
			SendErrorResponse(w, "invalid port number")
			return
		}

		//Check if the old port exists
		found := false
		for _, record := range upnpRuntimeConfig.ForwardRules {
			if record.PortNumber == oldPort {
				found = true
				break
			}
		}

		if !found {
			SendErrorResponse(w, "editing forward rule not found")
			return
		}

		//Delete the old port forward
		if oldPort != port && upnpClient != nil {
			//Remove the port forward if UPnP client is available
			err = upnpClient.ClosePort(oldPort)
			if err != nil {
				SendErrorResponse(w, err.Error())
				return
			}
		}

		//Remove from runtime config
		for i, record := range upnpRuntimeConfig.ForwardRules {
			if record.PortNumber == oldPort {
				upnpRuntimeConfig.ForwardRules = append(upnpRuntimeConfig.ForwardRules[:i], upnpRuntimeConfig.ForwardRules[i+1:]...)
				break
			}
		}

		//Create the new forward rule
		if upnpClient != nil {
			//Forward the port if UPnP client is available
			err = upnpClient.ForwardPort(port, name)
			if err != nil {
				SendErrorResponse(w, err.Error())
				return
			}
		}

		//Add to runtime config
		upnpRuntimeConfig.ForwardRules = append(upnpRuntimeConfig.ForwardRules, &PortForwardRecord{
			RuleName:   name,
			PortNumber: port,
		})

		//Save the runtime config
		SaveRuntimeConfig()
		SendOK(w)
	}
}

// Remove a port forward
func handleForwardPortRemove(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		port, err := PostInt(r, "port")
		if err != nil {
			SendErrorResponse(w, err.Error())
			return
		}

		if upnpClient != nil {
			//Remove the port forward if UPnP client is available
			err = upnpClient.ClosePort(port)
			if err != nil {
				SendErrorResponse(w, err.Error())
				return
			}
		}

		//Remove from runtime config
		for i, record := range upnpRuntimeConfig.ForwardRules {
			if record.PortNumber == port {
				upnpRuntimeConfig.ForwardRules = append(upnpRuntimeConfig.ForwardRules[:i], upnpRuntimeConfig.ForwardRules[i+1:]...)
				break
			}
		}

		//Save the runtime config
		SaveRuntimeConfig()
		SendOK(w)
	}
}

// Handle the port forward operations
func handleForwardPort(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		// List all the forwarded ports
		js, _ := json.Marshal(upnpRuntimeConfig.ForwardRules)
		SendJSONResponse(w, string(js))
	} else if r.Method == "POST" {
		//Add a new port forward
		port, err := PostInt(r, "port")
		if err != nil {
			SendErrorResponse(w, err.Error())
			return
		}

		name, err := PostPara(r, "name")
		if err != nil {
			SendErrorResponse(w, err.Error())
			return
		}

		if port < 1 || port > 65535 {
			SendErrorResponse(w, "invalid port number")
			return
		}

		if upnpClient != nil {
			//Forward the port if UPnP client is available
			err = upnpClient.ForwardPort(port, name)
			if err != nil {
				SendErrorResponse(w, err.Error())
				return
			}
		}

		//Add to runtime config
		upnpRuntimeConfig.ForwardRules = append(upnpRuntimeConfig.ForwardRules, &PortForwardRecord{
			RuleName:   name,
			PortNumber: port,
		})

		//Save the runtime config
		SaveRuntimeConfig()
		SendOK(w)
	}
}

/*
	Network Utilities
*/

// Send JSON response, with an extra json header
func SendJSONResponse(w http.ResponseWriter, json string) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(json))
}

func SendErrorResponse(w http.ResponseWriter, errMsg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{\"error\":\"" + errMsg + "\"}"))
}

func SendOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("\"OK\""))
}

// Get GET parameter
func GetPara(r *http.Request, key string) (string, error) {
	// Get first value from the URL query
	value := r.URL.Query().Get(key)
	if len(value) == 0 {
		return "", errors.New("invalid " + key + " given")
	}
	return value, nil
}

// Get GET paramter as boolean, accept 1 or true
func GetBool(r *http.Request, key string) (bool, error) {
	x, err := GetPara(r, key)
	if err != nil {
		return false, err
	}

	// Convert to lowercase and trim spaces just once to compare
	switch strings.ToLower(strings.TrimSpace(x)) {
	case "1", "true", "on":
		return true, nil
	case "0", "false", "off":
		return false, nil
	}

	return false, errors.New("invalid boolean given")
}

// Get POST parameter
func PostPara(r *http.Request, key string) (string, error) {
	// Try to parse the form
	if err := r.ParseForm(); err != nil {
		return "", err
	}
	// Get first value from the form
	x := r.Form.Get(key)
	if len(x) == 0 {
		return "", errors.New("invalid " + key + " given")
	}
	return x, nil
}

// Get POST paramter as boolean, accept 1 or true
func PostBool(r *http.Request, key string) (bool, error) {
	x, err := PostPara(r, key)
	if err != nil {
		return false, err
	}

	// Convert to lowercase and trim spaces just once to compare
	switch strings.ToLower(strings.TrimSpace(x)) {
	case "1", "true", "on":
		return true, nil
	case "0", "false", "off":
		return false, nil
	}

	return false, errors.New("invalid boolean given")
}

// Get POST paramter as int
func PostInt(r *http.Request, key string) (int, error) {
	x, err := PostPara(r, key)
	if err != nil {
		return 0, err
	}

	x = strings.TrimSpace(x)
	rx, err := strconv.Atoi(x)
	if err != nil {
		return 0, err
	}

	return rx, nil
}
