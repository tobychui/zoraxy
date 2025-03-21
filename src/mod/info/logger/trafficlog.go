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
func (l *Logger) LogHTTPRequest(r *http.Request, reqclass string, statusCode int) {
	go func() {
		l.ValidateAndUpdateLogFilepath()
		if l.logger == nil || l.file == nil {
			//logger is not initiated. Do not log http request
			return
		}
		clientIP := netutils.GetRequesterIP(r)
		requestURI := r.RequestURI
		statusCodeString := strconv.Itoa(statusCode)
		//fmt.Println("[" + time.Now().Format("2006-01-02 15:04:05.000000") + "] [router:" + reqclass + "] [client " + clientIP + "] " + r.Method + " " + requestURI + " " + statusCodeString)
		l.logger.Println("[" + time.Now().Format("2006-01-02 15:04:05.000000") + "] [router:" + reqclass + "] [origin:" + r.URL.Hostname() + "] [client " + clientIP + "] [useragent " + r.UserAgent() + "] " + r.Method + " " + requestURI + " " + statusCodeString)
	}()
}
