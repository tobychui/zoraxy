package domainsniff

import "net/http"

/*
	Promox API sniffer

	This handler sniff proxmox API endpoint and
	adjust the request accordingly to fix shits
	in the proxmox API server
*/

func IsProxmox(r *http.Request) bool {
	// Check if any of the cookies is named PVEAuthCookie
	for _, cookie := range r.Cookies() {
		if cookie.Name == "PVEAuthCookie" {
			return true
		}
	}
	return false
}
