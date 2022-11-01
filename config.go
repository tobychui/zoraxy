package main

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type Record struct {
	ProxyType   string
	Rootname    string
	ProxyTarget string
	UseTLS      bool
}

func SaveReverseProxyConfig(ptype string, rootname string, proxyTarget string, useTLS bool) error {
	os.MkdirAll("conf", 0775)
	filename := getFilenameFromRootName(rootname)

	//Generate record
	thisRecord := Record{
		ProxyType:   ptype,
		Rootname:    rootname,
		ProxyTarget: proxyTarget,
		UseTLS:      useTLS,
	}

	//Write to file
	js, _ := json.MarshalIndent(thisRecord, "", " ")
	return ioutil.WriteFile(filepath.Join("conf", filename), js, 0775)
}

func RemoveReverseProxyConfig(rootname string) error {
	filename := getFilenameFromRootName(rootname)
	log.Println("Config Removed: ", filepath.Join("conf", filename))
	if fileExists(filepath.Join("conf", filename)) {
		err := os.Remove(filepath.Join("conf", filename))
		if err != nil {
			log.Println(err.Error())
			return err
		}
	}

	//File already gone
	return nil
}

//Return ptype, rootname and proxyTarget, error if any
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
