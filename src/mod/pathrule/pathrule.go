package pathrule

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"imuslab.com/zoraxy/mod/utils"
)

/*
	Pathrules.go

	This script handle advance path settings and rules on particular
	paths of the incoming requests
*/

type Options struct {
	Enabled      bool   //If the pathrule is enabled.
	ConfigFolder string //The folder to store the path blocking config files
}

type BlockingPath struct {
	UUID          string
	MatchingPath  string
	ExactMatch    bool
	StatusCode    int
	CustomHeaders http.Header
	CustomHTML    []byte
	Enabled       bool
	CaseSenitive  bool
}

type Handler struct {
	Options       *Options
	BlockingPaths []*BlockingPath
}

// Create a new path blocker handler
func NewPathRuleHandler(options *Options) *Handler {
	//Create folder if not exists
	if !utils.FileExists(options.ConfigFolder) {
		os.Mkdir(options.ConfigFolder, 0775)
	}

	//Load the configs from file
	//TODO

	return &Handler{
		Options:       options,
		BlockingPaths: []*BlockingPath{},
	}
}

func (h *Handler) ListBlockingPath() []*BlockingPath {
	return h.BlockingPaths
}

// Get the blocker from matching path (path match, ignore tailing slash)
func (h *Handler) GetPathBlockerFromMatchingPath(matchingPath string) *BlockingPath {
	for _, blocker := range h.BlockingPaths {
		if (blocker.MatchingPath == matchingPath) || (strings.TrimSuffix(blocker.MatchingPath, "/") == strings.TrimSuffix(matchingPath, "/")) {
			return blocker
		}
	}

	return nil
}

func (h *Handler) GetPathBlockerFromUUID(UUID string) *BlockingPath {
	for _, blocker := range h.BlockingPaths {
		if blocker.UUID == UUID {
			return blocker
		}
	}

	return nil
}

func (h *Handler) AddBlockingPath(pathBlocker *BlockingPath) error {
	//Check if the blocker exists
	blockerPath := pathBlocker.MatchingPath
	targetBlocker := h.GetPathBlockerFromMatchingPath(blockerPath)
	if targetBlocker != nil {
		//Blocker with the same matching path already exists
		return errors.New("path blocker with the same path already exists")
	}

	h.BlockingPaths = append(h.BlockingPaths, pathBlocker)

	//Write the new config to file
	return h.SaveBlockerToFile(pathBlocker)
}

func (h *Handler) RemoveBlockingPathByUUID(uuid string) error {
	newBlockingList := []*BlockingPath{}
	for _, thisBlocker := range h.BlockingPaths {
		if thisBlocker.UUID != uuid {
			newBlockingList = append(newBlockingList, thisBlocker)
		}
	}

	if len(h.BlockingPaths) == len(newBlockingList) {
		//Nothing is removed
		return errors.New("given matching path blocker not exists")
	}

	h.BlockingPaths = newBlockingList

	return h.RemoveBlockerFromFile(uuid)
}

func (h *Handler) SaveBlockerToFile(pathBlocker *BlockingPath) error {
	saveFilename := filepath.Join(h.Options.ConfigFolder, pathBlocker.UUID)
	js, _ := json.MarshalIndent(pathBlocker, "", " ")
	return os.WriteFile(saveFilename, js, 0775)
}

func (h *Handler) RemoveBlockerFromFile(uuid string) error {
	expectedConfigFile := filepath.Join(h.Options.ConfigFolder, uuid)
	if !utils.FileExists(expectedConfigFile) {
		return errors.New("config file not found on disk")
	}

	return os.Remove(expectedConfigFile)
}

// Get all the matching blockers for the given URL path
// return all the path blockers and the max length matching rule
func (h *Handler) GetMatchingBlockers(urlPath string) ([]*BlockingPath, *BlockingPath) {
	urlPath = strings.TrimSuffix(urlPath, "/")
	matchingBlockers := []*BlockingPath{}
	var longestMatchingPrefix *BlockingPath = nil
	for _, thisBlocker := range h.BlockingPaths {
		if !thisBlocker.Enabled {
			//This blocker is not enabled. Ignore this
			continue
		}

		incomingURLPath := urlPath
		matchingPath := strings.TrimSuffix(thisBlocker.MatchingPath, "/")

		if !thisBlocker.CaseSenitive {
			//This is not case sensitive
			incomingURLPath = strings.ToLower(incomingURLPath)
			matchingPath = strings.ToLower(matchingPath)
		}

		if matchingPath == incomingURLPath {
			//This blocker have exact url path match
			matchingBlockers = append(matchingBlockers, thisBlocker)
			if longestMatchingPrefix == nil || len(thisBlocker.MatchingPath) > len(longestMatchingPrefix.MatchingPath) {
				longestMatchingPrefix = thisBlocker
			}
			continue
		}

		if !thisBlocker.ExactMatch && strings.HasPrefix(incomingURLPath, matchingPath) {
			//This blocker have prefix url match
			matchingBlockers = append(matchingBlockers, thisBlocker)
			if longestMatchingPrefix == nil || len(thisBlocker.MatchingPath) > len(longestMatchingPrefix.MatchingPath) {
				longestMatchingPrefix = thisBlocker
			}
			continue
		}
	}

	return matchingBlockers, longestMatchingPrefix
}
