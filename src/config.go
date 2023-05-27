package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"

	"imuslab.com/zoraxy/mod/dynamicproxy"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	Reverse Proxy Configs

	The following section handle
	the reverse proxy configs
*/

type Record struct {
	ProxyType            string
	Rootname             string
	ProxyTarget          string
	UseTLS               bool
	SkipTlsValidation    bool
	RequireBasicAuth     bool
	BasicAuthCredentials []*dynamicproxy.BasicAuthCredentials
}

func SaveReverseProxyConfig(proxyConfigRecord *Record) error {
	//TODO: Make this accept new def types
	os.MkdirAll("conf", 0775)
	filename := getFilenameFromRootName(proxyConfigRecord.Rootname)

	//Generate record
	thisRecord := proxyConfigRecord

	//Write to file
	js, _ := json.MarshalIndent(thisRecord, "", " ")
	return ioutil.WriteFile(filepath.Join("conf", filename), js, 0775)
}

func RemoveReverseProxyConfig(rootname string) error {
	filename := getFilenameFromRootName(rootname)
	removePendingFile := strings.ReplaceAll(filepath.Join("conf", filename), "\\", "/")
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
	thisRecord := Record{}
	configContent, err := ioutil.ReadFile(filename)
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

func getFilenameFromRootName(rootname string) string {
	//Generate a filename for this rootname
	filename := strings.ReplaceAll(rootname, ".", "_")
	filename = strings.ReplaceAll(filename, "/", "-")
	filename = filename + ".config"
	return filename
}
