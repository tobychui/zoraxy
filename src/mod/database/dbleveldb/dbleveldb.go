package dbleveldb

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"sync"

	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
	"imuslab.com/zoraxy/mod/database/dbinc"
)

// Ensure the DB struct implements the Backend interface
var _ dbinc.Backend = (*DB)(nil)

type DB struct {
	db    *leveldb.DB
	Table sync.Map //For emulating table creation
}

func NewDB(path string) (*DB, error) {
	//If the path is not a directory (e.g. /tmp/dbfile.db), convert the filename to directory
	if filepath.Ext(path) != "" {
		path = strings.ReplaceAll(path, ".", "_")
	}

	db, err := leveldb.OpenFile(path, nil)
	if err != nil {
		return nil, err
	}
	return &DB{db: db, Table: sync.Map{}}, nil
}

func (d *DB) NewTable(tableName string) error {
	//Create a table entry in the sync.Map
	d.Table.Store(tableName, true)
	return nil
}

func (d *DB) TableExists(tableName string) bool {
	_, ok := d.Table.Load(tableName)
	return ok
}

func (d *DB) DropTable(tableName string) error {
	d.Table.Delete(tableName)
	iter := d.db.NewIterator(nil, nil)
	defer iter.Release()

	for iter.Next() {
		key := iter.Key()
		if filepath.Dir(string(key)) == tableName {
			err := d.db.Delete(key, nil)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (d *DB) Write(tableName string, key string, value interface{}) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return d.db.Put([]byte(filepath.ToSlash(filepath.Join(tableName, key))), data, nil)
}

func (d *DB) Read(tableName string, key string, assignee interface{}) error {
	data, err := d.db.Get([]byte(filepath.ToSlash(filepath.Join(tableName, key))), nil)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, assignee)
}

func (d *DB) KeyExists(tableName string, key string) bool {
	_, err := d.db.Get([]byte(filepath.ToSlash(filepath.Join(tableName, key))), nil)
	return err == nil
}

func (d *DB) Delete(tableName string, key string) error {
	return d.db.Delete([]byte(filepath.ToSlash(filepath.Join(tableName, key))), nil)
}

func (d *DB) ListTable(tableName string) ([][][]byte, error) {
	iter := d.db.NewIterator(util.BytesPrefix([]byte(tableName+"/")), nil)
	defer iter.Release()

	var result [][][]byte
	for iter.Next() {
		key := iter.Key()
		//The key contains the table name as prefix. Trim it before returning
		value := iter.Value()
		result = append(result, [][]byte{[]byte(strings.TrimPrefix(string(key), tableName+"/")), value})
	}

	err := iter.Error()
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (d *DB) Close() {
	d.db.Close()
}
