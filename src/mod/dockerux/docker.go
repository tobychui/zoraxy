//go:build !windows
// +build !windows

package dockerux

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

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
	apiClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
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

func (d *UXOptimizer) GetCurrentContainerImage() string {
	if d == nil || !d.RunninInDocker {
		return ""
	}

	d.imageDetectMutex.Lock()
	defer d.imageDetectMutex.Unlock()

	if d.imageChecked {
		return strings.TrimSpace(d.detectedImage)
	}

	d.imageChecked = true
	d.detectedImage = strings.TrimSpace(d.detectCurrentContainerImage())
	return d.detectedImage
}

func (d *UXOptimizer) detectCurrentContainerImage() string {
	apiClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return ""
	}
	defer apiClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	hostname, err := os.Hostname()
	if err != nil {
		return ""
	}
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		return ""
	}

	containerInfo, err := apiClient.ContainerInspect(ctx, hostname)
	if err == nil {
		if image := strings.TrimSpace(containerInfo.Config.Image); image != "" {
			return image
		}
	}

	containers, err := apiClient.ContainerList(ctx, container.ListOptions{All: true})
	if err != nil {
		return ""
	}

	for _, currentContainer := range containers {
		containerID := strings.TrimSpace(currentContainer.ID)
		if containerID != "" && (containerID == hostname || strings.HasPrefix(containerID, hostname) || strings.HasPrefix(hostname, containerID)) {
			if image := strings.TrimSpace(currentContainer.Image); image != "" {
				return image
			}
		}

		for _, rawName := range currentContainer.Names {
			name := strings.TrimPrefix(strings.TrimSpace(rawName), "/")
			if name == hostname {
				if image := strings.TrimSpace(currentContainer.Image); image != "" {
					return image
				}
			}
		}
	}

	return ""
}

func (d *UXOptimizer) ResolveSuggestedNodeImage(defaultImage string) (string, string) {
	defaultImage = strings.TrimSpace(defaultImage)
	if defaultImage == "" {
		defaultImage = "zoraxydocker/zoraxy:latest"
	}

	if image := strings.TrimSpace(d.GetCurrentContainerImage()); image != "" {
		return image, "detected"
	}

	return defaultImage, "default"
}

func (d *UXOptimizer) RefreshCurrentContainerImage() error {
	if d == nil || !d.RunninInDocker {
		return nil
	}

	d.imageDetectMutex.Lock()
	defer d.imageDetectMutex.Unlock()

	image := strings.TrimSpace(d.detectCurrentContainerImage())
	d.imageChecked = true
	d.detectedImage = image
	if image == "" {
		return fmt.Errorf("current docker image not detected")
	}

	return nil
}
