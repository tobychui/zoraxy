package zoraxy_plugin

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

/*
	Plugins Includes.go

	This file is copied from Zoraxy source code
	You can always find the latest version under mod/plugins/includes.go
	Usually this file are backward compatible
*/

type PluginType int

const (
	PluginType_Router    PluginType = 0 //Router Plugin, used for handling / routing / forwarding traffic
	PluginType_Utilities PluginType = 1 //Utilities Plugin, used for utilities like Zerotier or Static Web Server that do not require interception with the dpcore
)

type StaticCaptureRule struct {
	CapturePath string `json:"capture_path"`
	//To be expanded
}

type ControlStatusCode int

const (
	ControlStatusCode_CAPTURED  ControlStatusCode = 280 //Traffic captured by plugin, ask Zoraxy not to process the traffic
	ControlStatusCode_UNHANDLED ControlStatusCode = 284 //Traffic not handled by plugin, ask Zoraxy to process the traffic
	ControlStatusCode_ERROR     ControlStatusCode = 580 //Error occurred while processing the traffic, ask Zoraxy to process the traffic and log the error
)

type SubscriptionEvent struct {
	EventName   string `json:"event_name"`
	EventSource string `json:"event_source"`
	Payload     string `json:"payload"` //Payload of the event, can be empty
}

type RuntimeConstantValue struct {
	ZoraxyVersion    string `json:"zoraxy_version"`
	ZoraxyUUID       string `json:"zoraxy_uuid"`
	DevelopmentBuild bool   `json:"development_build"` //Whether the Zoraxy is a development build or not
}

type PermittedAPIEndpoint struct {
	Method   string `json:"method"`   //HTTP method for the API endpoint (e.g., GET, POST)
	Endpoint string `json:"endpoint"` //The API endpoint that the plugin can access
	Reason   string `json:"reason"`   //The reason why the plugin needs to access this endpoint
}

/*
IntroSpect Payload

When the plugin is initialized with -introspect flag,
the plugin shell return this payload as JSON and exit
*/
type IntroSpect struct {
	/* Plugin metadata */
	ID            string     `json:"id"`             //Unique ID of your plugin, recommended using your own domain in reverse like com.yourdomain.pluginname
	Name          string     `json:"name"`           //Name of your plugin
	Author        string     `json:"author"`         //Author name of your plugin
	AuthorContact string     `json:"author_contact"` //Author contact of your plugin, like email
	Description   string     `json:"description"`    //Description of your plugin
	URL           string     `json:"url"`            //URL of your plugin
	Type          PluginType `json:"type"`           //Type of your plugin, Router(0) or Utilities(1)
	VersionMajor  int        `json:"version_major"`  //Major version of your plugin
	VersionMinor  int        `json:"version_minor"`  //Minor version of your plugin
	VersionPatch  int        `json:"version_patch"`  //Patch version of your plugin

	/*

		Endpoint Settings

	*/

	/*
		Static Capture Settings

		Once plugin is enabled these rules always applies to the enabled HTTP Proxy rule
		This is faster than dynamic capture, but less flexible
	*/
	StaticCapturePaths   []StaticCaptureRule `json:"static_capture_paths"`   //Static capture paths of your plugin, see Zoraxy documentation for more details
	StaticCaptureIngress string              `json:"static_capture_ingress"` //Static capture ingress path of your plugin (e.g. /s_handler)

	/*
		Dynamic Capture Settings

		Once plugin is enabled, these rules will be captured and forward to plugin sniff
		if the plugin sniff returns 280, the traffic will be captured
		otherwise, the traffic will be forwarded to the next plugin
		This is slower than static capture, but more flexible
	*/
	DynamicCaptureSniff   string `json:"dynamic_capture_sniff"`   //Dynamic capture sniff path of your plugin (e.g. /d_sniff)
	DynamicCaptureIngress string `json:"dynamic_capture_ingress"` //Dynamic capture ingress path of your plugin (e.g. /d_handler)

	/* UI Path for your plugin */
	UIPath string `json:"ui_path"` //UI path of your plugin (e.g. /ui), will proxy the whole subpath tree to Zoraxy Web UI as plugin UI

	/* Subscriptions Settings */
	SubscriptionPath    string            `json:"subscription_path"`    //Subscription event path of your plugin (e.g. /notifyme), a POST request with SubscriptionEvent as body will be sent to this path when the event is triggered
	SubscriptionsEvents map[string]string `json:"subscriptions_events"` //Subscriptions events of your plugin, paired with comments describing how the event is used, see Zoraxy documentation for more details

	/* API Access Control */
	PermittedAPIEndpoints []PermittedAPIEndpoint `json:"permitted_api_endpoints"` //List of API endpoints this plugin can access, and a description of why the plugin needs to access this endpoint
}

/*
ServeIntroSpect Function

This function will check if the plugin is initialized with -introspect flag,
if so, it will print the intro spect and exit

Place this function at the beginning of your plugin main function
*/
func ServeIntroSpect(pluginSpect *IntroSpect) {
	if len(os.Args) > 1 && os.Args[1] == "-introspect" {
		//Print the intro spect and exit
		jsonData, _ := json.MarshalIndent(pluginSpect, "", " ")
		fmt.Println(string(jsonData))
		os.Exit(0)
	}
}

/*
ConfigureSpec Payload

Zoraxy will start your plugin with -configure flag,
the plugin shell read this payload as JSON and configure itself
by the supplied values like starting a web server at given port
that listens to 127.0.0.1:port
*/
type ConfigureSpec struct {
	Port         int                  `json:"port"`                  //Port to listen
	RuntimeConst RuntimeConstantValue `json:"runtime_const"`         //Runtime constant values
	APIKey       string               `json:"api_key,omitempty"`     //API key for accessing Zoraxy APIs, if the plugin has permitted endpoints
	ZoraxyPort   int                  `json:"zoraxy_port,omitempty"` //The port that Zoraxy is running on, used for making API calls to Zoraxy
	//To be expanded
}

/*
RecvExecuteConfigureSpec Function

This function will read the configure spec from Zoraxy
and return the ConfigureSpec object

Place this function after ServeIntroSpect function in your plugin main function
*/
func RecvConfigureSpec() (*ConfigureSpec, error) {
	for i, arg := range os.Args {
		if strings.HasPrefix(arg, "-configure=") {
			var configSpec ConfigureSpec
			if err := json.Unmarshal([]byte(arg[11:]), &configSpec); err != nil {
				return nil, err
			}
			return &configSpec, nil
		} else if arg == "-configure" {
			var configSpec ConfigureSpec
			var nextArg string
			if len(os.Args) > i+1 {
				nextArg = os.Args[i+1]
				if err := json.Unmarshal([]byte(nextArg), &configSpec); err != nil {
					return nil, err
				}
			} else {
				return nil, fmt.Errorf("no port specified after -configure flag")
			}
			return &configSpec, nil
		}
	}
	return nil, fmt.Errorf("no -configure flag found")
}

/*
ServeAndRecvSpec Function

This function will serve the intro spect and return the configure spec
See the ServeIntroSpect and RecvConfigureSpec for more details
*/
func ServeAndRecvSpec(pluginSpect *IntroSpect) (*ConfigureSpec, error) {
	ServeIntroSpect(pluginSpect)
	return RecvConfigureSpec()
}
