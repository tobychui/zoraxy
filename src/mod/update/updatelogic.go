package update

import (
	"fmt"

	v308 "imuslab.com/zoraxy/mod/update/v308"
	v315 "imuslab.com/zoraxy/mod/update/v315"
	v322 "imuslab.com/zoraxy/mod/update/v322"
	v334 "imuslab.com/zoraxy/mod/update/v334"
	v335 "imuslab.com/zoraxy/mod/update/v335"
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
	} else if fromVersion == 321 && toVersion == 322 {
		//Updating from v3.2.1 to v3.2.2
		err := v322.UpdateFrom321To322()
		if err != nil {
			panic(err)
		}
	} else if fromVersion == 333 && toVersion == 334 {
		//Updating from v3.3.3 to v3.3.4
		//Migrate legacy default.* cert files to name-based fallback system
		if err := v334.UpdateFrom333To334(); err != nil {
			fmt.Println("Warning: fallback cert migration failed (non-fatal):", err)
		}
	} else if fromVersion == 334 && toVersion == 335 {
		//Updating from v3.3.4 to v3.3.5
		//Migrate ACME cert JSON: recursive_ns → disable_recursive_nss_check
		if err := v335.UpdateFrom334To335(); err != nil {
			fmt.Println("Warning: ACME NSs check config migration failed (non-fatal):", err)
		}
	}

	//ADD MORE VERSIONS HERE
}
