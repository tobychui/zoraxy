package main

import (
	"net/http"

	"imuslab.com/zoraxy/mod/auth"
)

// Register the APIs for Nodes Management
func RegisterNodeAPIs(authRouter *auth.NodeAuthMiddleware, publicRouter *http.ServeMux) {
	// api for node heartbeat
	authRouter.HandleFunc("/api/update", nodeManager.HandleNodeUpdate)
	authRouter.HandleFunc("/api/config", nodeManager.HandleGetProxyConfigs)
	authRouter.HandleFunc("/api/access", nodeManager.HandleGetAccessRules)
	authRouter.HandleFunc("/api/certs", nodeManager.HandleGetCertificates)
	authRouter.HandleFunc("/api/system", nodeManager.HandleGetSystemData)
	authRouter.HandleFunc("/api/telemetry", nodeManager.HandleNodeTelemetry)

	// management api for authorized users
	publicRouter.HandleFunc("/api/nodes/unregister", nodeManager.HandleUnregisterNode)
	publicRouter.HandleFunc("/api/nodes/register", nodeManager.HandleRegisterNode)
	publicRouter.HandleFunc("/api/nodes/list", nodeManager.HandleListNodes)
	publicRouter.HandleFunc("/api/nodes/info", nodeManager.HandleGetNodeInfo)
	publicRouter.HandleFunc("/api/nodes/summary", nodeManager.HandleGetTelemetrySummary)
	publicRouter.HandleFunc("/api/nodes/rotateToken", nodeManager.HandleRotateNodeToken)
	publicRouter.HandleFunc("/api/nodes/setEnabled", nodeManager.HandleSetNodeEnabled)
}

/* Register all the APIs */
func initNodeAPI(targetMux *http.ServeMux, mux *http.ServeMux) {
	authMiddleware := auth.NewNodeAuthMiddleware(
		auth.NodeMiddlewareOptions{
			TargetMux:   targetMux,
			NodeManager: nodeManager,
			DeniedHandler: func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "401 - Unauthorized", http.StatusUnauthorized)
			},
		},
	)

	RegisterNodeAPIs(authMiddleware, mux)
}
