package main

import (
	"net/http"
	"strings"

	"imuslab.com/zoraxy/mod/dynamicproxy"
)

/*
  Routing Rule

  This script handle special routing rules for some utilities functions
*/

// Register the system build-in routing rules into the core
func registerBuildInRoutingRules() {
	//Cloudflare email decoder
	//It decode the email address if you are proxying a cloudflare protected site
	//[email-protected] -> real@email.com
	dynamicProxyRouter.AddRoutingRules(&dynamicproxy.RoutingRule{
		ID: "cloudflare-decoder",
		MatchRule: func(r *http.Request) bool {
			return strings.HasSuffix(r.RequestURI, "cloudflare-static/email-decode.min.js")
		},
		RoutingHandler: func(w http.ResponseWriter, r *http.Request) {
			decoder := "function fixObfuscatedEmails(){let t=document.getElementsByClassName(\"__cf_email__\");for(let e=0;e<t.length;e++){let r=t[e],l=r.getAttribute(\"data-cfemail\");if(l){let a=decrypt(l);r.setAttribute(\"href\",\"mailto:\"+a),r.innerHTML=a}}}function decrypt(t){let e=\"\",r=parseInt(t.substr(0,2),16);for(let l=2;l<t.length;l+=2){let a=parseInt(t.substr(l,2),16)^r;e+=String.fromCharCode(a)}try{e=decodeURIComponent(escape(e))}catch(f){console.error(f)}return e}fixObfuscatedEmails();"
			w.Header().Set("Content-type", "text/javascript")
			w.Write([]byte(decoder))
		},
		Enabled:                false,
		UseSystemAccessControl: false,
	})

}
