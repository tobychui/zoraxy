//go:build !windows
// +build !windows

package dockerux

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"imuslab.com/zoraxy/mod/utils"
)

func (d *UXOptimizer) HandleDockerAvailable(w http.ResponseWriter, r *http.Request) {
	js, _ := json.Marshal(d.RunninInDocker)
	utils.SendJSONResponse(w, string(js))
}

func (d *UXOptimizer) HandleDockerContainersList(w http.ResponseWriter, r *http.Request) {
	apiClient, err := client.NewClientWithOpts(client.WithVersion("1.43"))
	if err != nil {
		d.SystemWideLogger.PrintAndLog("Docker", "Unable to create new docker client", err)
		utils.SendErrorResponse(w, "Docker client initiation failed")
		return
	}
	defer apiClient.Close()

	containers, err := apiClient.ContainerList(context.Background(), container.ListOptions{All: true})
	if err != nil {
		d.SystemWideLogger.PrintAndLog("Docker", "List docker container failed", err)
		utils.SendErrorResponse(w, "List docker container failed")
		return
	}

	networks, err := apiClient.NetworkList(context.Background(), types.NetworkListOptions{})
	if err != nil {
		d.SystemWideLogger.PrintAndLog("Docker", "List docker network failed", err)
		utils.SendErrorResponse(w, "List docker network failed")
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
