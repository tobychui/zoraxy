//go:build mipsle || riscv64
// +build mipsle riscv64

package database

import (
	"encoding/json"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

func newDatabase(dbfile string, readOnlyMode bool) (*Database, error) {
	dbRootPath := filepath.ToSlash(filepath.Clean(dbfile))
	dbRootPath = "fsdb/" + dbRootPath
	err := os.MkdirAll(dbRootPath, 0755)
	if err != nil {
		return nil, err
	}

	tableMap := sync.Map{}
	//build the table list from file system
	files, err := filepath.Glob(filepath.Join(dbRootPath, "/*"))
	if err != nil {
		return nil, err
	}

	for _, file := range files {
		if isDirectory(file) {
			tableMap.Store(filepath.Base(file), "")
		}
	}

	log.Println("Filesystem Emulated Key-value Database Service Started: " + dbRootPath)
	return &Database{
		Db:       dbRootPath,
		Tables:   tableMap,
		ReadOnly: readOnlyMode,
	}, nil
}

func (d *Database) dump(filename string) ([]string, error) {
	//Get all file objects from root
	rootfiles, err := filepath.Glob(filepath.Join(d.Db.(string), "/*"))
	if err != nil {
		return []string{}, err
	}

	//Filter out the folders
	rootFolders := []string{}
	for _, file := range rootfiles {
		if !isDirectory(file) {
			rootFolders = append(rootFolders, filepath.Base(file))
		}
	}

	return rootFolders, nil
}

func (d *Database) newTable(tableName string) error {
	if d.ReadOnly {
		return errors.New("Operation rejected in ReadOnly mode")
	}
	tablePath := filepath.Join(d.Db.(string), filepath.Base(tableName))
	if !fileExists(tablePath) {
		return os.MkdirAll(tablePath, 0755)
	}
	return nil
}

func (d *Database) tableExists(tableName string) bool {
	tablePath := filepath.Join(d.Db.(string), filepath.Base(tableName))
	if _, err := os.Stat(tablePath); errors.Is(err, os.ErrNotExist) {
		return false
	}

	if !isDirectory(tablePath) {
		return false
	}

	return true
}

func (d *Database) dropTable(tableName string) error {
	if d.ReadOnly {
		return errors.New("Operation rejected in ReadOnly mode")
	}
	tablePath := filepath.Join(d.Db.(string), filepath.Base(tableName))
	if d.tableExists(tableName) {
		return os.RemoveAll(tablePath)
	} else {
		return errors.New("table not exists")
	}

}

func (d *Database) write(tableName string, key string, value interface{}) error {
	if d.ReadOnly {
		return errors.New("Operation rejected in ReadOnly mode")
	}
	tablePath := filepath.Join(d.Db.(string), filepath.Base(tableName))
	js, err := json.Marshal(value)
	if err != nil {
		return err
	}

	key = strings.ReplaceAll(key, "/", "-SLASH_SIGN-")

	return os.WriteFile(filepath.Join(tablePath, key+".entry"), js, 0755)
}

func (d *Database) read(tableName string, key string, assignee interface{}) error {
	if !d.keyExists(tableName, key) {
		return errors.New("key not exists")
	}

	key = strings.ReplaceAll(key, "/", "-SLASH_SIGN-")

	tablePath := filepath.Join(d.Db.(string), filepath.Base(tableName))
	entryPath := filepath.Join(tablePath, key+".entry")
	content, err := os.ReadFile(entryPath)
	if err != nil {
		return err
	}

	err = json.Unmarshal(content, &assignee)
	return err
}

func (d *Database) keyExists(tableName string, key string) bool {
	key = strings.ReplaceAll(key, "/", "-SLASH_SIGN-")
	tablePath := filepath.Join(d.Db.(string), filepath.Base(tableName))
	entryPath := filepath.Join(tablePath, key+".entry")
	return fileExists(entryPath)
}

func (d *Database) delete(tableName string, key string) error {
	if d.ReadOnly {
		return errors.New("Operation rejected in ReadOnly mode")
	}
	if !d.keyExists(tableName, key) {
		return errors.New("key not exists")
	}
	key = strings.ReplaceAll(key, "/", "-SLASH_SIGN-")
	tablePath := filepath.Join(d.Db.(string), filepath.Base(tableName))
	entryPath := filepath.Join(tablePath, key+".entry")

	return os.Remove(entryPath)
}

func (d *Database) listTable(tableName string) ([][][]byte, error) {
	if !d.tableExists(tableName) {
		return [][][]byte{}, errors.New("table not exists")
	}
	tablePath := filepath.Join(d.Db.(string), filepath.Base(tableName))
	entries, err := filepath.Glob(filepath.Join(tablePath, "/*.entry"))
	if err != nil {
		return [][][]byte{}, err
	}

	var results [][][]byte = [][][]byte{}
	for _, entry := range entries {
		if !isDirectory(entry) {
			//Read it
			key := filepath.Base(entry)
			key = strings.TrimSuffix(key, filepath.Ext(key))
			key = strings.ReplaceAll(key, "-SLASH_SIGN-", "/")

			bkey := []byte(key)
			bval := []byte("")
			c, err := os.ReadFile(entry)
			if err != nil {
				break
			}

			bval = c
			results = append(results, [][]byte{bkey, bval})
		}
	}
	return results, nil
}

func (d *Database) close() {
	//Nothing to close as it is file system
}

func isDirectory(path string) bool {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false
	}

	return fileInfo.IsDir()
}

func fileExists(name string) bool {
	_, err := os.Stat(name)
	if err == nil {
		return true
	}
	if errors.Is(err, os.ErrNotExist) {
		return false
	}
	return false
}
