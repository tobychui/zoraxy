package utils

import (
	"errors"
	"log"
	"net"
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
	// Get first value from the URL query
	value := r.URL.Query().Get(key)
	if len(value) == 0 {
		return "", errors.New("invalid " + key + " given")
	}
	return value, nil
}

// Get GET paramter as boolean, accept 1 or true
func GetBool(r *http.Request, key string) (bool, error) {
	x, err := GetPara(r, key)
	if err != nil {
		return false, err
	}

	// Convert to lowercase and trim spaces just once to compare
	switch strings.ToLower(strings.TrimSpace(x)) {
	case "1", "true", "on":
		return true, nil
	case "0", "false", "off":
		return false, nil
	}

	return false, errors.New("invalid boolean given")
}

// Get POST parameter
func PostPara(r *http.Request, key string) (string, error) {
	// Try to parse the form
	if err := r.ParseForm(); err != nil {
		return "", err
	}
	// Get first value from the form
	x := r.Form.Get(key)
	if len(x) == 0 {
		return "", errors.New("invalid " + key + " given")
	}
	return x, nil
}

// Get POST paramter as boolean, accept 1 or true
func PostBool(r *http.Request, key string) (bool, error) {
	x, err := PostPara(r, key)
	if err != nil {
		return false, err
	}

	// Convert to lowercase and trim spaces just once to compare
	switch strings.ToLower(strings.TrimSpace(x)) {
	case "1", "true", "on":
		return true, nil
	case "0", "false", "off":
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
	if err == nil {
		// File exists
		return true
	} else if errors.Is(err, os.ErrNotExist) {
		// File does not exist
		return false
	}
	// Some other error
	return false
}

func IsDir(path string) bool {
	if !FileExists(path) {
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

// Validate if the listening address is correct
func ValidateListeningAddress(address string) bool {
	// Check if the address starts with a colon, indicating it's just a port
	if strings.HasPrefix(address, ":") {
		return true
	}

	// Split the address into host and port parts
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		// Try to parse it as just a port
		if _, err := strconv.Atoi(address); err == nil {
			return false // It's just a port number
		}
		return false // It's an invalid address
	}

	// Check if the port part is a valid number
	if _, err := strconv.Atoi(port); err != nil {
		return false
	}

	// Check if the host part is a valid IP address or empty (indicating any IP)
	if host != "" {
		if net.ParseIP(host) == nil {
			return false
		}
	}

	return true
}