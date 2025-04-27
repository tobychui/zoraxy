package main

/*
	Type and flag definations

	This file contains all the type and flag definations
	Author: tobychui
*/

import (
	"embed"
	"flag"
	"net/http"
	"time"

	"imuslab.com/zoraxy/mod/auth/sso/authentik"

	"imuslab.com/zoraxy/mod/access"
	"imuslab.com/zoraxy/mod/acme"
	"imuslab.com/zoraxy/mod/auth"
	"imuslab.com/zoraxy/mod/auth/sso/authelia"
	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/dockerux"
	"imuslab.com/zoraxy/mod/dynamicproxy/loadbalance"
	"imuslab.com/zoraxy/mod/dynamicproxy/redirection"
	"imuslab.com/zoraxy/mod/email"
	"imuslab.com/zoraxy/mod/forwardproxy"
	"imuslab.com/zoraxy/mod/geodb"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/info/logviewer"
	"imuslab.com/zoraxy/mod/mdns"
	"imuslab.com/zoraxy/mod/netstat"
	"imuslab.com/zoraxy/mod/pathrule"
	"imuslab.com/zoraxy/mod/plugins"
	"imuslab.com/zoraxy/mod/sshprox"
	"imuslab.com/zoraxy/mod/statistic"
	"imuslab.com/zoraxy/mod/statistic/analytic"
	"imuslab.com/zoraxy/mod/streamproxy"
	"imuslab.com/zoraxy/mod/tlscert"
	"imuslab.com/zoraxy/mod/uptime"
	"imuslab.com/zoraxy/mod/webserv"
)

const (
	/* Build Constants */
	SYSTEM_NAME    = "Zoraxy"
	SYSTEM_VERSION = "3.2.1"

	/* System Constants */
	TMP_FOLDER                   = "./tmp"
	WEBSERV_DEFAULT_PORT         = 5487
	MDNS_HOSTNAME_PREFIX         = "zoraxy_" /* Follow by node UUID */
	MDNS_IDENTIFY_DEVICE_TYPE    = "Network Gateway"
	MDNS_IDENTIFY_DOMAIN         = "zoraxy.aroz.org"
	MDNS_IDENTIFY_VENDOR         = "imuslab.com"
	MDNS_SCAN_TIMEOUT            = 30 /* Seconds */
	MDNS_SCAN_UPDATE_INTERVAL    = 15 /* Minutes */
	GEODB_CACHE_CLEAR_INTERVAL   = 15 /* Minutes */
	ACME_AUTORENEW_CONFIG_PATH   = "./conf/acme_conf.json"
	CSRF_COOKIENAME              = "zoraxy_csrf"
	LOG_PREFIX                   = "zr"
	LOG_EXTENSION                = ".log"
	STATISTIC_AUTO_SAVE_INTERVAL = 600 /* Seconds */

	/* Configuration Folder Storage Path Constants */
	CONF_HTTP_PROXY    = "./conf/proxy"
	CONF_STREAM_PROXY  = "./conf/streamproxy"
	CONF_CERT_STORE    = "./conf/certs"
	CONF_REDIRECTION   = "./conf/redirect"
	CONF_ACCESS_RULE   = "./conf/access"
	CONF_PATH_RULE     = "./conf/rules/pathrules"
	CONF_PLUGIN_GROUPS = "./conf/plugin_groups.json"
)

/* System Startup Flags */
var (
	webUIPort                  = flag.String("port", ":8000", "Management web interface listening port")
	databaseBackend            = flag.String("db", "auto", "Database backend to use (leveldb, boltdb, auto) Note that fsdb will be used on unsupported platforms like RISCV")
	noauth                     = flag.Bool("noauth", false, "Disable authentication for management interface")
	showver                    = flag.Bool("version", false, "Show version of this server")
	allowSshLoopback           = flag.Bool("sshlb", false, "Allow loopback web ssh connection (DANGER)")
	allowMdnsScanning          = flag.Bool("mdns", true, "Enable mDNS scanner and transponder")
	mdnsName                   = flag.String("mdnsname", "", "mDNS name, leave empty to use default (zoraxy_{node-uuid}.local)")
	runningInDocker            = flag.Bool("docker", false, "Run Zoraxy in docker compatibility mode")
	acmeAutoRenewInterval      = flag.Int("autorenew", 86400, "ACME auto TLS/SSL certificate renew check interval (seconds)")
	acmeCertAutoRenewDays      = flag.Int("earlyrenew", 30, "Number of days to early renew a soon expiring certificate (days)")
	enableHighSpeedGeoIPLookup = flag.Bool("fastgeoip", false, "Enable high speed geoip lookup, require 1GB extra memory (Not recommend for low end devices)")
	allowWebFileManager        = flag.Bool("webfm", true, "Enable web file manager for static web server root folder")
	enableAutoUpdate           = flag.Bool("cfgupgrade", true, "Enable auto config upgrade if breaking change is detected")

	/* Default Configuration Flags */
	defaultInboundPort          = flag.Int("default_inbound_port", 443, "Default web server listening port")
	defaultEnableInboundTraffic = flag.Bool("default_inbound_enabled", true, "If web server is enabled by default")

	/* Path Configuration Flags */
	//path_database  = flag.String("dbpath", "./sys.db", "Database path")
	//path_conf      = flag.String("conf", "./conf", "Configuration folder path")
	path_uuid      = flag.String("uuid", "./sys.uuid", "sys.uuid file path")
	path_logFile   = flag.String("log", "./log", "Log folder path")
	path_webserver = flag.String("webroot", "./www", "Static web server root folder. Only allow change in start paramters")
	path_plugin    = flag.String("plugin", "./plugins", "Plugin folder path")

	/* Maintaince & Development Function Flags */
	geoDbUpdate       = flag.Bool("update_geoip", false, "Download the latest GeoIP data and exit")
	development_build = flag.Bool("dev", false, "Use external web folder for UI development")
)

/* Global Variables and Handlers */
var (
	nodeUUID    = "generic" //System uuid in uuidv4 format, load from database on startup
	bootTime    = time.Now().Unix()
	requireAuth = true //Require authentication for webmin panel, override from flag

	/*
		Binary Embedding File System
	*/
	//go:embed web/*
	webres embed.FS

	/*
		Handler Modules
	*/
	sysdb          *database.Database              //System database
	authAgent      *auth.AuthAgent                 //Authentication agent
	tlsCertManager *tlscert.Manager                //TLS / SSL management
	redirectTable  *redirection.RuleTable          //Handle special redirection rule sets
	webminPanelMux *http.ServeMux                  //Server mux for handling webmin panel APIs
	csrfMiddleware func(http.Handler) http.Handler //CSRF protection middleware

	pathRuleHandler    *pathrule.Handler         //Handle specific path blocking or custom headers
	geodbStore         *geodb.Store              //GeoIP database, for resolving IP into country code
	accessController   *access.Controller        //Access controller, handle black list and white list
	netstatBuffers     *netstat.NetStatBuffers   //Realtime graph buffers
	statisticCollector *statistic.Collector      //Collecting statistic from visitors
	uptimeMonitor      *uptime.Monitor           //Uptime monitor service worker
	mdnsScanner        *mdns.MDNSHost            //mDNS discovery services
	webSshManager      *sshprox.Manager          //Web SSH connection service
	streamProxyManager *streamproxy.Manager      //Stream Proxy Manager for TCP / UDP forwarding
	acmeHandler        *acme.ACMEHandler         //Handler for ACME Certificate renew
	acmeAutoRenewer    *acme.AutoRenewer         //Handler for ACME auto renew ticking
	staticWebServer    *webserv.WebServer        //Static web server for hosting simple stuffs
	forwardProxy       *forwardproxy.Handler     //HTTP Forward proxy, basically VPN for web browser
	loadBalancer       *loadbalance.RouteManager //Global scope loadbalancer, store the state of the lb routing
	pluginManager      *plugins.Manager          //Plugin manager for managing plugins

	//Authentication Provider
	autheliaRouter  *authelia.AutheliaRouter   //Authelia router for Authelia authentication
	authentikRouter *authentik.AuthentikRouter //Authentik router for Authentik authentication

	//Helper modules
	EmailSender       *email.Sender         //Email sender that handle email sending
	AnalyticLoader    *analytic.DataLoader  //Data loader for Zoraxy Analytic
	DockerUXOptimizer *dockerux.UXOptimizer //Docker user experience optimizer, community contribution only
	SystemWideLogger  *logger.Logger        //Logger for Zoraxy
	LogViewer         *logviewer.Viewer     //Log viewer HTTP handlers
)
