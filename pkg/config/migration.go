package config

import (
	"fmt"
	"os"
	"path/filepath"
)

// MigrateStorage no longer copies or renames legacy storage. Raw file copies
// cannot safely migrate an active SQLite database (including its WAL).
//
// Deprecated: select the existing storage location instead. Deliberate migration
// requires a consistent database backup and must be planned separately.
func MigrateStorage(targetPath string) error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	oldPath := filepath.Join(home, ".pith")
	oldInfo, err := os.Stat(oldPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if !oldInfo.IsDir() {
		return fmt.Errorf("legacy storage is not a directory")
	}
	targetInfo, err := os.Stat(targetPath)
	if err == nil && os.SameFile(oldInfo, targetInfo) {
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	for _, name := range []string{"pith.db", "pith.db-wal", "pith.db-shm", "config.json"} {
		if _, err := os.Lstat(filepath.Join(oldPath, name)); err == nil {
			return fmt.Errorf("automatic legacy storage migration is disabled; select the existing storage location or arrange a consistent database backup before changing storage")
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}
