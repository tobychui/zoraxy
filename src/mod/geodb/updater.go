package geodb

import (
	"io"
	"log"
	"net/http"
	"os"

	"imuslab.com/zoraxy/mod/utils"
)

const (
	ipv4UpdateSource = "https://cdn.jsdelivr.net/npm/@ip-location-db/geo-whois-asn-country/geo-whois-asn-country-ipv4.csv"
	ipv6UpdateSource = "https://cdn.jsdelivr.net/npm/@ip-location-db/geo-whois-asn-country/geo-whois-asn-country-ipv6.csv"
)

// DownloadGeoDBUpdate download the latest geodb update
func DownloadGeoDBUpdate(externalGeoDBStoragePath string) {
	//Create the storage path if not exist
	if !utils.FileExists(externalGeoDBStoragePath) {
		os.MkdirAll(externalGeoDBStoragePath, 0755)
	}

	//Download the update
	log.Println("Downloading IPv4 database update...")
	err := downloadFile(ipv4UpdateSource, externalGeoDBStoragePath+"/geoipv4.csv")
	if err != nil {
		log.Println(err)
		return
	}

	log.Println("Downloading IPv6 database update...")
	err = downloadFile(ipv6UpdateSource, externalGeoDBStoragePath+"/geoipv6.csv")
	if err != nil {
		log.Println(err)
		return
	}

	log.Println("GeoDB update stored at: " + externalGeoDBStoragePath)
	log.Println("Exiting...")
}

// Utility functions
func downloadFile(url string, savepath string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	fileContent, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return os.WriteFile(savepath, fileContent, 0644)
}
