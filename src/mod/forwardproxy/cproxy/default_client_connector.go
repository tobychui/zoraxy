package cproxy

import "net/http"

type defaultClientConnector struct{}

func newClientConnector() *defaultClientConnector {
	return &defaultClientConnector{}
}

func (this *defaultClientConnector) Connect(response http.ResponseWriter) Socket {
	if hijacker, ok := response.(http.Hijacker); !ok {
		return nil
	} else if socket, _, _ := hijacker.Hijack(); socket == nil {
		return nil // this 'else if' exists to avoid the pointer nil != interface nil issue
	} else {
		return socket
	}
}
