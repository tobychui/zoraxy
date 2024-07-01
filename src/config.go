package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/dynamicproxy"
	"imuslab.com/zoraxy/mod/dynamicproxy/loadbalance"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	Reverse Proxy Configs

	The following section handle
	the reverse proxy configs
*/

type Record struct {
	ProxyType               string
	Rootname                string
	ProxyTarget             string
	UseTLS                  bool
	BypassGlobalTLS         bool
	SkipTlsValidation       bool
	RequireBasicAuth        bool
	BasicAuthCredentials    []*dynamicproxy.BasicAuthCredentials
	BasicAuthExceptionRules []*dynamicproxy.BasicAuthExceptionRule
}

/*
Load Reverse Proxy Config from file and append it to current runtime proxy router
*/
func LoadReverseProxyConfig(configFilepath string) error {
	//Load the config file from disk
	endpointConfig, err := os.ReadFile(configFilepath)
	if err != nil {
		return err
	}

	//Parse it into dynamic proxy endpoint
	thisConfigEndpoint := dynamicproxy.ProxyEndpoint{}
	err = json.Unmarshal(endpointConfig, &thisConfigEndpoint)
	if err != nil {
		return err
	}

	//Matching domain not set. Assume root
	if thisConfigEndpoint.RootOrMatchingDomain == "" {
		thisConfigEndpoint.RootOrMatchingDomain = "/"
	}

	if thisConfigEndpoint.ProxyType == dynamicproxy.ProxyType_Root {
		//This is a root config file
		rootProxyEndpoint, err := dynamicProxyRouter.PrepareProxyRoute(&thisConfigEndpoint)
		if err != nil {
			return err
		}

		dynamicProxyRouter.SetProxyRouteAsRoot(rootProxyEndpoint)

	} else if thisConfigEndpoint.ProxyType == dynamicproxy.ProxyType_Host {
		//This is a host config file
		readyProxyEndpoint, err := dynamicProxyRouter.PrepareProxyRoute(&thisConfigEndpoint)
		if err != nil {
			return err
		}

		dynamicProxyRouter.AddProxyRouteToRuntime(readyProxyEndpoint)
	} else {
		return errors.New("not supported proxy type")
	}

	SystemWideLogger.PrintAndLog("Proxy", thisConfigEndpoint.RootOrMatchingDomain+" -> "+loadbalance.GetUpstreamsAsString(thisConfigEndpoint.ActiveOrigins)+" routing rule loaded", nil)
	return nil
}

func filterProxyConfigFilename(filename string) string {
	//Filter out wildcard characters
	filename = strings.ReplaceAll(filename, "*", "(ST)")
	filename = strings.ReplaceAll(filename, "?", "(QM)")
	filename = strings.ReplaceAll(filename, "[", "(OB)")
	filename = strings.ReplaceAll(filename, "]", "(CB)")
	filename = strings.ReplaceAll(filename, "#", "(HT)")
	return filepath.ToSlash(filename)
}

func SaveReverseProxyConfig(endpoint *dynamicproxy.ProxyEndpoint) error {
	//Get filename for saving
	filename := filepath.Join("./conf/proxy/", endpoint.RootOrMatchingDomain+".config")
	if endpoint.ProxyType == dynamicproxy.ProxyType_Root {
		filename = "./conf/proxy/root.config"
	}

	filename = filterProxyConfigFilename(filename)

	//Save config to file
	js, err := json.MarshalIndent(endpoint, "", " ")
	if err != nil {
		return err
	}

	return os.WriteFile(filename, js, 0775)
}

func RemoveReverseProxyConfig(endpoint string) error {
	filename := filepath.Join("./conf/proxy/", endpoint+".config")
	if endpoint == "/" {
		filename = "./conf/proxy/root.config"
	}

	filename = filterProxyConfigFilename(filename)

	if !utils.FileExists(filename) {
		return errors.New("target endpoint not exists")
	}
	return os.Remove(filename)
}

// Get the default root config that point to the internal static web server
// this will be used if root config is not found (new deployment / missing root.config file)
func GetDefaultRootConfig() (*dynamicproxy.ProxyEndpoint, error) {
	//Default settings
	rootProxyEndpoint, err := dynamicProxyRouter.PrepareProxyRoute(&dynamicproxy.ProxyEndpoint{
		ProxyType:            dynamicproxy.ProxyType_Root,
		RootOrMatchingDomain: "/",
		ActiveOrigins: []*loadbalance.Upstream{
			{
				OriginIpOrDomain:    "127.0.0.1:" + staticWebServer.GetListeningPort(),
				RequireTLS:          false,
				SkipCertValidations: false,
				Weight:              0,
			},
		},
		InactiveOrigins:         []*loadbalance.Upstream{},
		BypassGlobalTLS:         false,
		VirtualDirectories:      []*dynamicproxy.VirtualDirectoryEndpoint{},
		RequireBasicAuth:        false,
		BasicAuthCredentials:    []*dynamicproxy.BasicAuthCredentials{},
		BasicAuthExceptionRules: []*dynamicproxy.BasicAuthExceptionRule{},
		DefaultSiteOption:       dynamicproxy.DefaultSite_InternalStaticWebServer,
		DefaultSiteValue:        "",
	})
	if err != nil {
		return nil, err
	}

	return rootProxyEndpoint, nil
}

/*
	Importer and Exporter of Zoraxy proxy config
*/

func ExportConfigAsZip(w http.ResponseWriter, r *http.Request) {
	includeSysDBRaw, _ := utils.GetPara(r, "includeDB")
	includeSysDB := false
	if includeSysDBRaw == "true" {
		//Include the system database in backup snapshot
		//Temporary set it to read only
		sysdb.ReadOnly = true
		includeSysDB = true
	}

	// Specify the folder path to be zipped
	folderPath := "./conf/"

	// Set the Content-Type header to indicate it's a zip file
	w.Header().Set("Content-Type", "application/zip")
	// Set the Content-Disposition header to specify the file name
	w.Header().Set("Content-Disposition", "attachment; filename=\"config.zip\"")

	// Create a zip writer
	zipWriter := zip.NewWriter(w)
	defer zipWriter.Close()

	// Walk through the folder and add files to the zip
	err := filepath.Walk(folderPath, func(filePath string, fileInfo os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if folderPath == filePath {
			//Skip root folder
			return nil
		}

		// Create a new file in the zip
		if !utils.IsDir(filePath) {
			zipFile, err := zipWriter.Create(filePath)
			if err != nil {
				return err
			}

			// Open the file on disk
			file, err := os.Open(filePath)
			if err != nil {
				return err
			}
			defer file.Close()

			// Copy the file contents to the zip file
			_, err = io.Copy(zipFile, file)
			if err != nil {
				return err
			}
		}

		return nil
	})

	if includeSysDB {
		//Also zip in the sysdb
		zipFile, err := zipWriter.Create("sys.db")
		if err != nil {
			SystemWideLogger.PrintAndLog("Backup", "Unable to zip sysdb", err)
			return
		}

		// Open the file on disk
		file, err := os.Open("sys.db")
		if err != nil {
			SystemWideLogger.PrintAndLog("Backup", "Unable to open sysdb", err)
			return
		}
		defer file.Close()

		// Copy the file contents to the zip file
		_, err = io.Copy(zipFile, file)
		if err != nil {
			SystemWideLogger.Println(err)
			return
		}

		//Restore sysdb state
		sysdb.ReadOnly = false
	}

	if err != nil {
		// Handle the error and send an HTTP response with the error message
		http.Error(w, fmt.Sprintf("Failed to zip folder: %v", err), http.StatusInternalServerError)
		return
	}
}

func ImportConfigFromZip(w http.ResponseWriter, r *http.Request) {
	// Check if the request is a POST with a file upload
	if r.Method != http.MethodPost {
		http.Error(w, "Invalid request method", http.StatusBadRequest)
		return
	}

	// Max file size limit (10 MB in this example)
	r.ParseMultipartForm(10 << 20)

	// Get the uploaded file
	file, handler, err := r.FormFile("file")
	if err != nil {
		http.Error(w, "Failed to retrieve uploaded file", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	if filepath.Ext(handler.Filename) != ".zip" {
		http.Error(w, "Upload file is not a zip file", http.StatusInternalServerError)
		return
	}
	// Create the target directory to unzip the files
	targetDir := "./conf"
	if utils.FileExists(targetDir) {
		//Backup the old config to old
		os.Rename("./conf", "./conf.old_"+strconv.Itoa(int(time.Now().Unix())))
	}

	err = os.MkdirAll(targetDir, os.ModePerm)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create target directory: %v", err), http.StatusInternalServerError)
		return
	}

	// Open the zip file
	zipReader, err := zip.NewReader(file, handler.Size)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to open zip file: %v", err), http.StatusInternalServerError)
		return
	}

	restoreDatabase := false

	// Extract each file from the zip archive
	for _, zipFile := range zipReader.File {
		// Open the file in the zip archive
		rc, err := zipFile.Open()
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to open file in zip: %v", err), http.StatusInternalServerError)
			return
		}
		defer rc.Close()

		// Create the corresponding file on disk
		zipFile.Name = strings.ReplaceAll(zipFile.Name, "../", "")
		fmt.Println("Restoring: " + strings.ReplaceAll(zipFile.Name, "\\", "/"))
		if zipFile.Name == "sys.db" {
			//Sysdb replacement. Close the database and restore
			sysdb.Close()
			restoreDatabase = true
		} else if !strings.HasPrefix(strings.ReplaceAll(zipFile.Name, "\\", "/"), "conf/") {
			//Malformed zip file.
			http.Error(w, fmt.Sprintf("Invalid zip file structure or version too old"), http.StatusInternalServerError)
			return
		}

		//Check if parent dir exists
		if !utils.FileExists(filepath.Dir(zipFile.Name)) {
			os.MkdirAll(filepath.Dir(zipFile.Name), 0775)
		}

		//Create the file
		newFile, err := os.Create(zipFile.Name)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to create file: %v", err), http.StatusInternalServerError)
			return
		}
		defer newFile.Close()

		// Copy the file contents from the zip to the new file
		_, err = io.Copy(newFile, rc)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to extract file from zip: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Send a success response
	w.WriteHeader(http.StatusOK)
	SystemWideLogger.Println("Configuration restored")
	fmt.Fprintln(w, "Configuration restored")

	if restoreDatabase {
		go func() {
			SystemWideLogger.Println("Database altered. Restarting in 3 seconds...")
			time.Sleep(3 * time.Second)
			os.Exit(0)
		}()

	}

}
