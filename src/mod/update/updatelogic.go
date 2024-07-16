package update

import v308 "imuslab.com/zoraxy/mod/update/v308"

// Updater Core logic
func runUpdateRoutineWithVersion(fromVersion int, toVersion int) {
	if fromVersion == 307 && toVersion == 308 {
		//Updating from v3.0.7 to v3.0.8
		err := v308.UpdateFrom307To308()
		if err != nil {
			panic(err)
		}
	}

	//ADD MORE VERSIONS HERE
}
