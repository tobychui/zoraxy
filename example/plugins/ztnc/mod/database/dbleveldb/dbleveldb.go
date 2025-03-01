package dbleveldb

import (
	"encoding/json"
	"log"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"aroz.org/zoraxy/ztnc/mod/database/dbinc"
	"github.com/syndtr/goleveldb/leveldb"
	"github.com/syndtr/goleveldb/leveldb/util"
)

// Ensure the DB struct implements the Backend interface
var _ dbinc.Backend = (*DB)(nil)

type DB struct {
	db               *leveldb.DB
	Table            sync.Map      //For emulating table creation
	batch            leveldb.Batch //Batch write
	writeFlushTicker *time.Ticker  //Ticker for flushing data into disk
	writeFlushStop   chan bool     //Stop channel for write flush ticker
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

	thisDB := &DB{
		db:    db,
		Table: sync.Map{},
		batch: leveldb.Batch{},
	}

	//Create a ticker to flush data into disk every 1 seconds
	writeFlushTicker := time.NewTicker(1 * time.Second)
	writeFlushStop := make(chan bool)
	go func() {
		for {
			select {
			case <-writeFlushTicker.C:
				if thisDB.batch.Len() == 0 {
					//No flushing needed
					continue
				}
				err = db.Write(&thisDB.batch, nil)
				if err != nil {
					log.Println("[LevelDB] Failed to flush data into disk: ", err)
				}
				thisDB.batch.Reset()
			case <-writeFlushStop:
				return
			}
		}
	}()

	thisDB.writeFlushTicker = writeFlushTicker
	thisDB.writeFlushStop = writeFlushStop

	return thisDB, nil
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
	d.batch.Put([]byte(filepath.ToSlash(filepath.Join(tableName, key))), data)
	return nil
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
	//Write the remaining data in batch back into disk
	d.writeFlushStop <- true
	d.writeFlushTicker.Stop()
	d.db.Write(&d.batch, nil)
	d.db.Close()
}
