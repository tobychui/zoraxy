package main

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"imuslab.com/zoraxy/mod/utils"
)

// IsDockerized checks if the program is running in a Docker container.
func IsDockerized() bool {
	// Check for the /proc/1/cgroup file
	file, err := os.Open("/proc/1/cgroup")
	if err != nil {
		return false
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "docker") {
			return true
		}
	}

	return false
}

// IsDockerInstalled checks if Docker is installed on the host.
func IsDockerInstalled() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

// HandleDockerAvaible check if teh docker related functions should be avaible in front-end
func HandleDockerAvailable(w http.ResponseWriter, r *http.Request) {
	dockerAvailable := IsDockerized()
	js, _ := json.Marshal(dockerAvailable)
	utils.SendJSONResponse(w, string(js))
}

// handleDockerContainersList return the current list of docker containers
// currently listening to the same bridge network interface. See PR #202 for details.
func HandleDockerContainersList(w http.ResponseWriter, r *http.Request) {
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
