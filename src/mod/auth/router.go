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

func (router *RouterDef) HandleFunc(endpoint string, handler func(http.ResponseWriter, *http.Request)) error {
	//Check if the endpoint already registered
	if _, exist := router.endpoints[endpoint]; exist {
		fmt.Println("WARNING! Duplicated registering of web endpoint: " + endpoint)
		return errors.New("endpoint register duplicated")
	}

	authAgent := router.option.AuthAgent

	//OK. Register handler
	http.HandleFunc(endpoint, func(w http.ResponseWriter, r *http.Request) {
		//Check authentication of the user
		if router.option.RequireAuth {
			authAgent.HandleCheckAuth(w, r, func(w http.ResponseWriter, r *http.Request) {
				handler(w, r)
			})
		} else {
			handler(w, r)
		}

	})

	router.endpoints[endpoint] = handler

	return nil
}
