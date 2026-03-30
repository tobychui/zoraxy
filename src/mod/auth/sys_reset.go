package auth

import (
	"fmt"

	"imuslab.com/zoraxy/mod/database"
)

/* Handle account reset by removing all users from the system */
func ResetAccount(databaseBackend *string, databasePath *string) {
	fmt.Println("Resetting admin account...")

	//Initialize the database to access the auth table
	backendType := database.GetRecommendedBackendType()
	if *databaseBackend != "auto" {
		switch *databaseBackend {
		case "leveldb":
			backendType = 2
		case "boltdb":
			backendType = 0
		case "fsdb":
			backendType = 1
		default:
			fmt.Println("Unknown database backend: " + *databaseBackend)
			fmt.Println("Using auto detected backend")
		}
	}

	db, err := database.NewDatabase(*databasePath, backendType)
	if err != nil {
		fmt.Println("Error opening database:", err)
		return
	}
	defer db.Close()

	//Check if auth table exists
	if !db.TableExists("auth") {
		fmt.Println("No auth table found. Nothing to reset.")
		return
	}

	//Drop the auth table to remove all users
	err = db.DropTable("auth")
	if err != nil {
		fmt.Println("Error dropping auth table:", err)
		return
	}

	fmt.Println("Successfully reset admin account. All users have been removed from the system.")
	fmt.Println("Please restart and create a new admin account.")
}
