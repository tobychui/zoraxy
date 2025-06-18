package forward

import "errors"

const (
	LogTitle = "Forward Auth"

	DatabaseTable = "auth_sso_forward"

	DatabaseKeyAddress                = "address"
	DatabaseKeyResponseHeaders        = "responseHeaders"
	DatabaseKeyResponseClientHeaders  = "responseClientHeaders"
	DatabaseKeyRequestHeaders         = "requestHeaders"
	DatabaseKeyRequestIncludedCookies = "requestIncludedCookies"
	DatabaseKeyRequestExcludedCookies = "requestExcludedCookies"

	HeaderXForwardedProto  = "X-Forwarded-Proto"
	HeaderXForwardedHost   = "X-Forwarded-Host"
	HeaderXForwardedFor    = "X-Forwarded-For"
	HeaderXForwardedURI    = "X-Forwarded-URI"
	HeaderXForwardedMethod = "X-Forwarded-Method"

	HeaderCookie = "Cookie"

	HeaderUpgrade          = "Upgrade"
	HeaderConnection       = "Connection"
	HeaderTransferEncoding = "Transfer-Encoding"
	HeaderTE               = "TE"
	HeaderTrailers         = "Trailers"
	HeaderKeepAlive        = "Keep-Alive"
)

var (
	ErrInternalServerError = errors.New("internal server error")
	ErrUnauthorized        = errors.New("unauthorized")
)

var (
	doNotCopyHeaders = []string{
		HeaderUpgrade,
		HeaderConnection,
		HeaderTransferEncoding,
		HeaderTE,
		HeaderTrailers,
		HeaderKeepAlive,
	}
)
