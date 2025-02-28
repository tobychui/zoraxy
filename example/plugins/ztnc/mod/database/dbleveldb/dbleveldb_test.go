package dbleveldb_test

import (
	"os"
	"testing"

	"aroz.org/zoraxy/ztnc/mod/database/dbleveldb"
)

func TestNewDB(t *testing.T) {
	path := "/tmp/testdb"
	defer os.RemoveAll(path)

	db, err := dbleveldb.NewDB(path)
	if err != nil {
		t.Fatalf("Failed to create new DB: %v", err)
	}
	defer db.Close()
}

func TestNewTable(t *testing.T) {
	path := "/tmp/testdb"
	defer os.RemoveAll(path)

	db, err := dbleveldb.NewDB(path)
	if err != nil {
		t.Fatalf("Failed to create new DB: %v", err)
	}
	defer db.Close()

	err = db.NewTable("testTable")
	if err != nil {
		t.Fatalf("Failed to create new table: %v", err)
	}
}

func TestTableExists(t *testing.T) {
	path := "/tmp/testdb"
	defer os.RemoveAll(path)

	db, err := dbleveldb.NewDB(path)
	if err != nil {
		t.Fatalf("Failed to create new DB: %v", err)
	}
	defer db.Close()

	db.NewTable("testTable")
	if !db.TableExists("testTable") {
		t.Fatalf("Table should exist")
	}
}

func TestDropTable(t *testing.T) {
	path := "/tmp/testdb"
	defer os.RemoveAll(path)

	db, err := dbleveldb.NewDB(path)
	if err != nil {
		t.Fatalf("Failed to create new DB: %v", err)
	}
	defer db.Close()

	db.NewTable("testTable")
	err = db.DropTable("testTable")
	if err != nil {
		t.Fatalf("Failed to drop table: %v", err)
	}

	if db.TableExists("testTable") {
		t.Fatalf("Table should not exist")
	}
}

func TestWriteAndRead(t *testing.T) {
	path := "/tmp/testdb"
	defer os.RemoveAll(path)

	db, err := dbleveldb.NewDB(path)
	if err != nil {
		t.Fatalf("Failed to create new DB: %v", err)
	}
	defer db.Close()

	db.NewTable("testTable")
	err = db.Write("testTable", "testKey", "testValue")
	if err != nil {
		t.Fatalf("Failed to write to table: %v", err)
	}

	var value string
	err = db.Read("testTable", "testKey", &value)
	if err != nil {
		t.Fatalf("Failed to read from table: %v", err)
	}

	if value != "testValue" {
		t.Fatalf("Expected 'testValue', got '%v'", value)
	}
}
func TestListTable(t *testing.T) {
	path := "/tmp/testdb"
	defer os.RemoveAll(path)

	db, err := dbleveldb.NewDB(path)
	if err != nil {
		t.Fatalf("Failed to create new DB: %v", err)
	}
	defer db.Close()

	db.NewTable("testTable")
	err = db.Write("testTable", "testKey1", "testValue1")
	if err != nil {
		t.Fatalf("Failed to write to table: %v", err)
	}
	err = db.Write("testTable", "testKey2", "testValue2")
	if err != nil {
		t.Fatalf("Failed to write to table: %v", err)
	}

	result, err := db.ListTable("testTable")
	if err != nil {
		t.Fatalf("Failed to list table: %v", err)
	}

	if len(result) != 2 {
		t.Fatalf("Expected 2 entries, got %v", len(result))
	}

	expected := map[string]string{
		"testTable/testKey1": "\"testValue1\"",
		"testTable/testKey2": "\"testValue2\"",
	}

	for _, entry := range result {
		key := string(entry[0])
		value := string(entry[1])
		if expected[key] != value {
			t.Fatalf("Expected value '%v' for key '%v', got '%v'", expected[key], key, value)
		}
	}
}
