package acmewizard

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	ACME Wizard

	This wizard help validate the acme settings and configurations
*/

func HandleGuidedStepCheck(w http.ResponseWriter, r *http.Request) {
	stepNoStr, err := utils.GetPara(r, "step")
	if err != nil {
		utils.SendErrorResponse(w, "invalid step number given")
		return
	}

	stepNo, err := strconv.Atoi(stepNoStr)
	if err != nil {
		utils.SendErrorResponse(w, "invalid step number given")
		return
	}

	if stepNo == 1 {
		isListening, err := isLocalhostListening()
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		js, _ := json.Marshal(isListening)
		utils.SendJSONResponse(w, string(js))
	} else if stepNo == 2 {
		publicIp, err := getPublicIPAddress()
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		publicIp = strings.TrimSpace(publicIp)

		httpServerReachable := isHTTPServerAvailable(publicIp)

		js, _ := json.Marshal(httpServerReachable)
		utils.SendJSONResponse(w, string(js))
	} else if stepNo == 3 {
		domain, err := utils.GetPara(r, "domain")
		if err != nil {
			utils.SendErrorResponse(w, "domain cannot be empty")
			return
		}

		domain = strings.TrimSpace(domain)

		//Check if the domain is reachable
		reachable := isDomainReachable(domain)
		if !reachable {
			utils.SendErrorResponse(w, "domain is not reachable")
			return
		}

		//Check http is setup correctly
		httpServerReachable := isHTTPServerAvailable(domain)
		js, _ := json.Marshal(httpServerReachable)
		utils.SendJSONResponse(w, string(js))
	} else if stepNo == 10 {
		//Resolve public Ip address for tour
		publicIp, err := getPublicIPAddress()
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
		js, _ := json.Marshal(publicIp)
		utils.SendJSONResponse(w, string(js))
	} else {
		utils.SendErrorResponse(w, "invalid step number")
	}
}

// Step 1
func isLocalhostListening() (isListening bool, err error) {
	timeout := 2 * time.Second
	isListening = false
	// Check if localhost is listening on port 80 (HTTP)
	conn, err := net.DialTimeout("tcp", "localhost:80", timeout)
	if err == nil {
		isListening = true
		conn.Close()
	}

	// Check if localhost is listening on port 443 (HTTPS)
	conn, err = net.DialTimeout("tcp", "localhost:443", timeout)
	if err == nil {
		isListening = true
		conn.Close()
	}

	if isListening {
		return true, nil
	}

	return isListening, err
}

// Step 2
func getPublicIPAddress() (string, error) {
	resp, err := http.Get("http://checkip.amazonaws.com/")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	ip, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(ip), nil
}

func isHTTPServerAvailable(ipAddress string) bool {
	client := http.Client{
		Timeout: 5 * time.Second, // Timeout for the HTTP request
	}

	urls := []string{
		"http://" + ipAddress + ":80",
		"https://" + ipAddress + ":443",
	}

	for _, url := range urls {
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			fmt.Println(err, url)
			continue // Ignore invalid URLs
		}

		// Disable TLS verification to handle invalid certificates
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}

		resp, err := client.Do(req)
		if err == nil {
			resp.Body.Close()
			return true // HTTP server is available
		}
	}

	return false // HTTP server is not available
}

// Step 3
func isDomainReachable(domain string) bool {
	_, err := net.LookupHost(domain)
	if err != nil {
		return false // Domain is not reachable
	}
	return true // Domain is reachable
}
