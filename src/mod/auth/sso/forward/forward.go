package forward

import (
	"encoding/json"
	"io"
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

	// RequestIncludedCookies is a list of cookie keys that if defined will be the only cookies sent in the request to
	// the authorization server.
	RequestIncludedCookies []string

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

	responseHeaders, responseClientHeaders, requestHeaders, requestIncludedCookies, requestExcludedCookies := "", "", "", "", ""

	options.Database.Read(DatabaseTable, DatabaseKeyResponseHeaders, &responseHeaders)
	options.Database.Read(DatabaseTable, DatabaseKeyResponseClientHeaders, &responseClientHeaders)
	options.Database.Read(DatabaseTable, DatabaseKeyRequestHeaders, &requestHeaders)
	options.Database.Read(DatabaseTable, DatabaseKeyRequestIncludedCookies, &requestIncludedCookies)
	options.Database.Read(DatabaseTable, DatabaseKeyRequestExcludedCookies, &requestExcludedCookies)

	options.ResponseHeaders = strings.Split(responseHeaders, ",")
	options.ResponseClientHeaders = strings.Split(responseClientHeaders, ",")
	options.RequestHeaders = strings.Split(requestHeaders, ",")
	options.RequestIncludedCookies = strings.Split(requestIncludedCookies, ",")
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
	js, _ := json.Marshal(map[string]any{
		DatabaseKeyAddress:                ar.options.Address,
		DatabaseKeyResponseHeaders:        ar.options.ResponseHeaders,
		DatabaseKeyResponseClientHeaders:  ar.options.ResponseClientHeaders,
		DatabaseKeyRequestHeaders:         ar.options.RequestHeaders,
		DatabaseKeyRequestIncludedCookies: ar.options.RequestIncludedCookies,
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
	requestIncludedCookies, _ := utils.PostPara(r, DatabaseKeyRequestIncludedCookies)
	requestExcludedCookies, _ := utils.PostPara(r, DatabaseKeyRequestExcludedCookies)

	// Write changes to runtime
	ar.options.Address = address
	ar.options.ResponseHeaders = strings.Split(responseHeaders, ",")
	ar.options.ResponseClientHeaders = strings.Split(responseClientHeaders, ",")
	ar.options.RequestHeaders = strings.Split(requestHeaders, ",")
	ar.options.RequestIncludedCookies = strings.Split(requestIncludedCookies, ",")
	ar.options.RequestExcludedCookies = strings.Split(requestExcludedCookies, ",")

	// Write changes to database
	ar.options.Database.Write(DatabaseTable, DatabaseKeyAddress, address)
	ar.options.Database.Write(DatabaseTable, DatabaseKeyResponseHeaders, responseHeaders)
	ar.options.Database.Write(DatabaseTable, DatabaseKeyResponseClientHeaders, responseClientHeaders)
	ar.options.Database.Write(DatabaseTable, DatabaseKeyRequestHeaders, requestHeaders)
	ar.options.Database.Write(DatabaseTable, DatabaseKeyRequestIncludedCookies, requestIncludedCookies)
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
		return ar.handle500Error(w, nil, "Address not set")
	}

	// Make a request to Authz Server to verify the request
	// TODO: Add opt-in support for copying the request body to the forward auth request. Currently it's just an
	//       empty body which is usually fine in most instances. It's likely best to see if anyone wants this feature
	//       as I'm unaware of any specific forward auth implementation that needs it.
	req, err := http.NewRequest(http.MethodGet, ar.options.Address, nil)
	if err != nil {
		return ar.handle500Error(w, err, "Unable to create request")
	}

	headerCopyIncluded(r.Header, req.Header, ar.options.RequestHeaders, true)
	headerCookieRedact(r, ar.options.RequestIncludedCookies, false)

	// TODO: Add support for headers from upstream proxies. This will likely involve implementing some form of
	//       proxy specific trust system within Zoraxy.
	rSetForwardedHeaders(r, req)

	// Make the Authz Request.
	respForwarded, err := ar.client.Do(req)
	if err != nil {
		return ar.handle500Error(w, err, "Unable to perform forwarded auth due to a request error")
	}

	defer respForwarded.Body.Close()

	// Responses within the 200-299 range are considered successful and allow the proxy to handle the request.
	if respForwarded.StatusCode >= http.StatusOK && respForwarded.StatusCode < http.StatusMultipleChoices {
		if len(ar.options.ResponseClientHeaders) != 0 {
			headerCopyIncluded(respForwarded.Header, w.Header(), ar.options.ResponseClientHeaders, false)
		}

		headerCookieRedact(r, ar.options.RequestExcludedCookies, true)

		if len(ar.options.ResponseHeaders) != 0 {
			// Copy specific user-specified headers from the response of the forward auth request to the request sent to the
			// upstream server/next hop.
			headerCopyIncluded(respForwarded.Header, w.Header(), ar.options.ResponseHeaders, false)
		}

		// Return the request to the proxy for forwarding to the backend.
		return nil
	}

	// Copy the unsuccessful response.
	headerCopyExcluded(respForwarded.Header, w.Header(), nil)

	w.WriteHeader(respForwarded.StatusCode)

	body, err := io.ReadAll(respForwarded.Body)
	if err != nil {
		return ar.handle500Error(w, err, "Unable to read response to forward auth request")
	}

	if _, err = w.Write(body); err != nil {
		return ar.handle500Error(w, err, "Unable to write response")
	}

	return ErrUnauthorized
}

// handle500Error is func intended on factorizing a commonly repeated functional flow within this provider.
func (ar *AuthRouter) handle500Error(w http.ResponseWriter, err error, message string) error {
	http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

	ar.options.Logger.PrintAndLog(LogTitle, message, err)

	return ErrInternalServerError
}
