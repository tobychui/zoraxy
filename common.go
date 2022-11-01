package main

import (
	"bufio"
	"encoding/base64"
	"errors"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

/*
	Basic Response Functions

	Send response with ease
*/
//Send text response with given w and message as string
func sendTextResponse(w http.ResponseWriter, msg string) {
	w.Write([]byte(msg))
}

//Send JSON response, with an extra json header
func sendJSONResponse(w http.ResponseWriter, json string) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(json))
}

func sendErrorResponse(w http.ResponseWriter, errMsg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("{\"error\":\"" + errMsg + "\"}"))
}

func sendOK(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte("\"OK\""))
}

/*
	The paramter move function (mv)

	You can find similar things in the PHP version of ArOZ Online Beta. You need to pass in
	r (HTTP Request Object)
	getParamter (string, aka $_GET['This string])

	Will return
	Paramter string (if any)
	Error (if error)

*/
func mv(r *http.Request, getParamter string, postMode bool) (string, error) {
	if postMode == false {
		//Access the paramter via GET
		keys, ok := r.URL.Query()[getParamter]

		if !ok || len(keys[0]) < 1 {
			//log.Println("Url Param " + getParamter +" is missing")
			return "", errors.New("GET paramter " + getParamter + " not found or it is empty")
		}

		// Query()["key"] will return an array of items,
		// we only want the single item.
		key := keys[0]
		return string(key), nil
	} else {
		//Access the parameter via POST
		r.ParseForm()
		x := r.Form.Get(getParamter)
		if len(x) == 0 || x == "" {
			return "", errors.New("POST paramter " + getParamter + " not found or it is empty")
		}
		return string(x), nil
	}

}

func stringInSlice(a string, list []string) bool {
	for _, b := range list {
		if b == a {
			return true
		}
	}
	return false
}

func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	if os.IsNotExist(err) {
		return false
	}
	return true
}

func IsDir(path string) bool {
	if fileExists(path) == false {
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

func inArray(arr []string, str string) bool {
	for _, a := range arr {
		if a == str {
			return true
		}
	}
	return false
}

func timeToString(targetTime time.Time) string {
	return targetTime.Format("2006-01-02 15:04:05")
}

func IntToString(number int) string {
	return strconv.Itoa(number)
}

func StringToInt(number string) (int, error) {
	return strconv.Atoi(number)
}

func StringToInt64(number string) (int64, error) {
	i, err := strconv.ParseInt(number, 10, 64)
	if err != nil {
		return -1, err
	}
	return i, nil
}

func Int64ToString(number int64) string {
	convedNumber := strconv.FormatInt(number, 10)
	return convedNumber
}

func GetUnixTime() int64 {
	return time.Now().Unix()
}

func LoadImageAsBase64(filepath string) (string, error) {
	if !fileExists(filepath) {
		return "", errors.New("File not exists")
	}
	f, _ := os.Open(filepath)
	reader := bufio.NewReader(f)
	content, _ := ioutil.ReadAll(reader)
	encoded := base64.StdEncoding.EncodeToString(content)
	return string(encoded), nil
}

//Get the IP address of the current authentication user
func getUserIPAddr(w http.ResponseWriter, r *http.Request) {
	requestPort, _ := mv(r, "port", false)
	showPort := false
	if requestPort == "true" {
		//Show port as well
		showPort = true
	}
	IPAddress := r.Header.Get("X-Real-Ip")
	if IPAddress == "" {
		IPAddress = r.Header.Get("X-Forwarded-For")
	}
	if IPAddress == "" {
		IPAddress = r.RemoteAddr
	}
	if !showPort {
		IPAddress = IPAddress[:strings.LastIndex(IPAddress, ":")]

	}
	w.Write([]byte(IPAddress))
	return
}
