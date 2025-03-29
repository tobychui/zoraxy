package main

import (
	"embed"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"plugins.zoraxy.aroz.org/zoraxy/upnp/mod/upnpc"
	plugin "plugins.zoraxy.aroz.org/zoraxy/upnp/mod/zoraxy_plugin"
)

const (
	PLUGIN_ID           = "org.aroz.zoraxy.plugins.upnp"
	UI_PATH             = "/ui"
	WEB_ROOT            = "/www"
	CONFIG_FILE         = "upnp.json"
	AUTO_RENEW_INTERVAL = 12 * 60 * 60 // 12 hours
)

type PortForwardRecord struct {
	RuleName   string
	PortNumber int
}

type UPnPConfig struct {
	ForwardRules []*PortForwardRecord
	Enabled      bool
}

//go:embed www/*
var content embed.FS

// Runtime variables
var (
	upnpRouterExists  bool        = false
	upnpRuntimeConfig *UPnPConfig = &UPnPConfig{
		ForwardRules: []*PortForwardRecord{},
		Enabled:      false,
	}
	upnpClient      *upnpc.UPnPClient = nil
	renewTickerStop chan bool
)

func main() {
	//Handle introspect
	runtimeCfg, err := plugin.ServeAndRecvSpec(&plugin.IntroSpect{
		ID:            PLUGIN_ID,
		Name:          "UPnP Forwarder",
		Author:        "aroz.org",
		AuthorContact: "https://github.com/aroz-online",
		Description:   "A UPnP Port Forwarder Plugin for Zoraxy",
		URL:           "https://github.com/aroz-online",
		Type:          plugin.PluginType_Utilities,
		VersionMajor:  1,
		VersionMinor:  0,
		VersionPatch:  0,
		UIPath:        UI_PATH,
	})
	if err != nil {
		//Terminate or enter standalone mode here
		fmt.Println("This is a plugin for Zoraxy and should not be run standalone\n Visit zoraxy.aroz.org to download Zoraxy.")
		panic(err)
	}

	//Read the configuration from file
	if _, err := os.Stat(CONFIG_FILE); os.IsNotExist(err) {
		err = os.WriteFile(CONFIG_FILE, []byte("{}"), 0644)
		if err != nil {
			panic(err)
		}
	}

	cfgBytes, err := os.ReadFile(CONFIG_FILE)
	if err != nil {
		panic(err)
	}

	//Load the configuration
	err = json.Unmarshal(cfgBytes, &upnpRuntimeConfig)
	if err != nil {
		panic(err)
	}

	//Start upnp client and auto-renew ticker
	go func() {
		TryStartUPnPClient()
	}()

	//Serve the plugin UI
	embedWebRouter := plugin.NewPluginEmbedUIRouter(PLUGIN_ID, &content, WEB_ROOT, UI_PATH)
	// For debugging, use the following line instead
	//embedWebRouter := plugin.NewPluginFileSystemUIRouter(PLUGIN_ID, "."+WEB_ROOT, UI_PATH)
	//embedWebRouter.EnableDebug = true
	embedWebRouter.RegisterTerminateHandler(func() {
		if renewTickerStop != nil {
			renewTickerStop <- true
		}
		// Do cleanup here if needed
		upnpClient.Close()
	}, nil)
	embedWebRouter.AttachHandlerToMux(nil)

	//Serve the API
	RegisterAPIs()

	//Start the IO server
	fmt.Println("UPnP Forwarder started at http://127.0.0.1:" + strconv.Itoa(runtimeCfg.Port))
	err = http.ListenAndServe("127.0.0.1:"+strconv.Itoa(runtimeCfg.Port), nil)
	if err != nil {
		panic(err)
	}
}

// RegisterAPIs registers the APIs for the plugin
func RegisterAPIs() {
	http.HandleFunc(UI_PATH+"/api/usable", handleUsableState)
	http.HandleFunc(UI_PATH+"/api/enable", handleEnableState)
	http.HandleFunc(UI_PATH+"/api/forward", handleForwardPort)
	http.HandleFunc(UI_PATH+"/api/edit", handleForwardPortEdit)
	http.HandleFunc(UI_PATH+"/api/remove", handleForwardPortRemove)
}

// TryStartUPnPClient tries to start the UPnP client
func TryStartUPnPClient() {
	if renewTickerStop != nil {
		renewTickerStop <- true
	}

	// Create UPnP client
	upnpClient, err := upnpc.NewUPNPClient()
	if err != nil {
		upnpRouterExists = false
		upnpRuntimeConfig.Enabled = false
		fmt.Println("UPnP router not found")
		SaveRuntimeConfig()
		return
	}
	upnpRouterExists = true

	//Check if the client is enabled by default
	if upnpRuntimeConfig.Enabled {
		// Forward all the ports
		for _, rule := range upnpRuntimeConfig.ForwardRules {
			err = upnpClient.ForwardPort(rule.PortNumber, rule.RuleName)
			if err != nil {
				fmt.Println("Unable to forward port", rule.PortNumber, ":", err)
				return
			}
		}
	}

	// Start the auto-renew ticker
	_, renewTickerStop = SetupAutoRenewTicker()
}

// SetupAutoRenewTicker sets up a ticker for auto-renewing the port forwarding rules
func SetupAutoRenewTicker() (*time.Ticker, chan bool) {
	ticker := time.NewTicker(AUTO_RENEW_INTERVAL * time.Second)
	closeChan := make(chan bool)
	go func() {
		for {
			select {
			case <-closeChan:
				ticker.Stop()
				return
			case <-ticker.C:
				if upnpClient != nil {
					upnpClient.RenewForwardRules()
				}
			}
		}
	}()
	return ticker, closeChan
}

// SaveRuntimeConfig saves the runtime configuration to file
func SaveRuntimeConfig() error {
	cfgBytes, err := json.Marshal(upnpRuntimeConfig)
	if err != nil {
		return err
	}

	err = os.WriteFile(CONFIG_FILE, cfgBytes, 0644)
	if err != nil {
		return err
	}

	return nil
}
