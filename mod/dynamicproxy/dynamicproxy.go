package dynamicproxy

import (
	"context"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"imuslab.com/arozos/ReverseProxy/mod/dynamicproxy/dpcore"
	"imuslab.com/arozos/ReverseProxy/mod/reverseproxy"
)

/*
	Allow users to setup manual proxying for specific path

*/
type Router struct {
	ListenPort        int
	ProxyEndpoints    *sync.Map
	SubdomainEndpoint *sync.Map
	Running           bool
	Root              *ProxyEndpoint
	mux               http.Handler
	useTLS            bool
	server            *http.Server
}

type RouterOption struct {
	Port int
}

type ProxyEndpoint struct {
	Root       string
	Domain     string
	RequireTLS bool
	Proxy      *dpcore.ReverseProxy `json:"-"`
}

type SubdomainEndpoint struct {
	MatchingDomain string
	Domain         string
	RequireTLS     bool
	Proxy          *reverseproxy.ReverseProxy `json:"-"`
}

type ProxyHandler struct {
	Parent *Router
}

func NewDynamicProxy(port int) (*Router, error) {
	proxyMap := sync.Map{}
	domainMap := sync.Map{}
	thisRouter := Router{
		ListenPort:        port,
		ProxyEndpoints:    &proxyMap,
		SubdomainEndpoint: &domainMap,
		Running:           false,
		useTLS:            false,
		server:            nil,
	}

	thisRouter.mux = &ProxyHandler{
		Parent: &thisRouter,
	}

	return &thisRouter, nil
}

//Start the dynamic routing
func (router *Router) StartProxyService() error {
	//Create a new server object
	if router.server != nil {
		return errors.New("Reverse proxy server already running")
	}

	if router.Root == nil {
		return errors.New("Reverse proxy router root not set")
	}

	router.server = &http.Server{Addr: ":" + strconv.Itoa(router.ListenPort), Handler: router.mux}
	router.Running = true
	go func() {
		err := router.server.ListenAndServe()
		log.Println("[DynamicProxy] " + err.Error())
	}()

	return nil
}

func (router *Router) StopProxyService() error {
	if router.server == nil {
		return errors.New("Reverse proxy server already stopped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := router.server.Shutdown(ctx)
	if err != nil {
		return err
	}

	//Discard the server object
	router.server = nil
	router.Running = false
	return nil
}

/*
	Add an URL into a custom proxy services
*/
func (router *Router) AddVirtualDirectoryProxyService(rootname string, domain string, requireTLS bool) error {
	if domain[len(domain)-1:] == "/" {
		domain = domain[:len(domain)-1]
	}

	if rootname[len(rootname)-1:] == "/" {
		rootname = rootname[:len(rootname)-1]
	}

	webProxyEndpoint := domain
	if requireTLS {
		webProxyEndpoint = "https://" + webProxyEndpoint
	} else {
		webProxyEndpoint = "http://" + webProxyEndpoint
	}
	//Create a new proxy agent for this root
	path, err := url.Parse(webProxyEndpoint)
	if err != nil {
		return err
	}

	proxy := dpcore.NewDynamicProxyCore(path, rootname)

	endpointObject := ProxyEndpoint{
		Root:       rootname,
		Domain:     domain,
		RequireTLS: requireTLS,
		Proxy:      proxy,
	}

	router.ProxyEndpoints.Store(rootname, &endpointObject)

	log.Println("Adding Proxy Rule: ", rootname+" to "+domain)
	return nil
}

/*
	Remove routing from RP

*/
func (router *Router) RemoveProxy(ptype string, key string) error {
	if ptype == "vdir" {
		router.ProxyEndpoints.Delete(key)
		return nil
	} else if ptype == "subd" {
		router.SubdomainEndpoint.Delete(key)
		return nil
	}
	return errors.New("invalid ptype")
}

/*
	Add an default router for the proxy server
*/
func (router *Router) SetRootProxy(proxyLocation string, requireTLS bool) error {
	if proxyLocation[len(proxyLocation)-1:] == "/" {
		proxyLocation = proxyLocation[:len(proxyLocation)-1]
	}

	webProxyEndpoint := proxyLocation
	if requireTLS {
		webProxyEndpoint = "https://" + webProxyEndpoint
	} else {
		webProxyEndpoint = "http://" + webProxyEndpoint
	}
	//Create a new proxy agent for this root
	path, err := url.Parse(webProxyEndpoint)
	if err != nil {
		return err
	}

	proxy := dpcore.NewDynamicProxyCore(path, "")

	rootEndpoint := ProxyEndpoint{
		Root:       "/",
		Domain:     proxyLocation,
		RequireTLS: requireTLS,
		Proxy:      proxy,
	}

	router.Root = &rootEndpoint
	return nil
}

//Do all the main routing in here
func (h *ProxyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(r.Host, ".") {
		//This might be a subdomain. See if there are any subdomain proxy router for this
		sep := h.Parent.getSubdomainProxyEndpointFromHostname(r.Host)
		if sep != nil {
			h.subdomainRequest(w, r, sep)
			return
		}
	}

	targetProxyEndpoint := h.Parent.getTargetProxyEndpointFromRequestURI(r.RequestURI)
	if targetProxyEndpoint != nil {
		h.proxyRequest(w, r, targetProxyEndpoint)
	} else {
		h.proxyRequest(w, r, h.Parent.Root)
	}
}
