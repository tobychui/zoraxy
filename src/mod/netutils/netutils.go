package netutils

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

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

func HandlePing(w http.ResponseWriter, r *http.Request) {
	targetIpOrDomain, err := utils.GetPara(r, "target")
	if err != nil {
		utils.SendErrorResponse(w, "invalid target (domain or ip) address given")
		return
	}

	results := []string{}
	for i := 0; i < 4; i++ {
		realIP, pingTime, ttl, err := PingIP(targetIpOrDomain)
		if err != nil {
			results = append(results, "Reply from "+realIP+": "+err.Error())
		} else {
			results = append(results, fmt.Sprintf("Reply from %s: Time=%dms TTL=%d", realIP, pingTime.Milliseconds(), ttl))
		}
	}

	js, _ := json.Marshal(results)
	utils.SendJSONResponse(w, string(js))

}
