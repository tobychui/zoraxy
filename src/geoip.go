package main

import (
	"net"
	"net/http"
	"strings"

	"github.com/oschwald/geoip2-golang"
)

func getCountryCodeFromRequest(r *http.Request) string {
	countryCode := ""

	// Get the IP address of the user from the request headers
	ipAddress := r.Header.Get("X-Forwarded-For")
	if ipAddress == "" {
		ipAddress = strings.Split(r.RemoteAddr, ":")[0]
	}

	// Open the GeoIP database
	db, err := geoip2.Open("./system/GeoIP2-Country.mmdb")
	if err != nil {
		// Handle the error
		return countryCode
	}
	defer db.Close()

	// Look up the country code for the IP address
	record, err := db.Country(net.ParseIP(ipAddress))
	if err != nil {
		// Handle the error
		return countryCode
	}

	// Get the ISO country code from the record
	countryCode = record.Country.IsoCode

	return countryCode
}
