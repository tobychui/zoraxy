package permissionpolicy

import (
	"net/http"
	"strings"
)

/*
	Content Security Policy

	This is a content security policy header modifier that changes
	the request content security policy fields

	author: tobychui

	//TODO: intergrate this with the dynamic proxy module
*/

type ContentSecurityPolicy struct {
	DefaultSrc              []string `json:"default_src"`
	ScriptSrc               []string `json:"script_src"`
	StyleSrc                []string `json:"style_src"`
	ImgSrc                  []string `json:"img_src"`
	ConnectSrc              []string `json:"connect_src"`
	FontSrc                 []string `json:"font_src"`
	ObjectSrc               []string `json:"object_src"`
	MediaSrc                []string `json:"media_src"`
	FrameSrc                []string `json:"frame_src"`
	WorkerSrc               []string `json:"worker_src"`
	ChildSrc                []string `json:"child_src"`
	ManifestSrc             []string `json:"manifest_src"`
	PrefetchSrc             []string `json:"prefetch_src"`
	FormAction              []string `json:"form_action"`
	FrameAncestors          []string `json:"frame_ancestors"`
	BaseURI                 []string `json:"base_uri"`
	Sandbox                 []string `json:"sandbox"`
	ReportURI               []string `json:"report_uri"`
	ReportTo                []string `json:"report_to"`
	UpgradeInsecureRequests bool     `json:"upgrade_insecure_requests"`
	BlockAllMixedContent    bool     `json:"block_all_mixed_content"`
}

// GetDefaultContentSecurityPolicy returns a ContentSecurityPolicy struct with default permissive settings
func GetDefaultContentSecurityPolicy() *ContentSecurityPolicy {
	return &ContentSecurityPolicy{
		DefaultSrc:              []string{"*"},
		ScriptSrc:               []string{"*"},
		StyleSrc:                []string{"*"},
		ImgSrc:                  []string{"*"},
		ConnectSrc:              []string{"*"},
		FontSrc:                 []string{"*"},
		ObjectSrc:               []string{"*"},
		MediaSrc:                []string{"*"},
		FrameSrc:                []string{"*"},
		WorkerSrc:               []string{"*"},
		ChildSrc:                []string{"*"},
		ManifestSrc:             []string{"*"},
		PrefetchSrc:             []string{"*"},
		FormAction:              []string{"*"},
		FrameAncestors:          []string{"*"},
		BaseURI:                 []string{"*"},
		Sandbox:                 []string{},
		ReportURI:               []string{},
		ReportTo:                []string{},
		UpgradeInsecureRequests: false,
		BlockAllMixedContent:    false,
	}
}

// ToHeader converts a ContentSecurityPolicy struct into a CSP header key-value pair
func (csp *ContentSecurityPolicy) ToHeader() []string {
	directives := []string{}

	addDirective := func(name string, sources []string) {
		if len(sources) > 0 {
			directives = append(directives, name+" "+strings.Join(sources, " "))
		}
	}

	addDirective("default-src", csp.DefaultSrc)
	addDirective("script-src", csp.ScriptSrc)
	addDirective("style-src", csp.StyleSrc)
	addDirective("img-src", csp.ImgSrc)
	addDirective("connect-src", csp.ConnectSrc)
	addDirective("font-src", csp.FontSrc)
	addDirective("object-src", csp.ObjectSrc)
	addDirective("media-src", csp.MediaSrc)
	addDirective("frame-src", csp.FrameSrc)
	addDirective("worker-src", csp.WorkerSrc)
	addDirective("child-src", csp.ChildSrc)
	addDirective("manifest-src", csp.ManifestSrc)
	addDirective("prefetch-src", csp.PrefetchSrc)
	addDirective("form-action", csp.FormAction)
	addDirective("frame-ancestors", csp.FrameAncestors)
	addDirective("base-uri", csp.BaseURI)
	if len(csp.Sandbox) > 0 {
		directives = append(directives, "sandbox "+strings.Join(csp.Sandbox, " "))
	}
	if len(csp.ReportURI) > 0 {
		addDirective("report-uri", csp.ReportURI)
	}
	if len(csp.ReportTo) > 0 {
		addDirective("report-to", csp.ReportTo)
	}
	if csp.UpgradeInsecureRequests {
		directives = append(directives, "upgrade-insecure-requests")
	}
	if csp.BlockAllMixedContent {
		directives = append(directives, "block-all-mixed-content")
	}

	headerValue := strings.Join(directives, "; ")
	return []string{"Content-Security-Policy", headerValue}
}

// InjectContentSecurityPolicyHeader injects the CSP header into the response
func InjectContentSecurityPolicyHeader(w http.ResponseWriter, csp *ContentSecurityPolicy) {
	if csp == nil || w.Header().Get("Content-Security-Policy") != "" {
		return
	}
	headerKV := csp.ToHeader()
	w.Header().Set(headerKV[0], headerKV[1])
}
