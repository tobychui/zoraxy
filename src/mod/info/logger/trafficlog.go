package logger

/*
	Traffic Log

	This script log the traffic of HTTP requests

*/
import (
	"net/http"
	"strconv"
	"time"

	"imuslab.com/zoraxy/mod/netutils"
)

// Log HTTP request. Note that this must run in go routine to prevent any blocking
// in reverse proxy router
func (l *Logger) LogHTTPRequest(r *http.Request, reqclass string, statusCode int, downstreamHostname string, upstreamHostname string) {
	go func() {
		l.ValidateAndUpdateLogFilepath()
		if l.logger == nil || l.file == nil {
			//logger is not initiated. Do not log http request
			return
		}
		clientIP := netutils.GetRequesterIP(r)
		requestURI := r.RequestURI
		statusCodeString := strconv.Itoa(statusCode)

		//Pretty print for debugging
		//fmt.Printf("------------\nRequest URL: %s (class: %s) \nUpstream Hostname: %s\nDownstream Hostname: %s\nStatus Code: %s\n", r.URL, reqclass, upstreamHostname, downstreamHostname, statusCodeString)
		l.logger.Println("[" + time.Now().Format("2006-01-02 15:04:05.000000") + "] [router:" + reqclass + "] [origin:" + downstreamHostname + "] [client: " + clientIP + "] [useragent: " + r.UserAgent() + "] " + r.Method + " " + requestURI + " " + statusCodeString)
	}()
}
