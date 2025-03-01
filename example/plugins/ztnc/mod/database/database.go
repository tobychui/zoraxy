package database

/*
	ArOZ Online Database Access Module
	author: tobychui

	This is an improved Object oriented base solution to the original
	aroz online database script.
*/

import (
	"log"
	"runtime"

	"aroz.org/zoraxy/ztnc/mod/database/dbinc"
)

type Database struct {
	Db          interface{} //This will be nil on openwrt, leveldb.DB on x64 platforms or bolt.DB on other platforms
	BackendType dbinc.BackendType
	Backend     dbinc.Backend
}

func NewDatabase(dbfile string, backendType dbinc.BackendType) (*Database, error) {
	if runtime.GOARCH == "riscv64" {
		log.Println("RISCV hardware detected, ignoring the backend type and using FS emulated database")
	}
	return newDatabase(dbfile, backendType)
}

// Get the recommended backend type for the current system
func GetRecommendedBackendType() dbinc.BackendType {
	//Check if the system is running on RISCV hardware
	if runtime.GOARCH == "riscv64" {
		//RISCV hardware, currently only support FS emulated database
		return dbinc.BackendFSOnly
	} else if runtime.GOOS == "windows" || (runtime.GOOS == "linux" && runtime.GOARCH == "amd64") {
		//Powerful hardware
		return dbinc.BackendBoltDB
		//return dbinc.BackendLevelDB
	}

	//Default to BoltDB, the safest option
	return dbinc.BackendBoltDB
}

/*
	Create / Drop a table
	Usage:
	err := sysdb.NewTable("MyTable")
	err := sysdb.DropTable("MyTable")
*/

// Create a new table
func (d *Database) NewTable(tableName string) error {
	return d.newTable(tableName)
}

// Check is table exists
func (d *Database) TableExists(tableName string) bool {
	return d.tableExists(tableName)
}

// Drop the given table
func (d *Database) DropTable(tableName string) error {
	return d.dropTable(tableName)
}

/*
Write to database with given tablename and key. Example Usage:

	type demo struct{
		content string
	}

	thisDemo := demo{
		content: "Hello World",
	}

err := sysdb.Write("MyTable", "username/message",thisDemo);
*/
func (d *Database) Write(tableName string, key string, value interface{}) error {
	return d.write(tableName, key, value)
}

/*
	Read from database and assign the content to a given datatype. Example Usage:

	type demo struct{
		content string
	}
	thisDemo := new(demo)
	err := sysdb.Read("MyTable", "username/message",&thisDemo);
*/

func (d *Database) Read(tableName string, key string, assignee interface{}) error {
	return d.read(tableName, key, assignee)
}

/*
Check if a key exists in the database table given tablename and key

	if sysdb.KeyExists("MyTable", "username/message"){
		log.Println("Key exists")
	}
*/
func (d *Database) KeyExists(tableName string, key string) bool {
	return d.keyExists(tableName, key)
}

/*
Delete a value from the database table given tablename and key

err := sysdb.Delete("MyTable", "username/message");
*/
func (d *Database) Delete(tableName string, key string) error {
	return d.delete(tableName, key)
}

/*
	//List table example usage
	//Assume the value is stored as a struct named "groupstruct"

	entries, err := sysdb.ListTable("test")
	if err != nil {
		panic(err)
	}
	for _, keypairs := range entries{
		log.Println(string(keypairs[0]))
		group := new(groupstruct)
		json.Unmarshal(keypairs[1], &group)
		log.Println(group);
	}

*/

func (d *Database) ListTable(tableName string) ([][][]byte, error) {
	return d.listTable(tableName)
}

/*
Close the database connection
*/
func (d *Database) Close() {
	d.close()
}
