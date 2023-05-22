package ganserv

import (
	"errors"
	"log"
	"os"
	"runtime"
	"strings"
)

func TryLoadorAskUserForAuthkey() (string, error) {
	//Check for zt auth token
	value, exists := os.LookupEnv("ZT_AUTH")
	if !exists {
		log.Println("Environment variable ZT_AUTH not defined. Trying to load authtoken from file.")
	} else {
		return value, nil
	}

	authKey := ""
	if runtime.GOOS == "windows" {
		if isAdmin() {
			//Read the secret file directly
			b, err := os.ReadFile("C:\\ProgramData\\ZeroTier\\One\\authtoken.secret")
			if err == nil {
				log.Println("Zerotier authkey loaded")
				authKey = string(b)
			} else {
				log.Println("Unable to read authkey at C:\\ProgramData\\ZeroTier\\One\\authtoken.secret: ", err.Error())
			}
		} else {
			//Elavate the permission to admin
			ak, err := readAuthTokenAsAdmin()
			if err == nil {
				log.Println("Zerotier authkey loaded")
				authKey = ak
			} else {
				log.Println("Unable to read authkey at C:\\ProgramData\\ZeroTier\\One\\authtoken.secret: ", err.Error())
			}
		}

	} else if runtime.GOOS == "linux" {
		if isAdmin() {
			//Try to read from source using sudo
			ak, err := readAuthTokenAsAdmin()
			if err == nil {
				log.Println("Zerotier authkey loaded")
				authKey = strings.TrimSpace(ak)
			} else {
				log.Println("Unable to read authkey at /var/lib/zerotier-one/authtoken.secret: ", err.Error())
			}
		} else {
			//Try read from source
			b, err := os.ReadFile("/var/lib/zerotier-one/authtoken.secret")
			if err == nil {
				log.Println("Zerotier authkey loaded")
				authKey = string(b)
			} else {
				log.Println("Unable to read authkey at /var/lib/zerotier-one/authtoken.secret: ", err.Error())
			}
		}

	} else if runtime.GOOS == "darwin" {
		b, err := os.ReadFile("/Library/Application Support/ZeroTier/One/authtoken.secret")
		if err == nil {
			log.Println("Zerotier authkey loaded")
			authKey = string(b)
		} else {
			log.Println("Unable to read authkey at /Library/Application Support/ZeroTier/One/authtoken.secret ", err.Error())
		}
	}

	authKey = strings.TrimSpace(authKey)

	if authKey == "" {
		return "", errors.New("Unable to load authkey from file")
	}

	return authKey, nil
}
