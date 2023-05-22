package dpcore

import (
	"net/url"
)

func replaceLocationHost(urlString string, newHost string, useTLS bool) (string, error) {
	u, err := url.Parse(urlString)
	if err != nil {
		return "", err
	}

	if useTLS {
		u.Scheme = "https"
	} else {
		u.Scheme = "http"
	}

	u.Host = newHost
	return u.String(), nil
}
