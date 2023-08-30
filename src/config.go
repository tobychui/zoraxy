package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"imuslab.com/zoraxy/mod/dynamicproxy"
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
	SkipTlsValidation       bool
	RequireBasicAuth        bool
	BasicAuthCredentials    []*dynamicproxy.BasicAuthCredentials
	BasicAuthExceptionRules []*dynamicproxy.BasicAuthExceptionRule
}

// Save a reverse proxy config record to file
func SaveReverseProxyConfigToFile(proxyConfigRecord *Record) error {
	//TODO: Make this accept new def types
	os.MkdirAll("./conf/proxy/", 0775)
	filename := getFilenameFromRootName(proxyConfigRecord.Rootname)

	//Generate record
	thisRecord := proxyConfigRecord

	//Write to file
	js, _ := json.MarshalIndent(thisRecord, "", " ")
	return os.WriteFile(filepath.Join("./conf/proxy/", filename), js, 0775)
}

// Save a running reverse proxy endpoint to file (with automatic endpoint to record conversion)
func SaveReverseProxyEndpointToFile(proxyEndpoint *dynamicproxy.ProxyEndpoint) error {
	recordToSave, err := ConvertProxyEndpointToRecord(proxyEndpoint)
	if err != nil {
		return err
	}
	return SaveReverseProxyConfigToFile(recordToSave)
}

func RemoveReverseProxyConfigFile(rootname string) error {
	filename := getFilenameFromRootName(rootname)
	removePendingFile := strings.ReplaceAll(filepath.Join("./conf/proxy/", filename), "\\", "/")
	log.Println("Config Removed: ", removePendingFile)
	if utils.FileExists(removePendingFile) {
		err := os.Remove(removePendingFile)
		if err != nil {
			log.Println(err.Error())
			return err
		}
	}

	//File already gone
	return nil
}

// Return ptype, rootname and proxyTarget, error if any
func LoadReverseProxyConfig(filename string) (*Record, error) {
	thisRecord := Record{
		ProxyType:               "",
		Rootname:                "",
		ProxyTarget:             "",
		UseTLS:                  false,
		SkipTlsValidation:       false,
		RequireBasicAuth:        false,
		BasicAuthCredentials:    []*dynamicproxy.BasicAuthCredentials{},
		BasicAuthExceptionRules: []*dynamicproxy.BasicAuthExceptionRule{},
	}

	configContent, err := os.ReadFile(filename)
	if err != nil {
		return &thisRecord, err
	}

	//Unmarshal the content into config
	err = json.Unmarshal(configContent, &thisRecord)
	if err != nil {
		return &thisRecord, err
	}

	//Return it
	return &thisRecord, nil
}

// Convert a running proxy endpoint object into a save-able record struct
func ConvertProxyEndpointToRecord(targetProxyEndpoint *dynamicproxy.ProxyEndpoint) (*Record, error) {
	thisProxyConfigRecord := Record{
		ProxyType:               targetProxyEndpoint.GetProxyTypeString(),
		Rootname:                targetProxyEndpoint.RootOrMatchingDomain,
		ProxyTarget:             targetProxyEndpoint.Domain,
		UseTLS:                  targetProxyEndpoint.RequireTLS,
		SkipTlsValidation:       targetProxyEndpoint.SkipCertValidations,
		RequireBasicAuth:        targetProxyEndpoint.RequireBasicAuth,
		BasicAuthCredentials:    targetProxyEndpoint.BasicAuthCredentials,
		BasicAuthExceptionRules: targetProxyEndpoint.BasicAuthExceptionRules,
	}

	return &thisProxyConfigRecord, nil
}

func getFilenameFromRootName(rootname string) string {
	//Generate a filename for this rootname
	filename := strings.ReplaceAll(rootname, ".", "_")
	filename = strings.ReplaceAll(filename, "/", "-")
	filename = filename + ".config"
	return filename
}

/*
	Importer and Exporter of Zoraxy proxy config
*/

func ExportConfigAsZip(w http.ResponseWriter, r *http.Request) {
	includeSysDBRaw, err := utils.GetPara(r, "includeDB")
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
	err = filepath.Walk(folderPath, func(filePath string, fileInfo os.FileInfo, err error) error {
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
			log.Println("[Backup] Unable to zip sysdb: " + err.Error())
			return
		}

		// Open the file on disk
		file, err := os.Open("sys.db")
		if err != nil {
			log.Println("[Backup] Unable to open sysdb: " + err.Error())
			return
		}
		defer file.Close()

		// Copy the file contents to the zip file
		_, err = io.Copy(zipFile, file)
		if err != nil {
			log.Println(err)
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
	log.Println("Configuration restored")
	fmt.Fprintln(w, "Configuration restored")

	if restoreDatabase {
		go func() {
			log.Println("Database altered. Restarting in 3 seconds...")
			time.Sleep(3 * time.Second)
			os.Exit(0)
		}()

	}

}
