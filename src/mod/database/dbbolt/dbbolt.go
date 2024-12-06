package dbbolt

import (
	"encoding/json"
	"errors"

	"github.com/boltdb/bolt"
)

type Database struct {
	Db interface{} //This is the bolt database object
}

func NewBoltDatabase(dbfile string) (*Database, error) {
	db, err := bolt.Open(dbfile, 0600, nil)
	if err != nil {
		return nil, err
	}

	return &Database{
		Db: db,
	}, err
}

// Create a new table
func (d *Database) NewTable(tableName string) error {
	err := d.Db.(*bolt.DB).Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(tableName))
		if err != nil {
			return err
		}
		return nil
	})

	return err
}

// Check is table exists
func (d *Database) TableExists(tableName string) bool {
	return d.Db.(*bolt.DB).View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(tableName))
		if b == nil {
			return errors.New("table not exists")
		}
		return nil
	}) == nil
}

// Drop the given table
func (d *Database) DropTable(tableName string) error {
	err := d.Db.(*bolt.DB).Update(func(tx *bolt.Tx) error {
		err := tx.DeleteBucket([]byte(tableName))
		if err != nil {
			return err
		}
		return nil
	})
	return err
}

// Write to table
func (d *Database) Write(tableName string, key string, value interface{}) error {
	jsonString, err := json.Marshal(value)
	if err != nil {
		return err
	}
	err = d.Db.(*bolt.DB).Update(func(tx *bolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte(tableName))
		if err != nil {
			return err
		}
		b := tx.Bucket([]byte(tableName))
		err = b.Put([]byte(key), jsonString)
		return err
	})
	return err
}

func (d *Database) Read(tableName string, key string, assignee interface{}) error {
	err := d.Db.(*bolt.DB).View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(tableName))
		v := b.Get([]byte(key))
		json.Unmarshal(v, &assignee)
		return nil
	})
	return err
}

func (d *Database) KeyExists(tableName string, key string) bool {
	resultIsNil := false
	if !d.TableExists(tableName) {
		//Table not exists. Do not proceed accessing key
		//log.Println("[DB] ERROR: Requesting key from table that didn't exist!!!")
		return false
	}
	err := d.Db.(*bolt.DB).View(func(tx *bolt.Tx) error {
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

func (d *Database) Delete(tableName string, key string) error {
	err := d.Db.(*bolt.DB).Update(func(tx *bolt.Tx) error {
		tx.Bucket([]byte(tableName)).Delete([]byte(key))
		return nil
	})

	return err
}

func (d *Database) ListTable(tableName string) ([][][]byte, error) {
	var results [][][]byte
	err := d.Db.(*bolt.DB).View(func(tx *bolt.Tx) error {
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
	d.Db.(*bolt.DB).Close()
}
