package update

/*
	Update.go

	This module handle cross version updates that contains breaking changes
	update command should always exit after the update is completed
*/

import (
	"fmt"
	"strconv"
	"strings"

	v308 "imuslab.com/zoraxy/mod/update/v308"
)

// Run config update. Version numbers are int. For example
// to update 3.0.7 to 3.0.8, use RunConfigUpdate(307, 308)
// This function support cross versions updates (e.g. 307 -> 310)
func RunConfigUpdate(fromVersion int, toVersion int) {
	for i := fromVersion; i < toVersion; i++ {
		oldVersion := fromVersion
		newVersion := fromVersion + 1
		fmt.Println("Updating from v", oldVersion, " to v", newVersion)
		runUpdateRoutineWithVersion(oldVersion, newVersion)
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
		err := v308.UpdateFrom307To308()
		if err != nil {
			panic(err)
		}
	}
}
