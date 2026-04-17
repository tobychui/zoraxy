package anubis

import "time"

type AnubisConfig struct {
	// Base
	Policy        string
	Ed25519KeyHex string

	// Config
	Difficulty     int
	ServeRobotsTXT bool
	WebmasterEmail string

	// Cookies
	CookieDomain        string
	CookieDynamicDomain bool
	CookieExpiration    time.Duration
	CookiePartitioned   bool
	CookieSecure        bool // default true
}
