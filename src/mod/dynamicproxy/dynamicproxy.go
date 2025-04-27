package dynamicproxy

import (
	"context"
	"crypto/tls"
	"encoding/json"
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
	thisRouter := Router{
		Option:           &option,
		ProxyEndpoints:   &proxyMap,
		Running:          false,
		server:           nil,
		routingRules:     []*RoutingRule{},
		loadBalancer:     option.LoadBalancer,
		rateLimitCounter: RequestCountPerIpTable{},
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

// Update TLS Version in runtime. Will restart proxy server if running.
// Set this to true to force TLS 1.2 or above
func (router *Router) UpdateTLSVersion(requireLatest bool) {
	router.Option.ForceTLSLatest = requireLatest
	router.Restart()
}

// Update port 80 listener state
func (router *Router) UpdatePort80ListenerState(useRedirect bool) {
	router.Option.ListenOnPort80 = useRedirect
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
		return errors.New("reverse proxy server already running")
	}

	//Check if root route is set
	if router.Root == nil {
		return errors.New("reverse proxy router root not set")
	}

	minVersion := tls.VersionTLS10
	if router.Option.ForceTLSLatest {
		minVersion = tls.VersionTLS12
	}
	config := &tls.Config{
		GetCertificate: router.Option.TlsManager.GetCert,
		MinVersion:     uint16(minVersion),
	}

	//Start rate limitor
	err := router.startRateLimterCounterResetTicker()
	if err != nil {
		return err
	}

	if router.Option.UseTls {
		router.server = &http.Server{
			Addr:      ":" + strconv.Itoa(router.Option.Port),
			Handler:   router.mux,
			TLSConfig: config,
		}
		router.Running = true

		if router.Option.Port != 80 && router.Option.ListenOnPort80 {
			//Add a 80 to 443 redirector
			httpServer := &http.Server{
				Addr: ":80",
				Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					//Check if the domain requesting allow non TLS mode
					domainOnly := r.Host
					if strings.Contains(r.Host, ":") {
						hostPath := strings.Split(r.Host, ":")
						domainOnly = hostPath[0]
					}
					sep := router.getProxyEndpointFromHostname(domainOnly)
					if sep != nil && sep.BypassGlobalTLS {
						//Allow routing via non-TLS handler
						originalHostHeader := r.Host
						if r.URL != nil {
							r.Host = r.URL.Host
						} else {
							//Fallback when the upstream proxy screw something up in the header
							r.URL, _ = url.Parse(originalHostHeader)
						}

						//Access Check (blacklist / whitelist)
						ruleID := sep.AccessFilterUUID
						if sep.AccessFilterUUID == "" {
							//Use default rule
							ruleID = "default"
						}
						accessRule, err := router.Option.AccessController.GetAccessRuleByID(ruleID)
						if err == nil {
							isBlocked, _ := accessRequestBlocked(accessRule, router.Option.WebDirectory, w, r)
							if isBlocked {
								return
							}
						}

						// Rate Limit
						if sep.RequireRateLimit {
							if err := router.handleRateLimit(w, r, sep); err != nil {
								return
							}
						}

						//Validate basic auth
						if sep.AuthenticationProvider.AuthMethod == AuthMethodBasic {
							err := handleBasicAuth(w, r, sep)
							if err != nil {
								return
							}
						}

						selectedUpstream, err := router.loadBalancer.GetRequestUpstreamTarget(w, r, sep.ActiveOrigins, sep.UseStickySession)
						if err != nil {
							http.ServeFile(w, r, "./web/hosterror.html")
							router.Option.Logger.PrintAndLog("dprouter", "failed to get upstream for hostname", err)
							router.logRequest(r, false, 404, "vdir-http", r.Host, "")
						}

						endpointProxyRewriteRules := GetDefaultHeaderRewriteRules()
						if sep.HeaderRewriteRules != nil {
							endpointProxyRewriteRules = sep.HeaderRewriteRules
						}

						selectedUpstream.ServeHTTP(w, r, &dpcore.ResponseRewriteRuleSet{
							ProxyDomain:         selectedUpstream.OriginIpOrDomain,
							OriginalHost:        originalHostHeader,
							UseTLS:              selectedUpstream.RequireTLS,
							HostHeaderOverwrite: endpointProxyRewriteRules.RequestHostOverwrite,
							NoRemoveHopByHop:    endpointProxyRewriteRules.DisableHopByHopHeaderRemoval,
							PathPrefix:          "",
							Version:             sep.parent.Option.HostVersion,
						})
						return
					}

					if router.Option.ForceHttpsRedirect {
						//Redirect to https is enabled
						protocol := "https://"
						if router.Option.Port == 443 {
							http.Redirect(w, r, protocol+r.Host+r.RequestURI, http.StatusTemporaryRedirect)
						} else {
							http.Redirect(w, r, protocol+r.Host+":"+strconv.Itoa(router.Option.Port)+r.RequestURI, http.StatusTemporaryRedirect)
						}
					} else {
						//Do not do redirection
						if sep != nil {
							//Sub-domain exists but not allow non-TLS access
							w.WriteHeader(http.StatusBadRequest)
							w.Write([]byte("400 - Bad Request"))
						} else {
							//No defined sub-domain
							if router.Root.DefaultSiteOption == DefaultSite_NoResponse {
								//No response. Just close the connection
								hijacker, ok := w.(http.Hijacker)
								if !ok {
									w.Header().Set("Connection", "close")
									return
								}
								conn, _, err := hijacker.Hijack()
								if err != nil {
									w.Header().Set("Connection", "close")
									return
								}
								conn.Close()
							} else {
								//Default behavior
								http.NotFound(w, r)
							}

						}

					}

				}),
				ReadTimeout:  3 * time.Second,
				WriteTimeout: 3 * time.Second,
				IdleTimeout:  120 * time.Second,
			}

			router.Option.Logger.PrintAndLog("dprouter", "Starting HTTP-to-HTTPS redirector (port 80)", nil)

			//Create a redirection stop channel
			stopChan := make(chan bool)

			//Start a blocking wait for shutting down the http to https redirection server
			go func() {
				<-stopChan
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				httpServer.Shutdown(ctx)
				router.Option.Logger.PrintAndLog("dprouter", "HTTP to HTTPS redirection listener stopped", nil)
			}()

			//Start the http server that listens to port 80 and redirect to 443
			go func() {
				if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
					//Unable to startup port 80 listener. Handle shutdown process gracefully
					stopChan <- true
					log.Fatalf("Could not start redirection server: %v\n", err)
				}
			}()
			router.tlsRedirectStop = stopChan
		}

		//Start the TLS server
		router.Option.Logger.PrintAndLog("dprouter", "Reverse proxy service started in the background (TLS mode)", nil)
		go func() {
			if err := router.server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
				router.Option.Logger.PrintAndLog("dprouter", "Could not start proxy server", err)
			}
		}()
	} else {
		//Serve with non TLS mode
		router.tlsListener = nil
		router.server = &http.Server{Addr: ":" + strconv.Itoa(router.Option.Port), Handler: router.mux}
		router.Running = true
		router.Option.Logger.PrintAndLog("dprouter", "Reverse proxy service started in the background (Plain HTTP mode)", nil)
		go func() {
			router.server.ListenAndServe()
		}()
	}

	return nil
}

func (router *Router) StopProxyService() error {
	if router.server == nil {
		return errors.New("reverse proxy server already stopped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	err := router.server.Shutdown(ctx)
	if err != nil {
		return err
	}

	//Stop TLS listener
	if router.tlsListener != nil {
		router.tlsListener.Close()
	}

	//Stop rate limiter
	if router.rateLimterStop != nil {
		go func() {
			// As the rate timer loop has a 1 sec ticker
			// stop the rate limiter in go routine can prevent
			// front end from freezing for 1 sec
			router.rateLimterStop <- true
		}()

	}

	//Stop TLS redirection (from port 80)
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
func (router *Router) Restart() error {
	//Stop the router if it is already running
	if router.Running {
		err := router.StopProxyService()
		if err != nil {
			return err
		}

		time.Sleep(100 * time.Millisecond)
		// Start the server
		err = router.StartProxyService()
		if err != nil {
			return err
		}
	}

	return nil
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
	subdEndpoint := router.getProxyEndpointFromHostname(hostname)
	return subdEndpoint != nil
}

/*
Load routing from RP
*/
func (router *Router) LoadProxy(matchingDomain string) (*ProxyEndpoint, error) {
	var targetProxyEndpoint *ProxyEndpoint
	router.ProxyEndpoints.Range(func(key, value interface{}) bool {
		key, ok := key.(string)
		if !ok {
			return true
		}
		v, ok := value.(*ProxyEndpoint)
		if !ok {
			return true
		}

		if key == strings.ToLower(matchingDomain) {
			targetProxyEndpoint = v
		}
		return true
	})

	if targetProxyEndpoint == nil {
		return nil, errors.New("target routing rule not found")
	}

	return targetProxyEndpoint, nil
}

// Deep copy a proxy endpoint, excluding runtime paramters
func CopyEndpoint(endpoint *ProxyEndpoint) *ProxyEndpoint {
	js, _ := json.Marshal(endpoint)
	newProxyEndpoint := ProxyEndpoint{}
	err := json.Unmarshal(js, &newProxyEndpoint)
	if err != nil {
		return nil
	}
	return &newProxyEndpoint
}

func (r *Router) GetProxyEndpointsAsMap() map[string]*ProxyEndpoint {
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
