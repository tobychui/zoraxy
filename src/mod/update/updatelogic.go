package update

import (
	v308 "imuslab.com/zoraxy/mod/update/v308"
	v315 "imuslab.com/zoraxy/mod/update/v315"
)

// Updater Core logic
func runUpdateRoutineWithVersion(fromVersion int, toVersion int) {
	if fromVersion == 307 && toVersion == 308 {
		//Updating from v3.0.7 to v3.0.8
		err := v308.UpdateFrom307To308()
		if err != nil {
			panic(err)
		}
	} else if fromVersion == 314 && toVersion == 315 {
		//Updating from v3.1.4 to v3.1.5
		err := v315.UpdateFrom314To315()
		if err != nil {
			panic(err)
		}
	}

	//ADD MORE VERSIONS HERE
}
