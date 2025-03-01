package main

import (
	"fmt"
	"net/http"
	"os"

	"aroz.org/zoraxy/ztnc/mod/database"
	"aroz.org/zoraxy/ztnc/mod/database/dbinc"
	"aroz.org/zoraxy/ztnc/mod/ganserv"
	"aroz.org/zoraxy/ztnc/mod/utils"
)

func startGanNetworkController() error {
	fmt.Println("Starting ZeroTier Network Controller")
	//Create a new database
	var err error
	sysdb, err = database.NewDatabase(DB_FILE_PATH, dbinc.BackendBoltDB)
	if err != nil {
		return err
	}

	//Initiate the GAN server manager
	usingZtAuthToken := ""
	ztAPIPort := 9993

	if utils.FileExists(AUTH_TOKEN_PATH) {
		authToken, err := os.ReadFile(AUTH_TOKEN_PATH)
		if err != nil {
			fmt.Println("Error reading auth config file:", err)
			return err
		}
		usingZtAuthToken = string(authToken)
		fmt.Println("Loaded ZeroTier Auth Token from file")
	}

	if usingZtAuthToken == "" {
		usingZtAuthToken, err = ganserv.TryLoadorAskUserForAuthkey()
		if err != nil {
			fmt.Println("Error getting ZeroTier Auth Token:", err)
		}
	}

	ganManager = ganserv.NewNetworkManager(&ganserv.NetworkManagerOptions{
		AuthToken: usingZtAuthToken,
		ApiPort:   ztAPIPort,
		Database:  sysdb,
	})

	return nil
}

func initApiEndpoints() {
	//UI_RELPATH must be the same as the one in the plugin intro spect
	// as Zoraxy plugin UI proxy will only forward the UI path to your plugin
	http.HandleFunc(UI_RELPATH+"/api/gan/network/info", ganManager.HandleGetNodeID)
	http.HandleFunc(UI_RELPATH+"/api/gan/network/add", ganManager.HandleAddNetwork)
	http.HandleFunc(UI_RELPATH+"/api/gan/network/remove", ganManager.HandleRemoveNetwork)
	http.HandleFunc(UI_RELPATH+"/api/gan/network/list", ganManager.HandleListNetwork)
	http.HandleFunc(UI_RELPATH+"/api/gan/network/name", ganManager.HandleNetworkNaming)
	http.HandleFunc(UI_RELPATH+"/api/gan/network/setRange", ganManager.HandleSetRanges)
	http.HandleFunc(UI_RELPATH+"/api/gan/network/join", ganManager.HandleServerJoinNetwork)
	http.HandleFunc(UI_RELPATH+"/api/gan/network/leave", ganManager.HandleServerLeaveNetwork)
	http.HandleFunc(UI_RELPATH+"/api/gan/members/list", ganManager.HandleMemberList)
	http.HandleFunc(UI_RELPATH+"/api/gan/members/ip", ganManager.HandleMemberIP)
	http.HandleFunc(UI_RELPATH+"/api/gan/members/name", ganManager.HandleMemberNaming)
	http.HandleFunc(UI_RELPATH+"/api/gan/members/authorize", ganManager.HandleMemberAuthorization)
	http.HandleFunc(UI_RELPATH+"/api/gan/members/delete", ganManager.HandleMemberDelete)
}
