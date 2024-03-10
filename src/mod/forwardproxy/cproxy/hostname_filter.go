package cproxy

import "net/http"

type hostnameFilter struct {
	authorized []string
}

func NewHostnameFilter(authorized []string) Filter {
	return &hostnameFilter{authorized: authorized}
}

func (this hostnameFilter) IsAuthorized(_ http.ResponseWriter, request *http.Request) bool {
	if len(this.authorized) == 0 {
		return true
	}

	host := request.URL.Host
	hostLength := len(host)
	for _, authorized := range this.authorized {
		if authorized[:2] == "*." {
			have, want := hostLength, len(authorized)-1
			if have > want && authorized[1:] == host[hostLength-want:] {
				return true
			}
		} else if authorized == host {
			return true
		}
	}

	return false
}
