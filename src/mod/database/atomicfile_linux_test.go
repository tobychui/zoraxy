//go:build linux && (mipsle || riscv64 || fsdb)

package database

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"imuslab.com/zoraxy/mod/database/dbinc"
)

func TestFilesystemNewRecordHonorsUmask(t *testing.T) {
	for _, mask := range []int{0022, 0077, 0777} {
		t.Run(fmt.Sprintf("%04o", mask), func(t *testing.T) {
			root := t.TempDir()
			db := &Database{Db: root, BackendType: dbinc.BackendFSOnly}
			if err := db.NewTable("oauth2"); err != nil {
				t.Fatal(err)
			}
			oldMask := syscall.Umask(mask)
			defer syscall.Umask(oldMask)
			reference := filepath.Join(root, "reference")
			if err := os.WriteFile(reference, []byte("old writer"), 0755); err != nil {
				t.Fatal(err)
			}
			if err := db.Write("oauth2", "settings", map[string]string{"secret": "secret"}); err != nil {
				t.Fatal(err)
			}
			oldInfo, err := os.Stat(reference)
			if err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(filepath.Join(root, "oauth2", "settings.entry"))
			if err != nil {
				t.Fatal(err)
			}
			if got, want := info.Mode().Perm(), oldInfo.Mode().Perm(); got & ^want != 0 {
				t.Fatalf("new record grants permissions beyond the old writer: got %04o, old %04o", got, want)
			}
		})
	}
}

func TestFilesystemReplacementPreservesExistingMode(t *testing.T) {
	for _, mode := range []os.FileMode{0000, 0600, 0640} {
		t.Run(fmt.Sprintf("%04o", mode), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "record")
			if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(path, mode); err != nil {
				t.Fatal(err)
			}
			oldMask := syscall.Umask(0077)
			defer syscall.Umask(oldMask)
			if err := atomicWriteFile(path, []byte("new")); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			if info.Mode().Perm() != mode {
				t.Fatalf("existing permissions changed: got %04o, want %04o", info.Mode().Perm(), mode)
			}
		})
	}
}
