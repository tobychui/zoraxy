package database

import (
	"encoding/json"
)

type BackupEntry struct {
	Key   string          `json:"key"`
	Value json.RawMessage `json:"value"`
}

type BackupData struct {
	Tables map[string][]BackupEntry `json:"tables"`
}

// Backup returns a JSON representation of the entire database
func (d *Database) Backup() ([]byte, error) {
	return d.BackupExcludeTables(nil)
}

// BackupExcludeTables returns a JSON representation of the database excluding the given tables.
func (d *Database) BackupExcludeTables(excludedTables []string) ([]byte, error) {
	tables, err := d.GetAllTables()
	if err != nil {
		return nil, err
	}

	excludedLookup := map[string]bool{}
	for _, tableName := range excludedTables {
		excludedLookup[tableName] = true
	}

	backup := BackupData{
		Tables: make(map[string][]BackupEntry),
	}

	for _, table := range tables {
		if excludedLookup[table] {
			continue
		}
		entries, err := d.ListTable(table)
		if err != nil {
			continue
		}

		var tableEntries []BackupEntry
		for _, entry := range entries {
			// entry[0] is key, entry[1] is value (both []byte)
			tableEntries = append(tableEntries, BackupEntry{
				Key:   string(entry[0]),
				Value: json.RawMessage(entry[1]),
			})
		}
		backup.Tables[table] = tableEntries
	}

	return json.MarshalIndent(backup, "", "  ")
}

// RestoreReplacePreservingTables replaces the database while keeping the existing content of the given tables.
func (d *Database) RestoreReplacePreservingTables(backupJSON []byte, preserveTables []string) error {
	preservedTables := map[string][]BackupEntry{}
	for _, tableName := range preserveTables {
		entries, err := d.ListTable(tableName)
		if err != nil {
			continue
		}

		clonedEntries := make([]BackupEntry, 0, len(entries))
		for _, entry := range entries {
			clonedEntries = append(clonedEntries, BackupEntry{
				Key:   string(entry[0]),
				Value: append(json.RawMessage{}, entry[1]...),
			})
		}
		preservedTables[tableName] = clonedEntries
	}

	var backup BackupData
	if err := json.Unmarshal(backupJSON, &backup); err != nil {
		return err
	}

	if backup.Tables == nil {
		backup.Tables = map[string][]BackupEntry{}
	}
	for tableName, entries := range preservedTables {
		backup.Tables[tableName] = entries
	}

	mergedBackup, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return err
	}

	return d.RestoreReplace(mergedBackup)
}

// Restore restores the database from a JSON representation
func (d *Database) Restore(backupJSON []byte) error {
	var backup BackupData
	err := json.Unmarshal(backupJSON, &backup)
	if err != nil {
		return err
	}

	// For each table in backup
	for tableName, entries := range backup.Tables {
		// Ensure table exists
		err = d.NewTable(tableName)
		if err != nil {
			return err
		}

		for _, entry := range entries {
			err = d.Write(tableName, entry.Key, entry.Value)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// RestoreReplace replaces all tables and entries in the database with the provided backup.
func (d *Database) RestoreReplace(backupJSON []byte) error {
	var backup BackupData
	if err := json.Unmarshal(backupJSON, &backup); err != nil {
		return err
	}

	currentTables, err := d.GetAllTables()
	if err != nil {
		return err
	}

	targetTables := map[string]bool{}
	for tableName := range backup.Tables {
		targetTables[tableName] = true
	}

	for _, tableName := range currentTables {
		if !targetTables[tableName] {
			if err := d.DropTable(tableName); err != nil {
				return err
			}
		}
	}

	for tableName, entries := range backup.Tables {
		if d.TableExists(tableName) {
			if err := d.DropTable(tableName); err != nil {
				return err
			}
		}

		if err := d.NewTable(tableName); err != nil {
			return err
		}

		for _, entry := range entries {
			if err := d.Write(tableName, entry.Key, entry.Value); err != nil {
				return err
			}
		}
	}

	return nil
}
