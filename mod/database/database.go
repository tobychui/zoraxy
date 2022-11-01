package database

/*
	ArOZ Online Database Access Module
	author: tobychui

	This is an improved Object oriented base solution to the original
	aroz online database script.
*/

import (
	"encoding/json"
	"errors"
	"log"
	"sync"

	"github.com/boltdb/bolt"
)

type Database struct {
	Db       *bolt.DB
	Tables   sync.Map
	ReadOnly bool
}

func NewDatabase(dbfile string, readOnlyMode bool) (*Database, error) {
	db, err := bolt.Open(dbfile, 0600, nil)
	log.Println("Key-value Database Service Started: " + dbfile)

	tableMap := sync.Map{}
	//Build the table list from database
	err = db.View(func(tx *bolt.Tx) error {
		return tx.ForEach(func(name []byte, _ *bolt.Bucket) error {
			tableMap.Store(string(name), "")
			return nil
		})
	})

	return &Database{
		Db:       db,
		Tables:   tableMap,
		ReadOnly: readOnlyMode,
	}, err
}

/*
	Create / Drop a table
	Usage:
	err := sysdb.NewTable("MyTable")
	err := sysdb.DropTable("MyTable")
*/

func (d *Database) UpdateReadWriteMode(readOnly bool) {
	d.ReadOnly = readOnly
}

//Dump the whole db into a log file
func (d *Database) Dump(filename string) ([]string, error) {
	results := []string{}

	d.Tables.Range(func(tableName, v interface{}) bool {
		entries, err := d.ListTable(tableName.(string))
		if err != nil {
			log.Println("Reading table " + tableName.(string) + " failed: " + err.Error())
			return false
		}
		for _, keypairs := range entries {
			results = append(results, string(keypairs[0])+":"+string(keypairs[1])+"\n")
		}
		return true
	})

	return results, nil
}

//Create a new table
func (d *Database) NewTable(tableName string) error {
	if d.ReadOnly == true {
		return errors.New("Operation rejected in ReadOnly mode")
	}

	err := d.Db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(tableName))
		if err != nil {
			return err
		}
		return nil
	})

	d.Tables.Store(tableName, "")
	return err
}

//Check is table exists
func (d *Database) TableExists(tableName string) bool {
	if _, ok := d.Tables.Load(tableName); ok {
		return true
	}
	return false
}

//Drop the given table
func (d *Database) DropTable(tableName string) error {
	if d.ReadOnly == true {
		return errors.New("Operation rejected in ReadOnly mode")
	}

	err := d.Db.Update(func(tx *bolt.Tx) error {
		err := tx.DeleteBucket([]byte(tableName))
		if err != nil {
			return err
		}
		return nil
	})
	return err
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
	if d.ReadOnly == true {
		return errors.New("Operation rejected in ReadOnly mode")
	}

	jsonString, err := json.Marshal(value)
	if err != nil {
		return err
	}
	err = d.Db.Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(tableName))
		b := tx.Bucket([]byte(tableName))
		err = b.Put([]byte(key), jsonString)
		return err
	})
	return err
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
	err := d.Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(tableName))
		v := b.Get([]byte(key))
		json.Unmarshal(v, &assignee)
		return nil
	})
	return err
}

func (d *Database) KeyExists(tableName string, key string) bool {
	resultIsNil := false
	err := d.Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(tableName))
		v := b.Get([]byte(key))
		if v == nil {
			resultIsNil = true
		}
		return nil
	})

	if err != nil {
		return false
	} else {
		if resultIsNil {
			return false
		} else {
			return true
		}
	}
}

/*
	Delete a value from the database table given tablename and key

	err := sysdb.Delete("MyTable", "username/message");
*/
func (d *Database) Delete(tableName string, key string) error {
	if d.ReadOnly == true {
		return errors.New("Operation rejected in ReadOnly mode")
	}

	err := d.Db.Update(func(tx *bolt.Tx) error {
		tx.Bucket([]byte(tableName)).Delete([]byte(key))
		return nil
	})

	return err
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
	var results [][][]byte
	err := d.Db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(tableName))
		c := b.Cursor()

		for k, v := c.First(); k != nil; k, v = c.Next() {
			results = append(results, [][]byte{k, v})
		}
		return nil
	})
	return results, err
}

func (d *Database) Close() {
	d.Db.Close()
	return
}
