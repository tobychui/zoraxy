package dynamicproxy

import (
	"errors"
	"net/http"
	"sort"
	"strings"

	"imuslab.com/zoraxy/mod/dynamicproxy/errorpage"
)

/*
	methodfilter.go

	Per-endpoint allow-list of HTTP methods.

	When an endpoint lists its allowed methods, a request made with
	any other method is answered with 405 Method Not Allowed and is
	never forwarded to the upstream or to a virtual directory. An
	endpoint with an empty list accepts every method.
*/

// HTTPMethod is an HTTP request method. Allow-lists accept only the
// methods of the IANA HTTP Method Registry
// (https://www.iana.org/assignments/http-methods).
type HTTPMethod string

// Methods defined by RFC 9110 and RFC 5789, same values as net/http
const (
	MethodGet     HTTPMethod = http.MethodGet
	MethodHead    HTTPMethod = http.MethodHead
	MethodPost    HTTPMethod = http.MethodPost
	MethodPut     HTTPMethod = http.MethodPut
	MethodPatch   HTTPMethod = http.MethodPatch
	MethodDelete  HTTPMethod = http.MethodDelete
	MethodConnect HTTPMethod = http.MethodConnect
	MethodOptions HTTPMethod = http.MethodOptions
	MethodTrace   HTTPMethod = http.MethodTrace
)

// Other methods of the IANA HTTP Method Registry (WebDAV, CalDAV,
// versioning, bindings, ...), which have no constant in net/http
const (
	MethodACL               HTTPMethod = "ACL"               //RFC 3744
	MethodBaselineControl   HTTPMethod = "BASELINE-CONTROL"  //RFC 3253
	MethodBind              HTTPMethod = "BIND"              //RFC 5842
	MethodCheckin           HTTPMethod = "CHECKIN"           //RFC 3253
	MethodCheckout          HTTPMethod = "CHECKOUT"          //RFC 3253
	MethodCopy              HTTPMethod = "COPY"              //RFC 4918
	MethodLabel             HTTPMethod = "LABEL"             //RFC 3253
	MethodLink              HTTPMethod = "LINK"              //RFC 2068
	MethodLock              HTTPMethod = "LOCK"              //RFC 4918
	MethodMerge             HTTPMethod = "MERGE"             //RFC 3253
	MethodMkactivity        HTTPMethod = "MKACTIVITY"        //RFC 3253
	MethodMkcalendar        HTTPMethod = "MKCALENDAR"        //RFC 4791
	MethodMkcol             HTTPMethod = "MKCOL"             //RFC 4918
	MethodMkredirectref     HTTPMethod = "MKREDIRECTREF"     //RFC 4437
	MethodMkworkspace       HTTPMethod = "MKWORKSPACE"       //RFC 3253
	MethodMove              HTTPMethod = "MOVE"              //RFC 4918
	MethodOrderpatch        HTTPMethod = "ORDERPATCH"        //RFC 3648
	MethodPropfind          HTTPMethod = "PROPFIND"          //RFC 4918
	MethodProppatch         HTTPMethod = "PROPPATCH"         //RFC 4918
	MethodQuery             HTTPMethod = "QUERY"             //RFC 10008
	MethodRebind            HTTPMethod = "REBIND"            //RFC 5842
	MethodReport            HTTPMethod = "REPORT"            //RFC 3253
	MethodSearch            HTTPMethod = "SEARCH"            //RFC 5323
	MethodUnbind            HTTPMethod = "UNBIND"            //RFC 5842
	MethodUncheckout        HTTPMethod = "UNCHECKOUT"        //RFC 3253
	MethodUnlink            HTTPMethod = "UNLINK"            //RFC 2068
	MethodUnlock            HTTPMethod = "UNLOCK"            //RFC 4918
	MethodUpdate            HTTPMethod = "UPDATE"            //RFC 3253
	MethodUpdateredirectref HTTPMethod = "UPDATEREDIRECTREF" //RFC 4437
	MethodVersionControl    HTTPMethod = "VERSION-CONTROL"   //RFC 3253
)

// RegisteredHTTPMethods lists the methods that can be used in an allow-list:
// the IANA HTTP Method Registry, without the two entries that are never the
// method of a proxied request ("PRI", the HTTP/2 connection preface, and "*")
var RegisteredHTTPMethods = []HTTPMethod{
	MethodACL, MethodBaselineControl, MethodBind, MethodCheckin, MethodCheckout,
	MethodConnect, MethodCopy, MethodDelete, MethodGet, MethodHead, MethodLabel,
	MethodLink, MethodLock, MethodMerge, MethodMkactivity, MethodMkcalendar,
	MethodMkcol, MethodMkredirectref, MethodMkworkspace, MethodMove,
	MethodOptions, MethodOrderpatch, MethodPatch, MethodPost, MethodPropfind,
	MethodProppatch, MethodPut, MethodQuery, MethodRebind, MethodReport,
	MethodSearch, MethodTrace, MethodUnbind, MethodUncheckout, MethodUnlink,
	MethodUnlock, MethodUpdate, MethodUpdateredirectref, MethodVersionControl,
}

// IsRegistered checks if the method is in RegisteredHTTPMethods
func (m HTTPMethod) IsRegistered() bool {
	for _, registeredMethod := range RegisteredHTTPMethods {
		if m == registeredMethod {
			return true
		}
	}
	return false
}

// JoinHTTPMethods joins the methods with the given separator,
// e.g. for the Allow header
func JoinHTTPMethods(methods []HTTPMethod, sep string) string {
	names := make([]string, len(methods))
	for i, method := range methods {
		names[i] = string(method)
	}
	return strings.Join(names, sep)
}

// ParseAllowedMethods converts a list of HTTP methods separated by comma or
// whitespace (e.g. "GET, HEAD, POST") into a sorted list without duplicates.
// Methods are converted to upper case and must be in the IANA HTTP Method Registry.
// An empty input returns an empty list, which means all methods are allowed.
func ParseAllowedMethods(raw string) ([]HTTPMethod, error) {
	methods := []HTTPMethod{}
	seen := map[HTTPMethod]bool{}
	for _, field := range strings.FieldsFunc(raw, func(c rune) bool {
		return c == ',' || c == ' ' || c == '\t' || c == '\n' || c == '\r'
	}) {
		method := HTTPMethod(strings.ToUpper(field))
		if !method.IsRegistered() {
			return nil, errors.New("unsupported HTTP method: " + field + " (not in the IANA HTTP Method Registry)")
		}
		if !seen[method] {
			seen[method] = true
			methods = append(methods, method)
		}
	}
	sort.Slice(methods, func(i, j int) bool { return methods[i] < methods[j] })
	return methods, nil
}

// IsMethodAllowed checks if a request with the given method can be served by
// this endpoint. Methods are case-sensitive (RFC 9110 section 9.1).
func (ep *ProxyEndpoint) IsMethodAllowed(method HTTPMethod) bool {
	if len(ep.AllowedMethods) == 0 {
		//No allow-list set, all methods are allowed
		return true
	}
	for _, allowedMethod := range ep.AllowedMethods {
		if allowedMethod == method {
			return true
		}
	}
	return false
}

// handleMethodNotAllowed answers a request whose method is not in the
// allow-list of the endpoint
func (h *ProxyHandler) handleMethodNotAllowed(w http.ResponseWriter, r *http.Request, pe *ProxyEndpoint) {
	w.Header().Set("Allow", JoinHTTPMethods(pe.AllowedMethods, ", "))
	h.Parent.errorPages().ServeErrorPage(w, errorpage.ActionMethodNotAllowed, http.StatusMethodNotAllowed)
	h.Parent.logRequest(r, false, http.StatusMethodNotAllowed, "method-blocked", r.Host, "blocked", pe)
}
