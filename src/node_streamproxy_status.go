package main

import (
	"strings"

	"imuslab.com/zoraxy/mod/streamproxy"
)

func resolveStreamProxyRemoteStatus(assignedNodeID string, configUUID string) *streamproxy.RemoteRuntimeState {
	if nodeManager == nil {
		return nil
	}

	assignedNodeID = strings.TrimSpace(assignedNodeID)
	configUUID = strings.TrimSpace(configUUID)
	if assignedNodeID == "" || configUUID == "" {
		return nil
	}

	targetNode, err := nodeManager.GetNodeByID(assignedNodeID)
	if err != nil {
		return nil
	}

	telemetry, err := nodeManager.LoadNodeTelemetry(assignedNodeID)
	online := nodeManager.IsNodeOnline(targetNode)
	if err != nil || telemetry == nil || telemetry.StreamProxy == nil {
		return &streamproxy.RemoteRuntimeState{
			Online: online,
			Status: map[bool]string{true: "unknown", false: "offline"}[online],
		}
	}

	runtime, ok := telemetry.StreamProxy[configUUID]
	if !ok || runtime == nil {
		return &streamproxy.RemoteRuntimeState{
			Online: online,
			Status: map[bool]string{true: "unknown", false: "offline"}[online],
		}
	}

	status := "stopped"
	if runtime.Running {
		status = "running"
	}
	if !online {
		status = "offline"
	}

	return &streamproxy.RemoteRuntimeState{
		Running:    runtime.Running,
		Online:     online,
		Status:     status,
		LastUpdate: runtime.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}
