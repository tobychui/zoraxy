package dynamicproxy

import (
	"encoding/json"
	"errors"
	"log"
	"os"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	rootRoute.go

	This script handle special case in routing where the root proxy
	entity is involved. This also include its setting object
	RootRoutingOptions
*/

var rootConfigFilepath string = "conf/root_config.json"

func loadRootRoutingOptionsFromFile() (*RootRoutingOptions, error) {
	if !utils.FileExists(rootConfigFilepath) {
		//Not found. Create a root option
		js, _ := json.MarshalIndent(RootRoutingOptions{}, "", " ")
		err := os.WriteFile(rootConfigFilepath, js, 0775)
		if err != nil {
			return nil, errors.New("Unable to write root config to file: " + err.Error())
		}
	}
	newRootOption := RootRoutingOptions{}
	rootOptionsBytes, err := os.ReadFile(rootConfigFilepath)
	if err != nil {
		log.Println("[Error] Unable to read root config file at " + rootConfigFilepath + ": " + err.Error())
		return nil, err
	}
	err = json.Unmarshal(rootOptionsBytes, &newRootOption)
	if err != nil {
		log.Println("[Error] Unable to parse root config file: " + err.Error())
		return nil, err
	}

	return &newRootOption, nil
}

// Save the new config to file. Note that this will not overwrite the runtime one
func (opt *RootRoutingOptions) SaveToFile() error {
	js, _ := json.MarshalIndent(opt, "", " ")
	err := os.WriteFile(rootConfigFilepath, js, 0775)
	return err
}
