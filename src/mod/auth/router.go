package auth

import (
	"errors"
	"fmt"
	"net/http"
)

type RouterOption struct {
	AuthAgent     *AuthAgent
	RequireAuth   bool                                     //This router require authentication
	DeniedHandler func(http.ResponseWriter, *http.Request) //Things to do when request is rejected
	TargetMux     *http.ServeMux
}

type RouterDef struct {
	option    RouterOption
	endpoints map[string]func(http.ResponseWriter, *http.Request)
}

func NewManagedHTTPRouter(option RouterOption) *RouterDef {
	return &RouterDef{
		option:    option,
		endpoints: map[string]func(http.ResponseWriter, *http.Request){},
	}
}

func (router *RouterDef) HandleFunc(endpoint string, handler func(http.ResponseWriter, *http.Request), pluginAccessible bool) error {
	//Check if the endpoint already registered
	if _, exist := router.endpoints[endpoint]; exist {
		fmt.Println("WARNING! Duplicated registering of web endpoint: " + endpoint)
		return errors.New("endpoint register duplicated")
	}

	authAgent := router.option.AuthAgent

	authWrapper := func(w http.ResponseWriter, r *http.Request) {
		//Check authentication of the user
		X_Plugin_Auth := r.Header.Get("X-Zoraxy-Plugin-Auth")
		if router.option.RequireAuth && !(pluginAccessible && X_Plugin_Auth == "true") {
			authAgent.HandleCheckAuth(w, r, func(w http.ResponseWriter, r *http.Request) {
				handler(w, r)
			})
		} else {
			handler(w, r)
		}
	}

	// if the endpoint is supposed to be plugin accessible, wrap it with plugin authentication middleware
	if pluginAccessible {
		authWrapper = router.option.AuthAgent.PluginAuthMiddleware.WrapHandler(endpoint, authWrapper)
	}

	//OK. Register handler
	if router.option.TargetMux == nil {
		http.HandleFunc(endpoint, authWrapper)
	} else {
		router.option.TargetMux.HandleFunc(endpoint, authWrapper)
	}

	router.endpoints[endpoint] = handler

	return nil
}
