package utils

import (
	"bufio"
	"encoding/base64"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
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

/*
	The paramter move function (mv)

	You can find similar things in the PHP version of ArOZ Online Beta. You need to pass in
	r (HTTP Request Object)
	getParamter (string, aka $_GET['This string])

	Will return
	Paramter string (if any)
	Error (if error)

*/
/*
func Mv(r *http.Request, getParamter string, postMode bool) (string, error) {
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
*/

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

func LoadImageAsBase64(filepath string) (string, error) {
	if !FileExists(filepath) {
		return "", errors.New("File not exists")
	}
	f, _ := os.Open(filepath)
	reader := bufio.NewReader(f)
	content, _ := io.ReadAll(reader)
	encoded := base64.StdEncoding.EncodeToString(content)
	return string(encoded), nil
}

// Use for redirections
func ConstructRelativePathFromRequestURL(requestURI string, redirectionLocation string) string {
	if strings.Count(requestURI, "/") == 1 {
		//Already root level
		return redirectionLocation
	}
	for i := 0; i < strings.Count(requestURI, "/")-1; i++ {
		redirectionLocation = "../" + redirectionLocation
	}

	return redirectionLocation
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
