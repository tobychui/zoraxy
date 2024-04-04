package cproxy

import "net/http"

type defaultHandler struct {
	filter          Filter
	clientConnector clientConnector
	serverConnector serverConnector
	meter           monitor
}

func newHandler(filter Filter, clientConnector clientConnector, serverConnector serverConnector, meter monitor) *defaultHandler {
	return &defaultHandler{
		filter:          filter,
		clientConnector: clientConnector,
		serverConnector: serverConnector,
		meter:           meter,
	}
}

func (this *defaultHandler) ServeHTTP(response http.ResponseWriter, request *http.Request) {
	this.meter.Measure(MeasurementHTTPRequest)

	if request.Method != "CONNECT" {
		this.meter.Measure(MeasurementBadMethod)
		writeResponseStatus(response, http.StatusMethodNotAllowed)

	} else if !this.filter.IsAuthorized(response, request) {
		this.meter.Measure(MeasurementUnauthorizedRequest)
		//writeResponseStatus(response, http.StatusUnauthorized)

	} else if client := this.clientConnector.Connect(response); client == nil {
		this.meter.Measure(MeasurementClientConnectionFailed)
		writeResponseStatus(response, http.StatusNotImplemented)

	} else if connection := this.serverConnector.Connect(client, request.URL.Host); connection == nil {
		this.meter.Measure(MeasurementServerConnectionFailed)
		_, _ = client.Write(statusBadGateway)
		_ = client.Close()

	} else {
		this.meter.Measure(MeasurementProxyReady)
		_, _ = client.Write(statusReady)
		connection.Proxy()
		this.meter.Measure(MeasurementProxyComplete)
	}
}

func writeResponseStatus(response http.ResponseWriter, statusCode int) {
	http.Error(response, http.StatusText(statusCode), statusCode)
}

var (
	statusBadGateway = []byte("HTTP/1.1 502 Bad Gateway\r\n\r\n")
	statusReady      = []byte("HTTP/1.1 200 OK\r\n\r\n")
)
