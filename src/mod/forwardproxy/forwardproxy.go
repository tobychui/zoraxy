package forwardproxy

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"imuslab.com/zoraxy/mod/database"
	"imuslab.com/zoraxy/mod/forwardproxy/cproxy"
	"imuslab.com/zoraxy/mod/info/logger"
	"imuslab.com/zoraxy/mod/utils"
)

type ZrFilter struct {
	//To be implemented
}

type Handler struct {
	server  *http.Server
	handler *http.Handler
	running bool
	db      *database.Database
	logger  *logger.Logger
	Port    int
}

func NewForwardProxy(sysdb *database.Database, port int, logger *logger.Logger) *Handler {
	thisFilter := ZrFilter{}
	handler := cproxy.New(cproxy.Options.Filter(thisFilter))

	return &Handler{
		db:      sysdb,
		server:  nil,
		handler: &handler,
		running: false,
		logger:  logger,
		Port:    port,
	}
}

// Start the forward proxy
func (h *Handler) Start() error {
	if h.running {
		return errors.New("forward proxy already running")
	}
	server := &http.Server{Addr: ":" + strconv.Itoa(h.Port), Handler: *h.handler}
	h.server = server

	go func() {
		if err := server.ListenAndServe(); err != nil {
			if err != nil {
				log.Println(err.Error())
			}
		}
	}()

	h.running = true
	return nil
}

// Stop the forward proxy
func (h *Handler) Stop() error {
	if h.running && h.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := h.server.Shutdown(ctx); err != nil {
			return err
		}
		h.running = false
		h.server = nil
	}
	return nil
}

// Update the port number of the forward proxy
func (h *Handler) UpdatePort(newPort int) error {
	h.Stop()
	h.Port = newPort
	return h.Start()
}

func (it ZrFilter) IsAuthorized(w http.ResponseWriter, r *http.Request) bool {
	return true
}

// Handle port change of the forward proxy
func (h *Handler) HandlePort(w http.ResponseWriter, r *http.Request) {
	port, err := utils.PostInt(r, "port")
	if err != nil {
		js, _ := json.Marshal(h.Port)
		utils.SendJSONResponse(w, string(js))
	} else {
		//Update the port
		err = h.UpdatePort(port)
		if err != nil {
			utils.SendErrorResponse(w, err.Error())
			return
		}
		h.logger.PrintAndLog("Forward Proxy", "HTTP Forward Proxy port updated to :"+strconv.Itoa(h.Port), nil)
		h.db.Write("fwdproxy", "port", port)
		utils.SendOK(w)
	}
}

// Handle power toggle of the forward proxys
func (h *Handler) HandleToogle(w http.ResponseWriter, r *http.Request) {
	enabled, err := utils.PostBool(r, "enable")
	if err != nil {
		//Get the current state of the forward proxy
		js, _ := json.Marshal(h.running)
		utils.SendJSONResponse(w, string(js))
	} else {
		if enabled {
			err = h.Start()
			if err != nil {
				h.logger.PrintAndLog("Forward Proxy", "Unable to start forward proxy server", err)
				utils.SendErrorResponse(w, err.Error())
				return
			}
			h.logger.PrintAndLog("Forward Proxy", "HTTP Forward Proxy Started, listening on :"+strconv.Itoa(h.Port), nil)
		} else {
			err = h.Stop()
			if err != nil {
				h.logger.PrintAndLog("Forward Proxy", "Unable to stop forward proxy server", err)
				utils.SendErrorResponse(w, err.Error())
				return
			}
			h.logger.PrintAndLog("Forward Proxy", "HTTP Forward Proxy Stopped", nil)
		}
		h.db.Write("fwdproxy", "enabled", enabled)
		utils.SendOK(w)
	}
}
