//go:build !mipsle && !riscv64
// +build !mipsle,!riscv64

package database

import (
	"errors"

	"aroz.org/zoraxy/ztnc/mod/database/dbbolt"
	"aroz.org/zoraxy/ztnc/mod/database/dbinc"
	"aroz.org/zoraxy/ztnc/mod/database/dbleveldb"
)

func newDatabase(dbfile string, backendType dbinc.BackendType) (*Database, error) {
	if backendType == dbinc.BackendFSOnly {
		return nil, errors.New("Unsupported backend type for this platform")
	}

	if backendType == dbinc.BackendLevelDB {
		db, err := dbleveldb.NewDB(dbfile)
		return &Database{
			Db:          nil,
			BackendType: backendType,
			Backend:     db,
		}, err
	}

	db, err := dbbolt.NewBoltDatabase(dbfile)
	return &Database{
		Db:          nil,
		BackendType: backendType,
		Backend:     db,
	}, err
}

func (d *Database) newTable(tableName string) error {
	return d.Backend.NewTable(tableName)
}

func (d *Database) tableExists(tableName string) bool {
	return d.Backend.TableExists(tableName)
}

func (d *Database) dropTable(tableName string) error {
	return d.Backend.DropTable(tableName)
}

func (d *Database) write(tableName string, key string, value interface{}) error {
	return d.Backend.Write(tableName, key, value)
}

func (d *Database) read(tableName string, key string, assignee interface{}) error {
	return d.Backend.Read(tableName, key, assignee)
}

func (d *Database) keyExists(tableName string, key string) bool {
	return d.Backend.KeyExists(tableName, key)
}

func (d *Database) delete(tableName string, key string) error {
	return d.Backend.Delete(tableName, key)
}

func (d *Database) listTable(tableName string) ([][][]byte, error) {
	return d.Backend.ListTable(tableName)
}

func (d *Database) close() {
	d.Backend.Close()
}
