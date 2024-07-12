package update

/*
	Update.go

	This module handle cross version updates that contains breaking changes
	update command should always exit after the update is completed
*/

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	v308 "imuslab.com/zoraxy/mod/update/v308"
	"imuslab.com/zoraxy/mod/utils"
)

// Run config update. Version numbers are int. For example
// to update 3.0.7 to 3.0.8, use RunConfigUpdate(307, 308)
// This function support cross versions updates (e.g. 307 -> 310)
func RunConfigUpdate(fromVersion int, toVersion int) {
	versionFile := "./conf/version"
	if fromVersion == 0 {
		//Run auto previous version detection
		fromVersion = 307
		if utils.FileExists(versionFile) {
			//Read the version file
			previousVersionText, err := os.ReadFile(versionFile)
			if err != nil {
				panic("Unable to read version file at " + versionFile)
			}

			//Convert the version to int
			versionInt, err := strconv.Atoi(strings.TrimSpace(string(previousVersionText)))
			if err != nil {
				panic("Unable to read version file at " + versionFile)
			}

			fromVersion = versionInt
		}

		if fromVersion == toVersion {
			//No need to update
			return
		}
	}

	//Do iterate update
	for i := fromVersion; i < toVersion; i++ {
		oldVersion := fromVersion
		newVersion := fromVersion + 1
		fmt.Println("Updating from v", oldVersion, " to v", newVersion)
		runUpdateRoutineWithVersion(oldVersion, newVersion)
		//Write the updated version to file
		os.WriteFile(versionFile, []byte(strconv.Itoa(newVersion)), 0775)
	}
	fmt.Println("Update completed")
}

func GetVersionIntFromVersionNumber(version string) int {
	versionNumberOnly := strings.ReplaceAll(version, ".", "")
	versionInt, _ := strconv.Atoi(versionNumberOnly)
	return versionInt
}

func runUpdateRoutineWithVersion(fromVersion int, toVersion int) {
	if fromVersion == 307 && toVersion == 308 {
		//Updating from v3.0.7 to v3.0.8
		err := v308.UpdateFrom307To308()
		if err != nil {
			panic(err)
		}
	}
}
