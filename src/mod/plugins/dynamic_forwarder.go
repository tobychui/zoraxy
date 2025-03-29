package plugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
	"imuslab.com/zoraxy/mod/plugins/zoraxy_plugin"
)

// StartDynamicForwardRouter create and start a dynamic forward router for
// this plugin
func (p *Plugin) StartDynamicForwardRouter() error {
	// Create a new dpcore object to forward the traffic to the plugin
	targetURL, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(p.AssignedPort) + p.Spec.DynamicCaptureIngress)
	if err != nil {
		fmt.Println("Failed to parse target URL: "+targetURL.String(), err)
		return err
	}
	thisRouter := dpcore.NewDynamicProxyCore(targetURL, "", &dpcore.DpcoreOptions{})
	p.dynamicRouteProxy = thisRouter
	return nil
}

// StopDynamicForwardRouter stops the dynamic forward router for this plugin
func (p *Plugin) StopDynamicForwardRouter() {
	if p.dynamicRouteProxy != nil {
		p.dynamicRouteProxy = nil
	}
}

// AcceptDynamicRoute returns whether this plugin accepts dynamic route
func (p *Plugin) AcceptDynamicRoute() bool {
	return p.Spec.DynamicCaptureSniff != "" && p.Spec.DynamicCaptureIngress != ""
}

func (p *Plugin) HandleDynamicRoute(w http.ResponseWriter, r *http.Request) bool {
	//Make sure p.Spec.DynamicCaptureSniff and p.Spec.DynamicCaptureIngress are not empty and start with /
	if !p.AcceptDynamicRoute() {
		return false
	}

	//Make sure the paths start with / and do not end with /
	if !strings.HasPrefix(p.Spec.DynamicCaptureSniff, "/") {
		p.Spec.DynamicCaptureSniff = "/" + p.Spec.DynamicCaptureSniff
	}
	p.Spec.DynamicCaptureSniff = strings.TrimSuffix(p.Spec.DynamicCaptureSniff, "/")
	if !strings.HasPrefix(p.Spec.DynamicCaptureIngress, "/") {
		p.Spec.DynamicCaptureIngress = "/" + p.Spec.DynamicCaptureIngress
	}
	p.Spec.DynamicCaptureIngress = strings.TrimSuffix(p.Spec.DynamicCaptureIngress, "/")

	//Send the request to the sniff endpoint
	sniffURL, err := url.Parse("http://127.0.0.1:" + strconv.Itoa(p.AssignedPort) + p.Spec.DynamicCaptureSniff + "/")
	if err != nil {
		//Error when parsing the sniff URL, let the next plugin handle the request
		return false
	}

	// Create an instance of CustomRequest with the original request's data
	forwardReq := zoraxy_plugin.EncodeForwardRequestPayload(r)

	// Encode the custom request object into JSON
	jsonData, err := json.Marshal(forwardReq)
	if err != nil {
		// Error when encoding the request, let the next plugin handle the request
		return false
	}

	//Generate a unique request ID
	uniqueRequestID := uuid.New().String()

	req, err := http.NewRequest("POST", sniffURL.String(), bytes.NewBuffer(jsonData))
	if err != nil {
		// Error when creating the request, let the next plugin handle the request
		return false
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Zoraxy-RequestID", uniqueRequestID)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		// Error when sending the request, let the next plugin handle the request
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Sniff endpoint did not return OK, let the next plugin handle the request
		return false
	}

	p.dynamicRouteProxy.ServeHTTP(w, r, &dpcore.ResponseRewriteRuleSet{
		UseTLS:       false,
		OriginalHost: r.Host,
		ProxyDomain:  "127.0.0.1:" + strconv.Itoa(p.AssignedPort),
		NoCache:      true,
		PathPrefix:   p.Spec.DynamicCaptureIngress,
		UpstreamHeaders: [][]string{
			{"X-Zoraxy-RequestID", uniqueRequestID},
		},
	})
	return true
}
