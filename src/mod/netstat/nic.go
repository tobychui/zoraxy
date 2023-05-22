package netstat

import (
	"encoding/json"
	"net"
	"net/http"

	"imuslab.com/zoraxy/mod/utils"
)

type NetworkInterface struct {
	Name string
	ID   int
	IPs  []string
}

func HandleListNetworkInterfaces(w http.ResponseWriter, r *http.Request) {
	nic, err := ListNetworkInterfaces()
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	js, _ := json.Marshal(nic)
	utils.SendJSONResponse(w, string(js))
}

func ListNetworkInterfaces() ([]NetworkInterface, error) {
	var interfaces []NetworkInterface

	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		var ips []string
		addrs, err := iface.Addrs()
		if err != nil {
			return nil, err
		}

		for _, addr := range addrs {
			ips = append(ips, addr.String())
		}

		interfaces = append(interfaces, NetworkInterface{
			Name: iface.Name,
			ID:   iface.Index,
			IPs:  ips,
		})
	}

	return interfaces, nil
}
