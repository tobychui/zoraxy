package main

import (
	"encoding/json"
	"log"
	"net/http"
	"path/filepath"

	"imuslab.com/arozos/ReverseProxy/mod/dynamicproxy"
)

var (
	dynamicProxyRouter *dynamicproxy.Router
)

//Add user customizable reverse proxy
func ReverseProxtInit() {
	dprouter, err := dynamicproxy.NewDynamicProxy(80)
	if err != nil {
		log.Println(err.Error())
		return
	}

	dynamicProxyRouter = dprouter

	http.HandleFunc("/enable", ReverseProxyHandleOnOff)
	http.HandleFunc("/add", ReverseProxyHandleAddEndpoint)
	http.HandleFunc("/status", ReverseProxyStatus)
	http.HandleFunc("/list", ReverseProxyList)
	http.HandleFunc("/del", DeleteProxyEndpoint)

	//Load all conf from files
	confs, _ := filepath.Glob("./conf/*.config")
	for _, conf := range confs {
		record, err := LoadReverseProxyConfig(conf)
		if err != nil {
			log.Println("Failed to load "+filepath.Base(conf), err.Error())
			return
		}

		if record.ProxyType == "root" {
			dynamicProxyRouter.SetRootProxy(record.ProxyTarget, record.UseTLS)
		} else if record.ProxyType == "subd" {
			dynamicProxyRouter.AddSubdomainRoutingService(record.Rootname, record.ProxyTarget, record.UseTLS)
		} else if record.ProxyType == "vdir" {
			dynamicProxyRouter.AddVirtualDirectoryProxyService(record.Rootname, record.ProxyTarget, record.UseTLS)
		} else {
			log.Println("Unsupported endpoint type: " + record.ProxyType + ". Skipping " + filepath.Base(conf))
		}
	}

	/*
		dynamicProxyRouter.SetRootProxy("192.168.0.107:8080", false)
		dynamicProxyRouter.AddSubdomainRoutingService("aroz.localhost", "192.168.0.107:8080/private/AOB/", false)
		dynamicProxyRouter.AddSubdomainRoutingService("loopback.localhost", "localhost:8080", false)
		dynamicProxyRouter.AddSubdomainRoutingService("git.localhost", "mc.alanyeung.co:3000", false)
		dynamicProxyRouter.AddVirtualDirectoryProxyService("/git/server/", "mc.alanyeung.co:3000", false)
	*/

	//Start Service
	dynamicProxyRouter.StartProxyService()

	/*
		go func() {
			time.Sleep(10 * time.Second)
			dynamicProxyRouter.StopProxyService()
			fmt.Println("Proxy stopped")
		}()
	*/
	log.Println("Dynamic Proxy service started")

}

func ReverseProxyHandleOnOff(w http.ResponseWriter, r *http.Request) {
	enable, _ := mv(r, "enable", true) //Support root, vdir and subd
	if enable == "true" {
		err := dynamicProxyRouter.StartProxyService()
		if err != nil {
			sendErrorResponse(w, err.Error())
			return
		}
	} else {
		err := dynamicProxyRouter.StopProxyService()
		if err != nil {
			sendErrorResponse(w, err.Error())
			return
		}
	}

	sendOK(w)
}

func ReverseProxyHandleAddEndpoint(w http.ResponseWriter, r *http.Request) {
	eptype, err := mv(r, "type", true) //Support root, vdir and subd
	if err != nil {
		sendErrorResponse(w, "type not defined")
		return
	}

	endpoint, err := mv(r, "ep", true)
	if err != nil {
		sendErrorResponse(w, "endpoint not defined")
		return
	}

	tls, _ := mv(r, "tls", true)
	if tls == "" {
		tls = "false"
	}

	useTLS := (tls == "true")
	rootname := ""
	if eptype == "vdir" {
		vdir, err := mv(r, "rootname", true)
		if err != nil {
			sendErrorResponse(w, "vdir not defined")
			return
		}
		rootname = vdir
		dynamicProxyRouter.AddVirtualDirectoryProxyService(vdir, endpoint, useTLS)

	} else if eptype == "subd" {
		subdomain, err := mv(r, "rootname", true)
		if err != nil {
			sendErrorResponse(w, "subdomain not defined")
			return
		}
		rootname = subdomain
		dynamicProxyRouter.AddSubdomainRoutingService(subdomain, endpoint, useTLS)
	} else if eptype == "root" {
		rootname = "root"
		dynamicProxyRouter.SetRootProxy(endpoint, useTLS)
	} else {
		//Invalid eptype
		sendErrorResponse(w, "Invalid endpoint type")
		return
	}

	//Save it
	SaveReverseProxyConfig(eptype, rootname, endpoint, useTLS)

	sendOK(w)

}

func DeleteProxyEndpoint(w http.ResponseWriter, r *http.Request) {
	ep, err := mv(r, "ep", true)
	if err != nil {
		sendErrorResponse(w, "Invalid ep given")
	}

	ptype, err := mv(r, "ptype", true)
	if err != nil {
		sendErrorResponse(w, "Invalid ptype given")
	}

	err = dynamicProxyRouter.RemoveProxy(ptype, ep)
	if err != nil {
		sendErrorResponse(w, err.Error())
	}

	RemoveReverseProxyConfig(ep)
	sendOK(w)
}

func ReverseProxyStatus(w http.ResponseWriter, r *http.Request) {
	js, _ := json.Marshal(dynamicProxyRouter)
	sendJSONResponse(w, string(js))
}

func ReverseProxyList(w http.ResponseWriter, r *http.Request) {
	eptype, err := mv(r, "type", true) //Support root, vdir and subd
	if err != nil {
		sendErrorResponse(w, "type not defined")
		return
	}

	if eptype == "vdir" {
		results := []*dynamicproxy.ProxyEndpoint{}
		dynamicProxyRouter.ProxyEndpoints.Range(func(key, value interface{}) bool {
			results = append(results, value.(*dynamicproxy.ProxyEndpoint))
			return true
		})

		js, _ := json.Marshal(results)
		sendJSONResponse(w, string(js))
	} else if eptype == "subd" {
		results := []*dynamicproxy.SubdomainEndpoint{}
		dynamicProxyRouter.SubdomainEndpoint.Range(func(key, value interface{}) bool {
			results = append(results, value.(*dynamicproxy.SubdomainEndpoint))
			return true
		})
		js, _ := json.Marshal(results)
		sendJSONResponse(w, string(js))
	} else if eptype == "root" {
		js, _ := json.Marshal(dynamicProxyRouter.Root)
		sendJSONResponse(w, string(js))
	} else {
		sendErrorResponse(w, "Invalid type given")
	}
}
