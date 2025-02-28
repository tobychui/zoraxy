package dbbolt_test

import (
	"os"
	"testing"

	"aroz.org/zoraxy/ztnc/mod/database/dbbolt"
)

func TestNewBoltDatabase(t *testing.T) {
	dbfile := "test.db"
	defer os.Remove(dbfile)

	db, err := dbbolt.NewBoltDatabase(dbfile)
	if err != nil {
		t.Fatalf("Failed to create new Bolt database: %v", err)
	}
	defer db.Close()

	if db.Db == nil {
		t.Fatalf("Expected non-nil database object")
	}
}

func TestNewTable(t *testing.T) {
	dbfile := "test.db"
	defer os.Remove(dbfile)

	db, err := dbbolt.NewBoltDatabase(dbfile)
	if err != nil {
		t.Fatalf("Failed to create new Bolt database: %v", err)
	}
	defer db.Close()

	err = db.NewTable("testTable")
	if err != nil {
		t.Fatalf("Failed to create new table: %v", err)
	}
}

func TestTableExists(t *testing.T) {
	dbfile := "test.db"
	defer os.Remove(dbfile)

	db, err := dbbolt.NewBoltDatabase(dbfile)
	if err != nil {
		t.Fatalf("Failed to create new Bolt database: %v", err)
	}
	defer db.Close()

	tableName := "testTable"
	err = db.NewTable(tableName)
	if err != nil {
		t.Fatalf("Failed to create new table: %v", err)
	}

	exists := db.TableExists(tableName)
	if !exists {
		t.Fatalf("Expected table %s to exist", tableName)
	}

	nonExistentTable := "nonExistentTable"
	exists = db.TableExists(nonExistentTable)
	if exists {
		t.Fatalf("Expected table %s to not exist", nonExistentTable)
	}
}
