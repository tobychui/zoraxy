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
	DatabaseKeyRequestIncludeBody     = "requestIncludeBody"
	DatabaseKeyUseXOriginalHeaders    = "useXOriginalHeaders"

	HeaderXForwardedProto  = "X-Forwarded-Proto"
	HeaderXForwardedHost   = "X-Forwarded-Host"
	HeaderXForwardedURI    = "X-Forwarded-URI"
	HeaderXForwardedFor    = "X-Forwarded-For"
	HeaderXForwardedMethod = "X-Forwarded-Method"

	HeaderXOriginalURL    = "X-Original-URL"
	HeaderXOriginalIP     = "X-Original-IP"
	HeaderXOriginalMethod = "X-Original-Method"

	HeaderCookie   = "Cookie"
	HeaderLocation = "Location"

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
