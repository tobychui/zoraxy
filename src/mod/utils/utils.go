package utils

import (
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

/*
	Common

	Some commonly used functions in ArozOS

*/

// Response related
func SendTextResponse(w http.ResponseWriter, msg string) {
	w.Write([]byte(msg))
}

// Send JSON response, with an extra json header
func SendJSONResponse(w http.ResponseWriter, json string) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(json))
}

func SendErrorResponse(w http.ResponseWriter, errMsg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{\"error\":\"" + errMsg + "\"}"))
}

func SendOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("\"OK\""))
}

// Get GET parameter
func GetPara(r *http.Request, key string) (string, error) {
	keys, ok := r.URL.Query()[key]
	if !ok || len(keys[0]) < 1 {
		return "", errors.New("invalid " + key + " given")
	} else {
		return keys[0], nil
	}
}

// Get POST paramter
func PostPara(r *http.Request, key string) (string, error) {
	r.ParseForm()
	x := r.Form.Get(key)
	if x == "" {
		return "", errors.New("invalid " + key + " given")
	} else {
		return x, nil
	}
}

// Get POST paramter as boolean, accept 1 or true
func PostBool(r *http.Request, key string) (bool, error) {
	x, err := PostPara(r, key)
	if err != nil {
		return false, err
	}

	x = strings.TrimSpace(x)

	if x == "1" || strings.ToLower(x) == "true" || strings.ToLower(x) == "on" {
		return true, nil
	} else if x == "0" || strings.ToLower(x) == "false" || strings.ToLower(x) == "off" {
		return false, nil
	}

	return false, errors.New("invalid boolean given")
}

// Get POST paramter as int
func PostInt(r *http.Request, key string) (int, error) {
	x, err := PostPara(r, key)
	if err != nil {
		return 0, err
	}

	x = strings.TrimSpace(x)
	rx, err := strconv.Atoi(x)
	if err != nil {
		return 0, err
	}

	return rx, nil
}

func FileExists(filename string) bool {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return true
}

func IsDir(path string) bool {
	if FileExists(path) == false {
		return false
	}
	fi, err := os.Stat(path)
	if err != nil {
		log.Fatal(err)
		return false
	}
	switch mode := fi.Mode(); {
	case mode.IsDir():
		return true
	case mode.IsRegular():
		return false
	}
	return false
}

func TimeToString(targetTime time.Time) string {
	return targetTime.Format("2006-01-02 15:04:05")
}

// Check if given string in a given slice
func StringInArray(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return true
		}
	}
	return false
}

func StringInArrayIgnoreCase(arr []string, str string) bool {
	smallArray := []string{}
	for _, item := range arr {
		smallArray = append(smallArray, strings.ToLower(item))
	}

	return StringInArray(smallArray, strings.ToLower(str))
}
