package errorpage

import (
	_ "embed"
)

/*
	Actions and Built-in Templates

	An action is a situation where Zoraxy generates the error
	response by itself (e.g. request blocked by whitelist) instead
	of forwarding the response from the upstream. Each action can
	be assigned a template in the template manager
*/

var (
	//go:embed templates/forbidden.html
	pageForbidden []byte
	//go:embed templates/hosterror.html
	pageHostError []byte
	//go:embed templates/rperror.html
	pageProxyError []byte
	//go:embed templates/generic.html
	pageGeneric []byte
)

type Action string

const (
	ActionBlacklist  Action = "blacklist" //Client IP is in the blacklist
	ActionWhitelist  Action = "whitelist" //Client IP is not in the whitelist
	ActionNotFound   Action = "notfound"  //Default site is set to 404 Not Found mode
	ActionHostError  Action = "hosterror" //Upstream not available or cannot be resolved
	ActionProxyError Action = "rperror"   //Unable to connect to the upstream
	ActionRateLimit  Action = "ratelimit" //Rate limit exceeded
	ActionVdirBlock  Action = "vdirblock" //Request matched a virtual directory in block access mode
)

// ActionInfo describes an action that can be assigned a template
type ActionInfo struct {
	ID             Action
	Name           string
	Description    string
	StatusCode     int    //Status code of the response, 0 if it is decided by the rule (e.g. vdir block)
	LegacyTemplate string //Template used when nothing is assigned, kept for backward compatibility
	Builtin        string //ID of the built-in template used when no template file is found
}

var actions = []*ActionInfo{
	{
		ID:             ActionBlacklist,
		Name:           "Blacklist Blocked",
		Description:    "Client IP or country is in the blacklist of the access rule",
		StatusCode:     403,
		LegacyTemplate: "blacklist.html",
		Builtin:        "forbidden",
	},
	{
		ID:             ActionWhitelist,
		Name:           "Whitelist Blocked",
		Description:    "Client IP or country is not in the whitelist of the access rule",
		StatusCode:     403,
		LegacyTemplate: "whitelist.html",
		Builtin:        "forbidden",
	},
	{
		ID:             ActionNotFound,
		Name:           "Default Site Not Found",
		Description:    "Default site is set to respond with 404 Not Found",
		StatusCode:     404,
		LegacyTemplate: "notfound.html",
		Builtin:        "hosterror",
	},
	{
		ID:             ActionHostError,
		Name:           "Upstream Not Found",
		Description:    "No upstream is available or the upstream hostname cannot be resolved",
		StatusCode:     404,
		LegacyTemplate: "hosterror.html",
		Builtin:        "hosterror",
	},
	{
		ID:             ActionProxyError,
		Name:           "Upstream Connection Error",
		Description:    "Zoraxy is unable to connect to the upstream server",
		StatusCode:     521,
		LegacyTemplate: "rperror.html",
		Builtin:        "rperror",
	},
	{
		ID:          ActionRateLimit,
		Name:        "Rate Limited",
		Description: "Client exceeded the request rate limit of the host",
		StatusCode:  429,
		Builtin:     "generic",
	},
	{
		ID:          ActionVdirBlock,
		Name:        "Virtual Directory Blocked",
		Description: "Request matched a virtual directory set to block access, status code is decided by the rule",
		StatusCode:  0,
		Builtin:     "generic",
	},
}

// BuiltinTemplate is a template embedded in the Zoraxy binary
type BuiltinTemplate struct {
	ID      string
	Name    string
	content []byte
}

var builtinTemplates = []*BuiltinTemplate{
	{ID: "generic", Name: "Generic Error Page", content: pageGeneric},
	{ID: "forbidden", Name: "Forbidden", content: pageForbidden},
	{ID: "hosterror", Name: "Host Not Found", content: pageHostError},
	{ID: "rperror", Name: "Reverse Proxy Error", content: pageProxyError},
}

// Templates that are not error pages but stored in the same folder
var otherSystemTemplates = map[string]string{
	"captcha_cloudflare.html":   "CAPTCHA Challenge (Cloudflare Turnstile)",
	"captcha_recaptcha_v2.html": "CAPTCHA Challenge (reCAPTCHA v2)",
	"captcha_recaptcha_v3.html": "CAPTCHA Challenge (reCAPTCHA v3)",
}

// Get the info of an action, return nil if the action is not defined
func GetActionInfo(action Action) *ActionInfo {
	for _, info := range actions {
		if info.ID == action {
			return info
		}
	}
	return nil
}

// Get the content of a built-in template, fallback to the generic template if id not found
func GetBuiltinTemplate(id string) []byte {
	for _, t := range builtinTemplates {
		if t.ID == id {
			return t.content
		}
	}
	return pageGeneric
}

// Get the built-in fallback template of an action
func getActionBuiltin(info *ActionInfo) []byte {
	if info == nil {
		return pageGeneric
	}
	return GetBuiltinTemplate(info.Builtin)
}
