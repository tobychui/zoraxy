package dynamicproxy

import (
	"context"
	"crypto/tls"
	"errors"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"imuslab.com/zoraxy/mod/dynamicproxy/dpcore"
)

/*
	Zoraxy Dynamic Proxy
*/

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
			router.Running = false
			return err
		}
		router.tlsListener = ln
		router.server = &http.Server{Addr: ":" + strconv.Itoa(router.Option.Port), Handler: router.mux}
		router.Running = true

		if router.Option.Port != 80 && router.Option.ForceHttpsRedirect {
			//Add a 80 to 443 redirector
			httpServer := &http.Server{
				Addr: ":80",
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					protocol := "https://"
					if router.Option.Port == 443 {
						http.Redirect(w, r, protocol+r.Host+r.RequestURI, http.StatusTemporaryRedirect)
					} else {
						http.Redirect(w, r, protocol+r.Host+":"+strconv.Itoa(router.Option.Port)+r.RequestURI, http.StatusTemporaryRedirect)
					}

				}),
				ReadTimeout:  3 * time.Second,
				WriteTimeout: 3 * time.Second,
				IdleTimeout:  120 * time.Second,
			}

			log.Println("Starting HTTP-to-HTTPS redirector (port 80)")

			//Create a redirection stop channel
			stopChan := make(chan bool)

			//Start a blocking wait for shutting down the http to https redirection server
			go func() {
				<-stopChan
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				httpServer.Shutdown(ctx)
				log.Println("HTTP to HTTPS redirection listener stopped")
			}()

			//Start the http server that listens to port 80 and redirect to 443
			go func() {
				if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					//Unable to startup port 80 listener. Handle shutdown process gracefully
					stopChan <- true
					log.Fatalf("Could not start server: %v\n", err)
				}
			}()
			router.tlsRedirectStop = stopChan
		}

		//Start the TLS server
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

	if router.tlsRedirectStop != nil {
		router.tlsRedirectStop <- true
	}

	//Discard the server object
	router.tlsListener = nil
	router.server = nil
	router.Running = false
	router.tlsRedirectStop = nil
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
func (router *Router) AddVirtualDirectoryProxyService(options *VdirOptions) error {
	domain := options.Domain
	if domain[len(domain)-1:] == "/" {
		domain = domain[:len(domain)-1]
	}

	/*
		if rootname[len(rootname)-1:] == "/" {
			rootname = rootname[:len(rootname)-1]
		}
	*/

	webProxyEndpoint := domain
	if options.RequireTLS {
		webProxyEndpoint = "https://" + webProxyEndpoint
	} else {
		webProxyEndpoint = "http://" + webProxyEndpoint
	}
	//Create a new proxy agent for this root
	path, err := url.Parse(webProxyEndpoint)
	if err != nil {
		return err
	}

	proxy := dpcore.NewDynamicProxyCore(path, options.RootName, options.SkipCertValidations)

	endpointObject := ProxyEndpoint{
		ProxyType:            ProxyType_Vdir,
		RootOrMatchingDomain: options.RootName,
		Domain:               domain,
		RequireTLS:           options.RequireTLS,
		SkipCertValidations:  options.SkipCertValidations,
		RequireBasicAuth:     options.RequireBasicAuth,
		BasicAuthCredentials: options.BasicAuthCredentials,
		Proxy:                proxy,
	}

	router.ProxyEndpoints.Store(options.RootName, &endpointObject)

	log.Println("Registered Proxy Rule: ", options.RootName+" to "+domain)
	return nil
}

/*
Load routing from RP
*/
func (router *Router) LoadProxy(ptype string, key string) (*ProxyEndpoint, error) {
	if ptype == "vdir" {
		proxy, ok := router.ProxyEndpoints.Load(key)
		if !ok {
			return nil, errors.New("target proxy not found")
		}
		return proxy.(*ProxyEndpoint), nil
	} else if ptype == "subd" {
		proxy, ok := router.SubdomainEndpoint.Load(key)
		if !ok {
			return nil, errors.New("target proxy not found")
		}
		return proxy.(*ProxyEndpoint), nil
	}

	return nil, errors.New("unsupported ptype")
}

/*
Save routing from RP
*/
func (router *Router) SaveProxy(ptype string, key string, newConfig *ProxyEndpoint) {
	if ptype == "vdir" {
		router.ProxyEndpoints.Store(key, newConfig)

	} else if ptype == "subd" {
		router.SubdomainEndpoint.Store(key, newConfig)
	}

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
func (router *Router) SetRootProxy(options *RootOptions) error {
	proxyLocation := options.ProxyLocation
	if proxyLocation[len(proxyLocation)-1:] == "/" {
		proxyLocation = proxyLocation[:len(proxyLocation)-1]
	}

	webProxyEndpoint := proxyLocation
	if options.RequireTLS {
		webProxyEndpoint = "https://" + webProxyEndpoint
	} else {
		webProxyEndpoint = "http://" + webProxyEndpoint
	}
	//Create a new proxy agent for this root
	path, err := url.Parse(webProxyEndpoint)
	if err != nil {
		return err
	}

	proxy := dpcore.NewDynamicProxyCore(path, "", options.SkipCertValidations)

	rootEndpoint := ProxyEndpoint{
		ProxyType:            ProxyType_Vdir,
		RootOrMatchingDomain: "/",
		Domain:               proxyLocation,
		RequireTLS:           options.RequireTLS,
		SkipCertValidations:  options.SkipCertValidations,
		RequireBasicAuth:     options.RequireBasicAuth,
		BasicAuthCredentials: options.BasicAuthCredentials,
		Proxy:                proxy,
	}

	router.Root = &rootEndpoint
	return nil
}

// Helpers to export the syncmap for easier processing
func (r *Router) GetSDProxyEndpointsAsMap() map[string]*ProxyEndpoint {
	m := make(map[string]*ProxyEndpoint)
	r.SubdomainEndpoint.Range(func(key, value interface{}) bool {
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
