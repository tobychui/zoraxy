package dynamicproxy

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
	"imuslab.com/zoraxy/mod/dynamicproxy/redirection"
	"imuslab.com/zoraxy/mod/geodb"
	"imuslab.com/zoraxy/mod/reverseproxy"
	"imuslab.com/zoraxy/mod/statistic"
	"imuslab.com/zoraxy/mod/tlscert"
)

/*
	Zoraxy Dynamic Proxy
*/
type RouterOption struct {
	Port               int
	UseTls             bool
	ForceHttpsRedirect bool
	TlsManager         *tlscert.Manager
	RedirectRuleTable  *redirection.RuleTable
	GeodbStore         *geodb.Store
	StatisticCollector *statistic.Collector
}

type Router struct {
	Option            *RouterOption
	ProxyEndpoints    *sync.Map
	SubdomainEndpoint *sync.Map
	Running           bool
	Root              *ProxyEndpoint
	mux               http.Handler
	server            *http.Server
	tlsListener       net.Listener
	routingRules      []*RoutingRule
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

func NewDynamicProxy(option RouterOption) (*Router, error) {
	proxyMap := sync.Map{}
	domainMap := sync.Map{}
	thisRouter := Router{
		Option:            &option,
		ProxyEndpoints:    &proxyMap,
		SubdomainEndpoint: &domainMap,
		Running:           false,
		server:            nil,
		routingRules:      []*RoutingRule{},
	}

	thisRouter.mux = &ProxyHandler{
		Parent: &thisRouter,
	}

	return &thisRouter, nil
}

// Update TLS setting in runtime. Will restart the proxy server
// if it is already running in the background
func (router *Router) UpdateTLSSetting(tlsEnabled bool) {
	router.Option.UseTls = tlsEnabled
	router.Restart()
}

// Update https redirect, which will require updates
func (router *Router) UpdateHttpToHttpsRedirectSetting(useRedirect bool) {
	router.Option.ForceHttpsRedirect = useRedirect
	router.Restart()
}

// Start the dynamic routing
func (router *Router) StartProxyService() error {
	//Create a new server object
	if router.server != nil {
		return errors.New("Reverse proxy server already running")
	}

	if router.Root == nil {
		return errors.New("Reverse proxy router root not set")
	}

	config := &tls.Config{
		GetCertificate: router.Option.TlsManager.GetCert,
	}

	if router.Option.UseTls {
		//Serve with TLS mode
		ln, err := tls.Listen("tcp", ":"+strconv.Itoa(router.Option.Port), config)
		if err != nil {
			log.Println(err)
			return err
		}
		router.tlsListener = ln
		router.server = &http.Server{Addr: ":" + strconv.Itoa(router.Option.Port), Handler: router.mux}
		router.Running = true

		if router.Option.Port == 443 && router.Option.ForceHttpsRedirect {
			//Add a 80 to 443 redirector
			httpServer := &http.Server{
				Addr: ":80",
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					http.Redirect(w, r, "https://"+r.Host+r.RequestURI, http.StatusTemporaryRedirect)
				}),
				ReadTimeout:  3 * time.Second,
				WriteTimeout: 3 * time.Second,
				IdleTimeout:  120 * time.Second,
			}

			log.Println("Starting HTTP-to-HTTPS redirector (port 80)")
			go func() {
				//Start another router to check if the router.server is killed. If yes, kill this server as well
				go func() {
					for router.server != nil {
						time.Sleep(100 * time.Millisecond)
					}
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					httpServer.Shutdown(ctx)
					log.Println(":80 to :433 redirection listener stopped")
				}()
				if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					log.Fatalf("Could not start server: %v\n", err)
				}
			}()
		}
		log.Println("Reverse proxy service started in the background (TLS mode)")
		go func() {
			if err := router.server.Serve(ln); err != nil && err != http.ErrServerClosed {
				log.Fatalf("Could not start server: %v\n", err)
			}
		}()
	} else {
		//Serve with non TLS mode
		router.tlsListener = nil
		router.server = &http.Server{Addr: ":" + strconv.Itoa(router.Option.Port), Handler: router.mux}
		router.Running = true
		log.Println("Reverse proxy service started in the background (Plain HTTP mode)")
		go func() {
			router.server.ListenAndServe()
			//log.Println("[DynamicProxy] " + err.Error())
		}()
	}

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

	if router.tlsListener != nil {
		router.tlsListener.Close()
	}

	//Discard the server object
	router.tlsListener = nil
	router.server = nil
	router.Running = false
	return nil
}

// Restart the current router if it is running.
// Startup the server if it is not running initially
func (router *Router) Restart() error {
	//Stop the router if it is already running
	if router.Running {
		err := router.StopProxyService()
		if err != nil {
			return err
		}
	}

	//Start the server
	err := router.StartProxyService()
	return err
}

/*
	Check if a given request is accessed via a proxied subdomain
*/

func (router *Router) IsProxiedSubdomain(r *http.Request) bool {
	hostname := r.Header.Get("X-Forwarded-Host")
	if hostname == "" {
		hostname = r.Host
	}
	hostname = strings.Split(hostname, ":")[0]
	subdEndpoint := router.getSubdomainProxyEndpointFromHostname(hostname)
	return subdEndpoint != nil
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
	//fmt.Println(ptype, key)
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

//Helpers to export the syncmap for easier processing
func (r *Router) GetSDProxyEndpointsAsMap() map[string]*SubdomainEndpoint {
	m := make(map[string]*SubdomainEndpoint)
	r.SubdomainEndpoint.Range(func(key, value interface{}) bool {
		k, ok := key.(string)
		if !ok {
			return true
		}
		v, ok := value.(*SubdomainEndpoint)
		if !ok {
			return true
		}
		m[k] = v
		return true
	})
	return m
}

func (r *Router) GetVDProxyEndpointsAsMap() map[string]*ProxyEndpoint {
	m := make(map[string]*ProxyEndpoint)
	r.ProxyEndpoints.Range(func(key, value interface{}) bool {
		k, ok := key.(string)
		if !ok {
			return true
		}
		v, ok := value.(*ProxyEndpoint)
		if !ok {
			return true
		}
		m[k] = v
		return true
	})
	return m
}
