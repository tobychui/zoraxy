package domainsniff

/*
	Domainsniff

	This package contain codes that perform project / domain specific behavior in Zoraxy
	If you want Zoraxy to handle a particular domain or open source project in a special way,
	you can add the checking logic here.

*/
import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/utils"
)

// Check if the domain is reachable and return err if not reachable
func DomainReachableWithError(domain string) error {
	timeout := 1 * time.Second
	conn, err := net.DialTimeout("tcp", domain, timeout)
	if err != nil {
		return err
	}

	conn.Close()
	return nil
}

// Check if a domain have TLS but it is self-signed or expired
// Return false if sniff error
func DomainIsSelfSigned(domain string) bool {
	//Extract the domain from URl in case the user input the full URL
	host, port, err := net.SplitHostPort(domain)
	if err != nil {
		host = domain
	} else {
		domain = host + ":" + port
	}
	if !strings.Contains(domain, ":") {
		domain = domain + ":443"
	}

	//Get the certificate
	conn, err := net.Dial("tcp", domain)
	if err != nil {
		return false
	}
	defer conn.Close()

	//Connect with TLS using secure verify
	tlsConn := tls.Client(conn, nil)
	err = tlsConn.Handshake()
	if err == nil {
		//This is a valid certificate
		fmt.Println()
		return false
	}

	//Connect with TLS using insecure skip verify
	config := &tls.Config{
		InsecureSkipVerify: true,
	}
	tlsConn = tls.Client(conn, config)
	err = tlsConn.Handshake()
	//If the handshake is successful, this is a self-signed certificate
	return err == nil
}

// Check if domain reachable
func DomainReachable(domain string) bool {
	return DomainReachableWithError(domain) == nil
}

// Check if domain is served by a web server using HTTPS
func DomainUsesTLS(targetURL string) bool {
	//Check if the site support https
	httpsUrl := fmt.Sprintf("https://%s", targetURL)
	httpUrl := fmt.Sprintf("http://%s", targetURL)

	client := http.Client{Timeout: 5 * time.Second}

	resp, err := client.Head(httpsUrl)
	if err == nil && resp.StatusCode == http.StatusOK {
		return true
	}

	resp, err = client.Head(httpUrl)
	if err == nil && resp.StatusCode == http.StatusOK {
		return false
	}

	//If the site is not reachable, return false
	return false
}

/*
	Request Handlers
*/
//Check if site support TLS
//Pass in ?selfsignchk=true to also check for self-signed certificate
func HandleCheckSiteSupportTLS(w http.ResponseWriter, r *http.Request) {
	targetURL, err := utils.PostPara(r, "url")
	if err != nil {
		utils.SendErrorResponse(w, "invalid url given")
		return
	}

	//If the selfsign flag is set, also chec for self-signed certificate
	_, err = utils.PostBool(r, "selfsignchk")
	if err == nil {
		//Return the https and selfsign status
		type result struct {
			Protocol string `json:"protocol"`
			SelfSign bool   `json:"selfsign"`
		}

		scanResult := result{Protocol: "http", SelfSign: false}

		if DomainUsesTLS(targetURL) {
			scanResult.Protocol = "https"
			if DomainIsSelfSigned(targetURL) {
				scanResult.SelfSign = true
			}
		}

		js, _ := json.Marshal(scanResult)
		utils.SendJSONResponse(w, string(js))
		return
	}

	if DomainUsesTLS(targetURL) {
		js, _ := json.Marshal("https")
		utils.SendJSONResponse(w, string(js))
		return
	} else {
		js, _ := json.Marshal("http")
		utils.SendJSONResponse(w, string(js))
		return
	}
}
