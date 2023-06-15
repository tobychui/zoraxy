package pathrule

import (
	"encoding/json"
	"net/http"
	"strconv"

	uuid "github.com/satori/go.uuid"
	"imuslab.com/zoraxy/mod/utils"
)

/*
	handler.go

	This script handles pathblock api
*/

func (h *Handler) HandleListBlockingPath(w http.ResponseWriter, r *http.Request) {
	js, _ := json.Marshal(h.BlockingPaths)
	utils.SendJSONResponse(w, string(js))
}

func (h *Handler) HandleAddBlockingPath(w http.ResponseWriter, r *http.Request) {
	matchingPath, err := utils.PostPara(r, "matchingPath")
	if err != nil {
		utils.SendErrorResponse(w, "invalid matching path given")
		return
	}

	exactMatch, err := utils.PostPara(r, "exactMatch")
	if err != nil {
		utils.SendErrorResponse(w, "invalid exact match value given")
		return
	}

	statusCodeString, err := utils.PostPara(r, "statusCode")
	if err != nil {
		utils.SendErrorResponse(w, "invalid status code given")
		return
	}

	statusCode, err := strconv.Atoi(statusCodeString)
	if err != nil {
		utils.SendErrorResponse(w, "invalid status code given")
		return
	}

	enabled, err := utils.PostPara(r, "enabled")
	if err != nil {
		utils.SendErrorResponse(w, "invalid enabled value given")
		return
	}

	caseSensitive, err := utils.PostPara(r, "caseSensitive")
	if err != nil {
		utils.SendErrorResponse(w, "invalid case sensitive value given")
		return
	}

	targetBlockingPath := BlockingPath{
		UUID:          uuid.NewV4().String(),
		MatchingPath:  matchingPath,
		ExactMatch:    exactMatch == "true",
		StatusCode:    statusCode,
		CustomHeaders: http.Header{},
		CustomHTML:    []byte(""),
		Enabled:       enabled == "true",
		CaseSenitive:  caseSensitive == "true",
	}

	err = h.AddBlockingPath(&targetBlockingPath)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}

	utils.SendOK(w)
}

func (h *Handler) HandleRemoveBlockingPath(w http.ResponseWriter, r *http.Request) {
	blockerUUID, err := utils.PostPara(r, "uuid")
	if err != nil {
		utils.SendErrorResponse(w, "invalid uuid given")
		return
	}

	targetRule := h.GetPathBlockerFromUUID(blockerUUID)
	if targetRule == nil {
		//Not found
		utils.SendErrorResponse(w, "target path blocker not found")
		return
	}

	err = h.RemoveBlockingPathByUUID(blockerUUID)
	if err != nil {
		utils.SendErrorResponse(w, err.Error())
		return
	}
	utils.SendOK(w)
}
