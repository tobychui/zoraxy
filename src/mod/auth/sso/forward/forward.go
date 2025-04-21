package forward

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/utils"
)

type AuthRouterOptions struct {
	// Address of the forward auth endpoint.
	Address string

	// ResponseHeaders is a list of headers to be copied from the response if provided by the forward auth endpoint to
	// the request.
	ResponseHeaders []string

	// ResponseClientHeaders is a list of headers to be copied from the response if provided by the forward auth
	// endpoint to the response to the client.
	ResponseClientHeaders []string

	// RequestHeaders is a list of headers to be copied from the request to the authorization server. If empty all
	// headers are copied.
	RequestHeaders []string

	// RequestExcludedCookies is a list of cookie keys that should be removed from every request sent to the upstream.
	RequestExcludedCookies []string

	Logger   *logger.Logger
	Database *database.Database
}

type AuthRouter struct {
	client  *http.Client
	options *AuthRouterOptions
}

// NewAuthRouter creates a new AuthRouter object
func NewAuthRouter(options *AuthRouterOptions) *AuthRouter {
	options.Database.NewTable(DatabaseTable)

	//Read settings from database if available.
	options.Database.Read(DatabaseTable, DatabaseKeyAddress, &options.Address)

	responseHeaders, responseClientHeaders, requestHeaders, requestExcludedCookies := "", "", "", ""

	options.Database.Read(DatabaseTable, DatabaseKeyResponseHeaders, &responseHeaders)
	options.Database.Read(DatabaseTable, DatabaseKeyResponseClientHeaders, &responseClientHeaders)
	options.Database.Read(DatabaseTable, DatabaseKeyRequestHeaders, &requestHeaders)
	options.Database.Read(DatabaseTable, DatabaseKeyRequestExcludedCookies, &requestExcludedCookies)

	options.ResponseHeaders = strings.Split(responseHeaders, ",")
	options.ResponseClientHeaders = strings.Split(responseHeaders, ",")
	options.RequestHeaders = strings.Split(requestHeaders, ",")
	options.RequestExcludedCookies = strings.Split(requestExcludedCookies, ",")

	return &AuthRouter{
		client: &http.Client{
			CheckRedirect: func(r *http.Request, via []*http.Request) (err error) {
				return http.ErrUseLastResponse
			},
		},
		options: options,
	}
}

// HandleAPIOptions is the internal handler for setting the options.
func (ar *AuthRouter) HandleAPIOptions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		ar.handleOptionsGET(w, r)
	case http.MethodPost:
		ar.handleOptionsPOST(w, r)
	default:
		ar.handleOptionsMethodNotAllowed(w, r)
	}
}

func (ar *AuthRouter) handleOptionsGET(w http.ResponseWriter, r *http.Request) {
	js, _ := json.Marshal(map[string]interface{}{
		DatabaseKeyAddress:                ar.options.Address,
		DatabaseKeyResponseHeaders:        ar.options.ResponseHeaders,
		DatabaseKeyResponseClientHeaders:  ar.options.ResponseClientHeaders,
		DatabaseKeyRequestHeaders:         ar.options.RequestHeaders,
		DatabaseKeyRequestExcludedCookies: ar.options.RequestExcludedCookies,
	})

	utils.SendJSONResponse(w, string(js))

	return
}

func (ar *AuthRouter) handleOptionsPOST(w http.ResponseWriter, r *http.Request) {
	// Update the settings
	address, err := utils.PostPara(r, DatabaseKeyAddress)
	if err != nil {
		utils.SendErrorResponse(w, "address not found")

		return
	}

	// These are optional fields and can be empty strings.
	responseHeaders, _ := utils.PostPara(r, DatabaseKeyResponseHeaders)
	responseClientHeaders, _ := utils.PostPara(r, DatabaseKeyResponseClientHeaders)
	requestHeaders, _ := utils.PostPara(r, DatabaseKeyRequestHeaders)
	requestExcludedCookies, _ := utils.PostPara(r, DatabaseKeyRequestExcludedCookies)

	// Write changes to runtime
	ar.options.Address = address
	ar.options.ResponseHeaders = strings.Split(responseHeaders, ",")
	ar.options.ResponseClientHeaders = strings.Split(responseClientHeaders, ",")
	ar.options.RequestHeaders = strings.Split(requestHeaders, ",")
	ar.options.RequestExcludedCookies = strings.Split(requestExcludedCookies, ",")

	// Write changes to database
	ar.options.Database.Write(DatabaseTable, DatabaseKeyAddress, address)
	ar.options.Database.Write(DatabaseTable, DatabaseKeyResponseHeaders, responseHeaders)
	ar.options.Database.Write(DatabaseTable, DatabaseKeyResponseClientHeaders, responseClientHeaders)
	ar.options.Database.Write(DatabaseTable, DatabaseKeyRequestHeaders, requestHeaders)
	ar.options.Database.Write(DatabaseTable, DatabaseKeyRequestExcludedCookies, requestExcludedCookies)

	utils.SendOK(w)
}

func (ar *AuthRouter) handleOptionsMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)

	return
}

// HandleAuthProviderRouting is the internal handler for Forward Auth authentication.
func (ar *AuthRouter) HandleAuthProviderRouting(w http.ResponseWriter, r *http.Request) error {
	if ar.options.Address == "" {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		ar.options.Logger.PrintAndLog(LogTitle, "Address not set", nil)

		return ErrInternalServerError
	}

	// Make a request to Authz Server to verify the request
	req, err := http.NewRequest(http.MethodGet, ar.options.Address, nil)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		ar.options.Logger.PrintAndLog(LogTitle, "Unable to create request", err)

		return ErrInternalServerError
	}

	// TODO: Add opt-in support for copying the request body to the forward auth request.
	headerCopyIncluded(r.Header, req.Header, ar.options.RequestHeaders, true)

	// TODO: Add support for upstream headers.
	rSetForwardedHeaders(r, req)

	// Make the Authz Request.
	respForwarded, err := ar.client.Do(req)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		ar.options.Logger.PrintAndLog(LogTitle, "Unable to perform forwarded auth due to a request error", err)

		return ErrInternalServerError
	}

	defer respForwarded.Body.Close()

	body, err := io.ReadAll(respForwarded.Body)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		ar.options.Logger.PrintAndLog(LogTitle, "Unable to read response to forward auth request", err)

		return ErrInternalServerError
	}

	// Responses within the 200-299 range are considered successful and allow the proxy to handle the request.
	if respForwarded.StatusCode >= http.StatusOK && respForwarded.StatusCode < http.StatusMultipleChoices {
		if len(ar.options.ResponseClientHeaders) != 0 {
			headerCopyIncluded(respForwarded.Header, w.Header(), ar.options.ResponseClientHeaders, false)
		}

		if len(ar.options.RequestExcludedCookies) != 0 {
			// If the user has specified a list of cookies to be removed from the request, deterministically remove them.
			headerCookieRedact(r, ar.options.RequestExcludedCookies)
		}

		if len(ar.options.ResponseHeaders) != 0 {
			// Copy specific user-specified headers from the response of the forward auth request to the request sent to the
			// upstream server/next hop.
			headerCopyIncluded(respForwarded.Header, w.Header(), ar.options.ResponseHeaders, false)
		}

		return nil
	}

	// Copy the response.
	headerCopyExcluded(respForwarded.Header, w.Header(), nil)

	w.WriteHeader(respForwarded.StatusCode)
	if _, err = w.Write(body); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		ar.options.Logger.PrintAndLog(LogTitle, "Unable to write response", err)

		return ErrInternalServerError
	}

	return ErrUnauthorized
}

func scheme(r *http.Request) string {
	if r.TLS != nil {
		return "https"
	}

	return "http"
}

func headerCookieRedact(r *http.Request, excluded []string) {
	original := r.Cookies()

	if len(original) == 0 {
		return
	}

	var cookies []string

	for _, cookie := range original {
		if stringInSlice(cookie.Name, excluded) {
			continue
		}

		cookies = append(cookies, cookie.String())
	}

	r.Header.Set(HeaderCookie, strings.Join(cookies, "; "))
}

func headerCopyExcluded(original, destination http.Header, excludedHeaders []string) {
	for key, values := range original {
		// We should never copy the headers in the below list.
		if stringInSliceFold(key, doNotCopyHeaders) {
			continue
		}

		if stringInSliceFold(key, excludedHeaders) {
			continue
		}

		destination[key] = append(destination[key], values...)
	}
}

func headerCopyIncluded(original, destination http.Header, includedHeaders []string, allIfEmpty bool) {
	if allIfEmpty && len(includedHeaders) == 0 {
		headerCopyAll(original, destination)
	} else {
		headerCopyIncludedExact(original, destination, includedHeaders)
	}
}

func headerCopyAll(original, destination http.Header) {
	for key, values := range original {
		// We should never copy the headers in the below list, even if they're in the list provided by a user.
		if stringInSliceFold(key, doNotCopyHeaders) {
			continue
		}

		destination[key] = append(destination[key], values...)
	}
}

func headerCopyIncludedExact(original, destination http.Header, keys []string) {
	for _, key := range keys {
		// We should never copy the headers in the below list, even if they're in the list provided by a user.
		if stringInSliceFold(key, doNotCopyHeaders) {
			continue
		}

		if values, ok := original[key]; ok {
			destination[key] = append(destination[key], values...)
		}
	}
}

func stringInSlice(needle string, haystack []string) bool {
	if len(haystack) == 0 {
		return false
	}

	for _, v := range haystack {
		if needle == v {
			return true
		}
	}

	return false
}

func stringInSliceFold(needle string, haystack []string) bool {
	if len(haystack) == 0 {
		return false
	}

	for _, v := range haystack {
		if strings.EqualFold(needle, v) {
			return true
		}
	}

	return false
}

func rSetForwardedHeaders(r, req *http.Request) {
	if r.RemoteAddr != "" {
		before, _, _ := strings.Cut(r.RemoteAddr, ":")

		if ip := net.ParseIP(before); ip != nil {
			req.Header.Set(HeaderXForwardedFor, ip.String())
		}
	}

	req.Header.Set(HeaderXForwardedMethod, r.Method)
	req.Header.Set(HeaderXForwardedProto, scheme(r))
	req.Header.Set(HeaderXForwardedHost, r.Host)
	req.Header.Set(HeaderXForwardedURI, r.URL.Path)
}
