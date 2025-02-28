//go:build linux
// +build linux

package ganserv

import (
	"os"
	"os/exec"
	"os/user"
	"strings"

	"aroz.org/zoraxy/ztnc/mod/utils"
)

func readAuthTokenAsAdmin() (string, error) {
	if utils.FileExists("./conf/authtoken.secret") {
		authKey, err := os.ReadFile("./conf/authtoken.secret")
		if err == nil {
			return strings.TrimSpace(string(authKey)), nil
		}
	}

	cmd := exec.Command("sudo", "cat", "/var/lib/zerotier-one/authtoken.secret")
	output, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(output), nil
}

func isAdmin() bool {
	currentUser, err := user.Current()
	if err != nil {
		return false
	}
	return currentUser.Username == "root"
}
