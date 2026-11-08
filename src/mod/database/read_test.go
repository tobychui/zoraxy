//go:build !mipsle && !riscv64 && !fsdb

package database

import (
	"errors"
	"path/filepath"
	"testing"

	"imuslab.com/zoraxy/mod/database/dbinc"
)

func TestReadDistinguishesMissingCorruptAndUnavailable(t *testing.T) {
	for _, backend := range []dbinc.BackendType{dbinc.BackendBoltDB, dbinc.BackendLevelDB} {
		t.Run(backend.String(), func(t *testing.T) {
			db, err := NewDatabase(filepath.Join(t.TempDir(), "db"), backend)
			if err != nil {
				t.Fatal(err)
			}
			closed := false
			t.Cleanup(func() {
				if !closed {
					db.Close()
				}
			})
			var value string
			if err := db.Read("missing", "key", &value); !errors.Is(err, dbinc.ErrNotFound) {
				t.Fatalf("absent table: %v", err)
			}
			if err := db.NewTable("settings"); err != nil {
				t.Fatal(err)
			}
			if err := db.Read("settings", "missing", &value); !errors.Is(err, dbinc.ErrNotFound) {
				t.Fatalf("absent key: %v", err)
			}
			if err := db.Write("settings", "key", 123); err != nil {
				t.Fatal(err)
			}
			if err := db.Read("settings", "key", &value); err == nil || errors.Is(err, dbinc.ErrNotFound) {
				t.Fatalf("decode error: %v", err)
			}
			db.Close()
			closed = true
			if err := db.Read("settings", "key", &value); err == nil || errors.Is(err, dbinc.ErrNotFound) {
				t.Fatalf("closed database: %v", err)
			}
		})
	}
}
