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
	options.Database.Read(DatabaseTable, DatabaseKeyResponseHeaders, &options.ResponseHeaders)

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
		ar.OptionsMethodNotAllowedHandleFunc(w, r)
	}
}

func (ar *AuthRouter) handleOptionsGET(w http.ResponseWriter, r *http.Request) {
	js, _ := json.Marshal(map[string]interface{}{
		DatabaseKeyAddress:         ar.options.Address,
		DatabaseKeyResponseHeaders: ar.options.ResponseHeaders,
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

	responseHeaders, err := utils.PostPara(r, DatabaseKeyResponseHeaders)
	if err != nil {
		utils.SendErrorResponse(w, "address not found")

		return
	}

	// Write changes to runtime
	ar.options.Address = address
	ar.options.ResponseHeaders = strings.Split(responseHeaders, ",")

	// Write changes to database
	ar.options.Database.Write(DatabaseTable, DatabaseKeyAddress, address)
	ar.options.Database.Write(DatabaseTable, DatabaseKeyResponseHeaders, responseHeaders)

	utils.SendOK(w)
}

func (ar *AuthRouter) OptionsMethodNotAllowedHandleFunc(w http.ResponseWriter, r *http.Request) {
	http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)

	return
}

// HandleAuthProviderRouting is the internal handler for Forward Auth authentication.
func (ar *AuthRouter) HandleAuthProviderRouting(w http.ResponseWriter, r *http.Request) error {
	if ar.options.Address == "" {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		ar.options.Logger.PrintAndLog("Forward Auth", "Address not set", nil)

		return ErrInternalServerError
	}

	// Make a request to Authz Server to verify the request
	req, err := http.NewRequest(http.MethodGet, ar.options.Address, nil)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		ar.options.Logger.PrintAndLog("Forward Auth", "Unable to create request", err)

		return ErrInternalServerError
	}

	headerCopyExcluded(r.Header, req.Header, doNotCopyHeaders)

	// TODO: Add support for upstream headers.
	req.Header.Set(HeaderXForwardedMethod, r.Method)
	req.Header.Set(HeaderXForwardedProto, scheme(r))
	req.Header.Set(HeaderXForwardedHost, r.Host)
	req.Header.Set(HeaderXForwardedURI, r.URL.Path)
	req.Header.Set(HeaderXForwardedFor, r.RemoteAddr)

	// Make the Authz Request.
	respForwarded, err := ar.client.Do(req)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		ar.options.Logger.PrintAndLog("Forward Auth", "Unable to perform forwarded auth due to a request error", err)

		return ErrInternalServerError
	}

	defer respForwarded.Body.Close()

	body, err := io.ReadAll(respForwarded.Body)
	if err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		ar.options.Logger.PrintAndLog("Forward Auth", "Unable to read response to forward auth request", err)

		return ErrInternalServerError
	}

	// Responses within the 200-299 range are considered successful and allow the proxy to handle the request.
	if respForwarded.StatusCode >= http.StatusOK && respForwarded.StatusCode < http.StatusMultipleChoices {
		headerCopyIncluded(respForwarded.Header, w.Header(), ar.options.ResponseHeaders)

		return nil
	}

	// Copy the response.

	headerCopyExcluded(respForwarded.Header, w.Header(), doNotCopyHeaders)

	w.WriteHeader(respForwarded.StatusCode)
	if _, err = w.Write(body); err != nil {
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)

		ar.options.Logger.PrintAndLog("Forward Auth", "Unable to write response", err)

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

func headerCopyExcluded(original, destination http.Header, excludedHeaders []string) {
	for key, values := range original {
		if headerInList(key, excludedHeaders) {
			continue
		}

		destination[key] = append(destination[key], values...)
	}
}

func headerCopyIncluded(original, destination http.Header, includedHeaders []string) {
	for key, values := range original {
		if !headerInList(key, includedHeaders) {
			continue
		}

		destination[key] = append(destination[key], values...)
	}
}

func headerInList(a string, list []string) bool {
	if len(list) == 0 {
		return false
	}

	for _, b := range list {
		if strings.EqualFold(a, b) {
			return true
		}
	}

	return false
}
