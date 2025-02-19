package plugins

/*
	Plugins Includes.go

	This file contains the common types and structs that are used by the plugins
	If you are building a Zoraxy plugin with Golang, you can use this file to include
	the common types and structs that are used by the plugins
*/

type PluginType int

const (
	PluginType_Router    PluginType = 0 //Router Plugin, used for handling / routing / forwarding traffic
	PluginType_Utilities PluginType = 1 //Utilities Plugin, used for utilities like Zerotier or Static Web Server that do not require interception with the dpcore
)

type CaptureRule struct {
	CapturePath     string `json:"capture_path"`
	IncludeSubPaths bool   `json:"include_sub_paths"`
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
	ZoraxyVersion string `json:"zoraxy_version"`
	ZoraxyUUID    string `json:"zoraxy_uuid"`
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
		Global Capture Settings

		Once plugin is enabled these rules always applies, no matter which HTTP Proxy rule it is enabled on
		This captures the whole traffic of Zoraxy

		Notes: Will raise a warning on the UI when the user enables the plugin on a HTTP Proxy rule
	*/
	GlobalCapturePath    []CaptureRule `json:"global_capture_path"`    //Global traffic capture path of your plugin
	GlobalCaptureIngress string        `json:"global_capture_ingress"` //Global traffic capture ingress path of your plugin (e.g. /g_handler)

	/*
		Always Capture Settings

		Once the plugin is enabled on a given HTTP Proxy rule,
		these always applies
	*/
	AlwaysCapturePath    []CaptureRule `json:"always_capture_path"`    //Always capture path of your plugin when enabled on a HTTP Proxy rule (e.g. /myapp)
	AlwaysCaptureIngress string        `json:"always_capture_ingress"` //Always capture ingress path of your plugin when enabled on a HTTP Proxy rule (e.g. /a_handler)

	/*
		Dynamic Capture Settings

		Once the plugin is enabled on a given HTTP Proxy rule,
		the plugin can capture the request and decided if the request
		shall be handled by itself or let it pass through

	*/
	DynmaicCaptureIngress string `json:"capture_path"` //Traffic capture path of your plugin (e.g. /capture)
	DynamicHandleIngress  string `json:"handle_path"`  //Traffic handle path of your plugin (e.g. /handler)

	/* UI Path for your plugin */
	UIPath string `json:"ui_path"` //UI path of your plugin (e.g. /ui), will proxy the whole subpath tree to Zoraxy Web UI as plugin UI

	/* Subscriptions Settings */
	SubscriptionPath    string            `json:"subscription_path"`    //Subscription event path of your plugin (e.g. /notifyme), a POST request with SubscriptionEvent as body will be sent to this path when the event is triggered
	SubscriptionsEvents map[string]string `json:"subscriptions_events"` //Subscriptions events of your plugin, see Zoraxy documentation for more details
}

/*
ConfigureSpec Payload

Zoraxy will start your plugin with -configure flag,
the plugin shell read this payload as JSON and configure itself
by the supplied values like starting a web server at given port
that listens to 127.0.0.1:port
*/
type ConfigureSpec struct {
	Port         int                  `json:"port"`          //Port to listen
	RuntimeConst RuntimeConstantValue `json:"runtime_const"` //Runtime constant values
	//To be expanded
}
