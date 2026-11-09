//go:build mipsle || riscv64 || fsdb

package database

import (
	"errors"
	"io"
	"os"
	"path/filepath"
)

type replacementFile interface {
	io.Writer
	Name() string
	Chmod(os.FileMode) error
	Sync() error
	Close() error
}

type replacementFileOps struct {
	createTemp func(string, string) (replacementFile, error)
	rename     func(string, string) error
	remove     func(string) error
}

func atomicWriteFile(path string, data []byte) error {
	var mode *os.FileMode
	// Match WriteFile's treatment of an existing file's permissions.
	if info, err := os.Stat(path); err == nil {
		existingMode := info.Mode().Perm()
		mode = &existingMode
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return replaceFile(path, data, mode, replacementFileOps{
		createTemp: func(dir, pattern string) (replacementFile, error) { return os.CreateTemp(dir, pattern) },
		rename:     os.Rename,
		remove:     os.Remove,
	})
}

// On the target Unix platforms, a same-directory rename atomically replaces the
// old record. Never remove or truncate the destination before replacement.
// Sync the contents before publishing, but do not claim power-loss durability:
// the directory entry is not synced, and hardware/filesystem guarantees vary.
func replaceFile(path string, data []byte, mode *os.FileMode, ops replacementFileOps) error {
	file, err := ops.createTemp(filepath.Dir(path), ".db-write-*")
	if err != nil {
		return err
	}
	closed := false
	defer func() {
		if !closed {
			_ = file.Close()
		}
		_ = ops.remove(file.Name())
	}()
	// New records keep CreateTemp's private 0600 permissions, filtered by the
	// process umask. Chmod would bypass that mask. Only existing records need
	// their previously established permissions restored (including mode 0000).
	if mode != nil {
		if err := file.Chmod(*mode); err != nil {
			return err
		}
	}
	n, err := file.Write(data)
	if err != nil {
		return err
	}
	if n != len(data) {
		return io.ErrShortWrite
	}
	if err := file.Sync(); err != nil {
		return err
	}
	err = file.Close()
	closed = true
	if err != nil {
		return err
	}
	return ops.rename(file.Name(), path)
}
