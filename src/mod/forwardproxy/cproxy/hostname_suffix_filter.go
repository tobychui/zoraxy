package cproxy

import (
	"net/http"
	"strings"
)

type hostnameSuffixFilter struct {
	authorized []string
}

func NewHostnameSuffixFilter(authorized []string) Filter {
	return &hostnameSuffixFilter{authorized: authorized}
}

func (this hostnameSuffixFilter) IsAuthorized(_ http.ResponseWriter, request *http.Request) bool {
	host := request.URL.Host

	for _, authorized := range this.authorized {
		if strings.HasSuffix(host, authorized) {
			return true
		}
	}

	return false
}
