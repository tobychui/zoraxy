package rewrite

import (
	"fmt"
	"net"
	"net/http"
	"strings"
)

// GetHeaderVariableValuesFromRequest returns a map of header variables and their values
// note that variables behavior is not exactly identical to nginx variables
func GetHeaderVariableValuesFromRequest(r *http.Request) map[string]string {
	vars := make(map[string]string)

	// Request-specific variables
	vars["$host"] = r.Host
	vars["$remote_addr"] = r.RemoteAddr
	remoteIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		remoteIP = r.RemoteAddr // Fallback to the full RemoteAddr if parsing fails
	}
	vars["$remote_ip"] = remoteIP
	vars["$request_uri"] = r.RequestURI
	vars["$request_method"] = r.Method
	vars["$content_length"] = fmt.Sprintf("%d", r.ContentLength)
	vars["$content_type"] = r.Header.Get("Content-Type")

	// Parsed URI elements
	vars["$uri"] = r.URL.Path
	vars["$args"] = r.URL.RawQuery
	vars["$scheme"] = r.URL.Scheme
	vars["$query_string"] = r.URL.RawQuery

	// User agent and referer
	vars["$http_user_agent"] = r.UserAgent()
	vars["$http_referer"] = r.Referer()

	return vars
}

// CustomHeadersIncludeDynamicVariables checks if the user-defined headers contain dynamic variables
// use for early exit when processing the headers
func CustomHeadersIncludeDynamicVariables(userDefinedHeaders []*UserDefinedHeader) bool {
	for _, header := range userDefinedHeaders {
		if strings.Contains(header.Value, "$") {
			return true
		}
	}
	return false
}

// PopulateRequestHeaderVariables populates the user-defined headers with the values from the request
func PopulateRequestHeaderVariables(r *http.Request, userDefinedHeaders []*UserDefinedHeader) []*UserDefinedHeader {
	if !CustomHeadersIncludeDynamicVariables(userDefinedHeaders) {
		// Early exit if there are no dynamic variables
		return userDefinedHeaders
	}
	vars := GetHeaderVariableValuesFromRequest(r)
	populatedHeaders := []*UserDefinedHeader{}
	// Populate the user-defined headers with the values from the request
	for _, header := range userDefinedHeaders {
		thisHeader := header.Copy()
		for key, value := range vars {
			thisHeader.Value = strings.ReplaceAll(thisHeader.Value, key, value)
		}
		populatedHeaders = append(populatedHeaders, thisHeader)
	}
	return populatedHeaders
}
