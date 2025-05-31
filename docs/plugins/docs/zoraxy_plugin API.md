# Zoraxy Plugin APIs
This API documentation is auto-generated from the Zoraxy plugin source code.


<pre><code class='language-go'>
package zoraxy_plugin // import "{{your_module_package_name_in_go.mod}}/mod/plugins/zoraxy_plugin"


FUNCTIONS

func ServeIntroSpect(pluginSpect *IntroSpect)
    ServeIntroSpect Function

    This function will check if the plugin is initialized with -introspect flag,
    if so, it will print the intro spect and exit

    Place this function at the beginning of your plugin main function


TYPES

type ConfigureSpec struct {
	Port         int                  `json:"port"`          //Port to listen
	RuntimeConst RuntimeConstantValue `json:"runtime_const"` //Runtime constant values

}
    ConfigureSpec Payload

    Zoraxy will start your plugin with -configure flag, the plugin shell read
    this payload as JSON and configure itself by the supplied values like
    starting a web server at given port that listens to 127.0.0.1:port

func RecvConfigureSpec() (*ConfigureSpec, error)
    RecvExecuteConfigureSpec Function

    This function will read the configure spec from Zoraxy and return the
    ConfigureSpec object

    Place this function after ServeIntroSpect function in your plugin main
    function

func ServeAndRecvSpec(pluginSpect *IntroSpect) (*ConfigureSpec, error)
    ServeAndRecvSpec Function

    This function will serve the intro spect and return the configure spec See
    the ServeIntroSpect and RecvConfigureSpec for more details

type ControlStatusCode int

const (
	ControlStatusCode_CAPTURED  ControlStatusCode = 280 //Traffic captured by plugin, ask Zoraxy not to process the traffic
	ControlStatusCode_UNHANDLED ControlStatusCode = 284 //Traffic not handled by plugin, ask Zoraxy to process the traffic
	ControlStatusCode_ERROR     ControlStatusCode = 580 //Error occurred while processing the traffic, ask Zoraxy to process the traffic and log the error
)
type DynamicSniffForwardRequest struct {
	Method     string              `json:"method"`
	Hostname   string              `json:"hostname"`
	URL        string              `json:"url"`
	Header     map[string][]string `json:"header"`
	RemoteAddr string              `json:"remote_addr"`
	Host       string              `json:"host"`
	RequestURI string              `json:"request_uri"`
	Proto      string              `json:"proto"`
	ProtoMajor int                 `json:"proto_major"`
	ProtoMinor int                 `json:"proto_minor"`

	// Has unexported fields.
}
        Sniffing and forwarding

        The following functions are here to help with
        sniffing and forwarding requests to the dynamic
        router.

    A custom request object to be used in the dynamic sniffing

func DecodeForwardRequestPayload(jsonBytes []byte) (DynamicSniffForwardRequest, error)
    DecodeForwardRequestPayload decodes JSON bytes into a
    DynamicSniffForwardRequest object

func EncodeForwardRequestPayload(r *http.Request) DynamicSniffForwardRequest
    GetForwardRequestPayload returns a DynamicSniffForwardRequest object from an
    http.Request object

func (dsfr *DynamicSniffForwardRequest) GetRequest() *http.Request
    GetRequest returns the original http.Request object, for debugging purposes

func (dsfr *DynamicSniffForwardRequest) GetRequestUUID() string
    GetRequestUUID returns the request UUID if this UUID is empty string,
    that might indicate the request is not coming from the dynamic router

type IntroSpect struct {
	//  Plugin metadata
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

	//		Static Capture Settings
	//
	//		Once plugin is enabled these rules always applies to the enabled HTTP Proxy rule
	//		This is faster than dynamic capture, but less flexible

	StaticCapturePaths   []StaticCaptureRule `json:"static_capture_paths"`   //Static capture paths of your plugin, see Zoraxy documentation for more details
	StaticCaptureIngress string              `json:"static_capture_ingress"` //Static capture ingress path of your plugin (e.g. /s_handler)

	//		Dynamic Capture Settings
	//
	//		Once plugin is enabled, these rules will be captured and forward to plugin sniff
	//		if the plugin sniff returns 280, the traffic will be captured
	//		otherwise, the traffic will be forwarded to the next plugin
	//		This is slower than static capture, but more flexible

	DynamicCaptureSniff   string `json:"dynamic_capture_sniff"`   //Dynamic capture sniff path of your plugin (e.g. /d_sniff)
	DynamicCaptureIngress string `json:"dynamic_capture_ingress"` //Dynamic capture ingress path of your plugin (e.g. /d_handler)

	//  UI Path for your plugin
	UIPath string `json:"ui_path"` //UI path of your plugin (e.g. /ui), will proxy the whole subpath tree to Zoraxy Web UI as plugin UI

	//  Subscriptions Settings
	SubscriptionPath    string            `json:"subscription_path"`    //Subscription event path of your plugin (e.g. /notifyme), a POST request with SubscriptionEvent as body will be sent to this path when the event is triggered
	SubscriptionsEvents map[string]string `json:"subscriptions_events"` //Subscriptions events of your plugin, see Zoraxy documentation for more details
}
    IntroSpect Payload

    When the plugin is initialized with -introspect flag, the plugin shell
    return this payload as JSON and exit

type PathRouter struct {
	// Has unexported fields.
}

func NewPathRouter() *PathRouter
    NewPathRouter creates a new PathRouter

func (p *PathRouter) PrintRequestDebugMessage(r *http.Request)

func (p *PathRouter) RegisterDynamicCaptureHandle(capture_ingress string, mux *http.ServeMux, handlefunc func(http.ResponseWriter, *http.Request))
    RegisterDynamicCaptureHandle register the dynamic capture ingress path with
    a handler

func (p *PathRouter) RegisterDynamicSniffHandler(sniff_ingress string, mux *http.ServeMux, handler SniffHandler)
    RegisterDynamicSniffHandler registers a dynamic sniff handler for a path
    You can decide to accept or skip the request based on the request header and
    paths

func (p *PathRouter) RegisterPathHandler(path string, handler http.Handler)
    RegisterPathHandler registers a handler for a path

func (p *PathRouter) RegisterStaticCaptureHandle(capture_ingress string, mux *http.ServeMux)
    StartStaticCapture starts the static capture ingress

func (p *PathRouter) RemovePathHandler(path string)
    RemovePathHandler removes a handler for a path

func (p *PathRouter) SetDebugPrintMode(enable bool)
    SetDebugPrintMode sets the debug print mode

func (p *PathRouter) SetDefaultHandler(handler http.Handler)
    SetDefaultHandler sets the default handler for the router This handler will
    be called if no path handler is found

type PluginType int

const (
	PluginType_Router    PluginType = 0 //Router Plugin, used for handling / routing / forwarding traffic
	PluginType_Utilities PluginType = 1 //Utilities Plugin, used for utilities like Zerotier or Static Web Server that do not require interception with the dpcore
)
type PluginUiDebugRouter struct {
	PluginID      string //The ID of the plugin
	TargetDir     string //The directory where the UI files are stored
	HandlerPrefix string //The prefix of the handler used to route this router, e.g. /ui
	EnableDebug   bool   //Enable debug mode
	// Has unexported fields.
}

func NewPluginFileSystemUIRouter(pluginID string, targetDir string, handlerPrefix string) *PluginUiDebugRouter
    NewPluginFileSystemUIRouter creates a new PluginUiRouter with file system
    The targetDir is the directory where the UI files are stored (e.g. ./www)
    The handlerPrefix is the prefix of the handler used to route this router
    The handlerPrefix should start with a slash (e.g. /ui) that matches the
    http.Handle path All prefix should not end with a slash

func (p *PluginUiDebugRouter) AttachHandlerToMux(mux *http.ServeMux)
    Attach the file system UI handler to the target http.ServeMux

func (p *PluginUiDebugRouter) Handler() http.Handler
    GetHttpHandler returns the http.Handler for the PluginUiRouter

func (p *PluginUiDebugRouter) RegisterTerminateHandler(termFunc func(), mux *http.ServeMux)
    RegisterTerminateHandler registers the terminate handler for the
    PluginUiRouter The terminate handler will be called when the plugin is
    terminated from Zoraxy plugin manager if mux is nil, the handler will be
    registered to http.DefaultServeMux

type PluginUiRouter struct {
	PluginID       string    //The ID of the plugin
	TargetFs       *embed.FS //The embed.FS where the UI files are stored
	TargetFsPrefix string    //The prefix of the embed.FS where the UI files are stored, e.g. /web
	HandlerPrefix  string    //The prefix of the handler used to route this router, e.g. /ui
	EnableDebug    bool      //Enable debug mode
	// Has unexported fields.
}

func NewPluginEmbedUIRouter(pluginID string, targetFs *embed.FS, targetFsPrefix string, handlerPrefix string) *PluginUiRouter
    NewPluginEmbedUIRouter creates a new PluginUiRouter with embed.FS The
    targetFsPrefix is the prefix of the embed.FS where the UI files are stored
    The targetFsPrefix should be relative to the root of the embed.FS The
    targetFsPrefix should start with a slash (e.g. /web) that corresponds to the
    root folder of the embed.FS The handlerPrefix is the prefix of the handler
    used to route this router The handlerPrefix should start with a slash (e.g.
    /ui) that matches the http.Handle path All prefix should not end with a
    slash

func (p *PluginUiRouter) AttachHandlerToMux(mux *http.ServeMux)
    Attach the embed UI handler to the target http.ServeMux

func (p *PluginUiRouter) Handler() http.Handler
    GetHttpHandler returns the http.Handler for the PluginUiRouter

func (p *PluginUiRouter) RegisterTerminateHandler(termFunc func(), mux *http.ServeMux)
    RegisterTerminateHandler registers the terminate handler for the
    PluginUiRouter The terminate handler will be called when the plugin is
    terminated from Zoraxy plugin manager if mux is nil, the handler will be
    registered to http.DefaultServeMux

type RuntimeConstantValue struct {
	ZoraxyVersion    string `json:"zoraxy_version"`
	ZoraxyUUID       string `json:"zoraxy_uuid"`
	DevelopmentBuild bool   `json:"development_build"` //Whether the Zoraxy is a development build or not
}

type SniffHandler func(*DynamicSniffForwardRequest) SniffResult

type SniffResult int

const (
	SniffResultAccpet SniffResult = iota // Forward the request to this plugin dynamic capture ingress
	SniffResultSkip                      // Skip this plugin and let the next plugin handle the request
)
type StaticCaptureRule struct {
	CapturePath string `json:"capture_path"`
}

type SubscriptionEvent struct {
	EventName   string `json:"event_name"`
	EventSource string `json:"event_source"`
	Payload     string `json:"payload"` //Payload of the event, can be empty
}

</code></pre>
