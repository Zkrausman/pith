package config

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestMigrateStorageNeverMutatesFiles(t *testing.T) {
	for _, targetKind := range []string{"missing", "existing", "file", "same", "dot", "relative", "case", "symlink"} {
		t.Run(targetKind, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("PITH_STORAGE", filepath.Join(home, "selected"))
			legacy := filepath.Join(home, ".pith")
			if err := os.MkdirAll(legacy, 0700); err != nil {
				t.Fatal(err)
			}
			files := []string{"pith.db", "pith.db-wal", "pith.db-shm", "config.json", "pith.db.bak", "config.json.bak"}
			before := make(map[string]os.FileInfo)
			for _, name := range files {
				path := filepath.Join(legacy, name)
				if err := os.WriteFile(path, []byte("source-"+name), 0600); err != nil {
					t.Fatal(err)
				}
				info, err := os.Stat(path)
				if err != nil {
					t.Fatal(err)
				}
				before[name] = info
			}
			target := filepath.Join(home, "target")
			same := false
			switch targetKind {
			case "same":
				target, same = legacy, true
			case "dot":
				target, same = legacy+string(os.PathSeparator)+".", true
			case "relative":
				t.Chdir(home)
				target, same = ".pith", true
			case "case":
				if runtime.GOOS != "windows" {
					t.Skip("Windows case alias")
				}
				target, same = strings.ToUpper(legacy), true
			case "symlink":
				if err := os.Symlink(legacy, target); err != nil {
					t.Skipf("symlink unavailable: %v", err)
				}
				same = true
			case "existing":
				if err := os.Mkdir(target, 0700); err != nil {
					t.Fatal(err)
				}
				for _, name := range files {
					if err := os.WriteFile(filepath.Join(target, name), []byte("destination-"+name), 0600); err != nil {
						t.Fatal(err)
					}
				}
			case "file":
				if err := os.WriteFile(target, []byte("destination"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err := MigrateStorage(target)
			if same && err != nil {
				t.Fatal(err)
			}
			if !same && err == nil {
				t.Fatal("distinct target must require deliberate migration")
			}
			for _, name := range files {
				path := filepath.Join(legacy, name)
				data, err := os.ReadFile(path)
				if err != nil || string(data) != "source-"+name {
					t.Fatalf("source changed: %s %v", name, err)
				}
				info, err := os.Stat(path)
				if err != nil || !info.ModTime().Equal(before[name].ModTime()) {
					t.Fatalf("source metadata changed: %s %v", name, err)
				}
				if targetKind == "existing" {
					data, err := os.ReadFile(filepath.Join(target, name))
					if err != nil || string(data) != "destination-"+name {
						t.Fatalf("destination changed: %s %v", name, err)
					}
				}
			}
			if targetKind == "missing" {
				if _, err := os.Stat(target); !os.IsNotExist(err) {
					t.Fatalf("created target: %v", err)
				}
			}
			if targetKind == "file" {
				data, err := os.ReadFile(target)
				if err != nil || string(data) != "destination" {
					t.Fatalf("target file changed: %v", err)
				}
			}
		})
	}
}

func TestMigrateStorageMissingAndErrors(t *testing.T) {
	for _, kind := range []string{"no-directory", "empty", "legacy-file", "config-directory", "wal-only"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("PITH_STORAGE", filepath.Join(home, "selected"))
			legacy, target := filepath.Join(home, ".pith"), filepath.Join(home, "target")
			if kind == "legacy-file" {
				if err := os.WriteFile(legacy, []byte("file"), 0600); err != nil {
					t.Fatal(err)
				}
			} else if kind != "no-directory" {
				if err := os.Mkdir(legacy, 0700); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "config-directory" {
				if err := os.Mkdir(filepath.Join(legacy, "config.json"), 0700); err != nil {
					t.Fatal(err)
				}
			}
			if kind == "wal-only" {
				if err := os.WriteFile(filepath.Join(legacy, "pith.db-wal"), []byte("wal"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			err := MigrateStorage(target)
			wantError := kind != "no-directory" && kind != "empty"
			if (err != nil) != wantError {
				t.Fatalf("error=%v wantError=%v", err, wantError)
			}
		})
	}
}
