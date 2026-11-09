//go:build mipsle || riscv64 || fsdb

package database

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"imuslab.com/zoraxy/mod/database/dbinc"
)

func TestFilesystemReadDistinguishesMissingCorruptAndUnavailable(t *testing.T) {
	db := &Database{Db: t.TempDir(), BackendType: dbinc.BackendFSOnly}
	if err := db.NewTable("oauth2"); err != nil {
		t.Fatal(err)
	}
	var value string
	if err := db.Read("oauth2", "missing", &value); !errors.Is(err, dbinc.ErrNotFound) {
		t.Fatalf("missing: %v", err)
	}
	if err := db.Write("oauth2", "settings", 123); err != nil {
		t.Fatal(err)
	}
	if err := db.Read("oauth2", "settings", &value); err == nil || errors.Is(err, dbinc.ErrNotFound) {
		t.Fatalf("decode: %v", err)
	}
	// A directory in place of a record is an actual filesystem read failure.
	path := filepath.Join(db.Db.(string), "oauth2", "settings.entry")
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := db.Read("oauth2", "settings", &value); err == nil || errors.Is(err, dbinc.ErrNotFound) {
		t.Fatalf("unreadable: %v", err)
	}
}

// Faults are injected at the file-operation boundary of the real filesystem
// replacement code, not through a BoltDB/LevelDB backend unused on these targets.
type faultyReplacementFile struct {
	*os.File
	failure string
}

var errReplacementTest = errors.New("injected filesystem failure")

func (f faultyReplacementFile) Write(p []byte) (int, error) {
	if f.failure == "write" || f.failure == "short-write" {
		n, err := f.File.Write(p[:len(p)/2])
		if err != nil {
			return n, err
		}
		if f.failure == "write" {
			return n, errReplacementTest
		}
		return n, nil
	}
	return f.File.Write(p)
}
func (f faultyReplacementFile) Chmod(mode os.FileMode) error {
	if f.failure == "chmod" {
		return errReplacementTest
	}
	return f.File.Chmod(mode)
}
func (f faultyReplacementFile) Sync() error {
	if f.failure == "sync" {
		return errReplacementTest
	}
	return f.File.Sync()
}
func (f faultyReplacementFile) Close() error {
	err := f.File.Close()
	if err != nil {
		return err
	}
	if f.failure == "close" {
		return errReplacementTest
	}
	return nil
}

func TestFilesystemFailedReplacementPreservesPreviousRecord(t *testing.T) {
	for _, failure := range []string{"create", "chmod", "write", "short-write", "sync", "close", "rename"} {
		t.Run(failure, func(t *testing.T) {
			root := t.TempDir()
			db := &Database{Db: root, BackendType: dbinc.BackendFSOnly}
			if err := db.NewTable("oauth2"); err != nil {
				t.Fatal(err)
			}
			old := map[string]string{"oauth2ClientId": "client", "oauth2DefaultAllowedGroups": "admins"}
			if err := db.Write("oauth2", "settings", old); err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "oauth2", "settings.entry")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			ops := replacementFileOps{
				createTemp: func(dir, pattern string) (replacementFile, error) {
					if failure == "create" {
						return nil, errReplacementTest
					}
					f, err := os.CreateTemp(dir, pattern)
					if err != nil {
						return nil, err
					}
					return faultyReplacementFile{f, failure}, nil
				},
				rename: func(src, dst string) error {
					if failure == "rename" {
						return errReplacementTest
					}
					return os.Rename(src, dst)
				},
				remove: os.Remove,
			}
			mode := os.FileMode(0755)
			err = replaceFile(path, []byte(`{"oauth2ClientId":"changed","oauth2DefaultAllowedGroups":""}`), &mode, ops)
			if !errors.Is(err, errReplacementTest) && !(failure == "short-write" && errors.Is(err, io.ErrShortWrite)) {
				t.Fatalf("unexpected replacement result: %v", err)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("old record damaged: %s, %v", after, err)
			}
			// Reopen through the actual filesystem database after the failed save.
			restarted := &Database{Db: root, BackendType: dbinc.BackendFSOnly}
			var got map[string]string
			if err := restarted.Read("oauth2", "settings", &got); err != nil || got["oauth2DefaultAllowedGroups"] != "admins" || got["oauth2ClientId"] != "client" {
				t.Fatalf("restart lost previous settings: %v, %v", got, err)
			}
			entries, err := os.ReadDir(filepath.Dir(path))
			if err != nil {
				t.Fatal(err)
			}
			if len(entries) != 1 || entries[0].Name() != "settings.entry" {
				t.Fatalf("temporary records were left behind: %v", entries)
			}
			if err := restarted.Write("oauth2", "settings", map[string]string{"oauth2DefaultAllowedGroups": "operators"}); err != nil {
				t.Fatal(err)
			}
			if err := db.Read("oauth2", "settings", &got); err != nil || got["oauth2DefaultAllowedGroups"] != "operators" {
				t.Fatalf("successful replacement failed: %v, %v", got, err)
			}
		})
	}
}
