package netutils

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"github.com/likexian/whois"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	This script handles basic network utilities like
	- traceroute
	- ping
*/

func HandleTraceRoute(w http.ResponseWriter, r *http.Request) {
	targetIpOrDomain, err := utils.GetPara(r, "target")
	if err != nil {
		utils.SendErrorResponse(w, "invalid target (domain or ip) address given")
		return
	}

	maxhopsString, err := utils.GetPara(r, "maxhops")
	if err != nil {
		maxhopsString = "64"
	}

	maxHops, err := strconv.Atoi(maxhopsString)
	if err != nil {
		maxHops = 64
	}

	results, err := TraceRoute(targetIpOrDomain, maxHops)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	js, _ := json.Marshal(results)
	utils.SendJSONResponse(w, string(js))
}

func TraceRoute(targetIpOrDomain string, maxHops int) ([]string, error) {
	return traceroute(targetIpOrDomain, maxHops)
}

func HandleWhois(w http.ResponseWriter, r *http.Request) {
	targetIpOrDomain, err := utils.GetPara(r, "target")
	if err != nil {
		utils.SendErrorResponse(w, "invalid target (domain or ip) address given")
		return
	}

	raw, _ := utils.GetPara(r, "raw")

	result, err := whois.Whois(targetIpOrDomain)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	if raw == "true" {
		utils.SendTextResponse(w, result)
		return
	}
	if isDomainName(targetIpOrDomain) {
		//Is Domain
		parsedOutput, err := ParseWHOISResponse(result)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		js, _ := json.Marshal(parsedOutput)
		utils.SendJSONResponse(w, string(js))
	} else {
		//Is IP
		parsedOutput, err := ParseWhoisIpData(result)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}

		js, _ := json.Marshal(parsedOutput)
		utils.SendJSONResponse(w, string(js))
	}
}

func HandlePing(w http.ResponseWriter, r *http.Request) {
	targetIpOrDomain, err := utils.GetPara(r, "target")
	if err != nil {
		utils.SendErrorResponse(w, "invalid target (domain or ip) address given")
		return
	}

	type MixedPingResults struct {
		ICMP []string
		TCP  []string
		UDP  []string
	}

	results := MixedPingResults{
		ICMP: []string{},
		TCP:  []string{},
		UDP:  []string{},
	}

	//Ping ICMP
	for i := 0; i < 4; i++ {
		realIP, pingTime, ttl, err := PingIP(targetIpOrDomain)
		if err != nil {
			results.ICMP = append(results.ICMP, "Reply from "+realIP+": "+err.Error())
		} else {
			results.ICMP = append(results.ICMP, fmt.Sprintf("Reply from %s: Time=%dms TTL=%d", realIP, pingTime.Milliseconds(), ttl))
		}
	}

	//Ping TCP
	for i := 0; i < 4; i++ {
		pingTime, err := TCPPing(targetIpOrDomain)
		if err != nil {
			results.TCP = append(results.TCP, "Reply from "+resolveIpFromDomain(targetIpOrDomain)+": "+err.Error())
		} else {
			results.TCP = append(results.TCP, fmt.Sprintf("Reply from %s: Time=%dms", resolveIpFromDomain(targetIpOrDomain), pingTime.Milliseconds()))
		}
	}
	//Ping UDP
	for i := 0; i < 4; i++ {
		pingTime, err := UDPPing(targetIpOrDomain)
		if err != nil {
			results.UDP = append(results.UDP, "Reply from "+resolveIpFromDomain(targetIpOrDomain)+": "+err.Error())
		} else {
			results.UDP = append(results.UDP, fmt.Sprintf("Reply from %s: Time=%dms", resolveIpFromDomain(targetIpOrDomain), pingTime.Milliseconds()))
		}
	}

	js, _ := json.Marshal(results)
	utils.SendJSONResponse(w, string(js))

}

func resolveIpFromDomain(targetIpOrDomain string) string {
	//Resolve target ip address
	targetIpAddrString := ""
	ipAddr, err := net.ResolveIPAddr("ip", targetIpOrDomain)
	if err != nil {
		targetIpAddrString = targetIpOrDomain
	} else {
		targetIpAddrString = ipAddr.IP.String()
	}

	return targetIpAddrString
}
