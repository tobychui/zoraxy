package main

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"imuslab.com/zoraxy/mod/utils"
)

func handleDockerContainersList(w http.ResponseWriter, r *http.Request) {
	apiClient, err := client.NewClientWithOpts(client.WithVersion("1.43"))
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	defer apiClient.Close()

	containers, err := apiClient.ContainerList(context.Background(), container.ListOptions{All: true})
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	networks, err := apiClient.NetworkList(context.Background(), types.NetworkListOptions{})
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	result := map[string]interface{}{
		"network":    networks,
		"containers": containers,
	}

	js, err := json.Marshal(result)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendJSONResponse(w, string(js))
}
