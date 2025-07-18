package zoraxy_plugin

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

/*

	Dynamic Path Handler

*/

type SniffResult int

const (
	SniffResultAccept SniffResult = iota // Forward the request to this plugin dynamic capture ingress
	SniffResultSkip                      // Skip this plugin and let the next plugin handle the request
)

type SniffHandler func(*DynamicSniffForwardRequest) SniffResult

/*
RegisterDynamicSniffHandler registers a dynamic sniff handler for a path
You can decide to accept or skip the request based on the request header and paths
*/
func (p *PathRouter) RegisterDynamicSniffHandler(sniff_ingress string, mux *http.ServeMux, handler SniffHandler) {
	if !strings.HasSuffix(sniff_ingress, "/") {
		sniff_ingress = sniff_ingress + "/"
	}
	mux.Handle(sniff_ingress, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.enableDebugPrint {
			fmt.Println("Request captured by dynamic sniff path: " + r.RequestURI)
		}

		// Decode the request payload
		jsonBytes, err := io.ReadAll(r.Body)
		if err != nil {
			if p.enableDebugPrint {
				fmt.Println("Error reading request body:", err)
			}
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		payload, err := DecodeForwardRequestPayload(jsonBytes)
		if err != nil {
			if p.enableDebugPrint {
				fmt.Println("Error decoding request payload:", err)
				fmt.Print("Payload: ")
				fmt.Println(string(jsonBytes))
			}
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Get the forwarded request UUID
		forwardUUID := r.Header.Get("X-Zoraxy-RequestID")
		payload.requestUUID = forwardUUID
		payload.rawRequest = r

		sniffResult := handler(&payload)
		if sniffResult == SniffResultAccept {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("OK"))
		} else {
			w.WriteHeader(http.StatusNotImplemented)
			w.Write([]byte("SKIP"))
		}
	}))
}

// RegisterDynamicCaptureHandle register the dynamic capture ingress path with a handler
func (p *PathRouter) RegisterDynamicCaptureHandle(capture_ingress string, mux *http.ServeMux, handlefunc func(http.ResponseWriter, *http.Request)) {
	if !strings.HasSuffix(capture_ingress, "/") {
		capture_ingress = capture_ingress + "/"
	}
	mux.Handle(capture_ingress, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if p.enableDebugPrint {
			fmt.Println("Request captured by dynamic capture path: " + r.RequestURI)
		}

		rewrittenURL := r.RequestURI
		rewrittenURL = strings.TrimPrefix(rewrittenURL, capture_ingress)
		rewrittenURL = strings.ReplaceAll(rewrittenURL, "//", "/")
		if rewrittenURL == "" {
			rewrittenURL = "/"
		}
		if !strings.HasPrefix(rewrittenURL, "/") {
			rewrittenURL = "/" + rewrittenURL
		}
		r.RequestURI = rewrittenURL

		handlefunc(w, r)
	}))
}

/*
	Sniffing and forwarding

	The following functions are here to help with
	sniffing and forwarding requests to the dynamic
	router.
*/
// A custom request object to be used in the dynamic sniffing
type DynamicSniffForwardRequest struct {
	Method     string              `json:"method"`
	Hostname   string              `json:"hostname"`
	URL        string              `json:"url"`
	Header     map[string][]string `json:"header"`
	RemoteAddr string              `json:"remote_addr"`
	Host       string              `json:"host"`
	RequestURI string              `json:"request_uri"`
	Proto      string              `json:"proto"`
	ProtoMajor int                 `json:"proto_major"`
	ProtoMinor int                 `json:"proto_minor"`

	/* Internal use */
	rawRequest  *http.Request `json:"-"`
	requestUUID string        `json:"-"`
}

// GetForwardRequestPayload returns a DynamicSniffForwardRequest object from an http.Request object
func EncodeForwardRequestPayload(r *http.Request) DynamicSniffForwardRequest {
	return DynamicSniffForwardRequest{
		Method:     r.Method,
		Hostname:   r.Host,
		URL:        r.URL.String(),
		Header:     r.Header,
		RemoteAddr: r.RemoteAddr,
		Host:       r.Host,
		RequestURI: r.RequestURI,
		Proto:      r.Proto,
		ProtoMajor: r.ProtoMajor,
		ProtoMinor: r.ProtoMinor,
		rawRequest: r,
	}
}

// DecodeForwardRequestPayload decodes JSON bytes into a DynamicSniffForwardRequest object
func DecodeForwardRequestPayload(jsonBytes []byte) (DynamicSniffForwardRequest, error) {
	var payload DynamicSniffForwardRequest
	err := json.Unmarshal(jsonBytes, &payload)
	if err != nil {
		return DynamicSniffForwardRequest{}, err
	}
	return payload, nil
}

// GetRequest returns the original http.Request object, for debugging purposes
func (dsfr *DynamicSniffForwardRequest) GetRequest() *http.Request {
	return dsfr.rawRequest
}

// GetRequestUUID returns the request UUID
// if this UUID is empty string, that might indicate the request
// is not coming from the dynamic router
func (dsfr *DynamicSniffForwardRequest) GetRequestUUID() string {
	return dsfr.requestUUID
}
