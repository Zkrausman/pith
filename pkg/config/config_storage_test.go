package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// Isolate historical default discovery even in tests that clear PITH_STORAGE.
func TestMain(m *testing.M) {
	base, err := os.MkdirTemp("", "pith-config-default-")
	if err != nil {
		panic(err)
	}
	TheBrainBase = base
	code := m.Run()
	os.RemoveAll(base)
	os.Exit(code)
}

func TestLoadConfigSelectedStorage(t *testing.T) {
	for _, kind := range []string{"legacy", "missing", "config-override", "selected-override", "historical-default"} {
		t.Run(kind, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("PITH_STORAGE", "")
			base := TheBrainBase
			TheBrainBase = t.TempDir()
			t.Cleanup(func() { TheBrainBase = base })
			legacy := filepath.Join(home, ".pith")
			selected := legacy
			if kind == "selected-override" {
				selected = t.TempDir()
				t.Setenv("PITH_STORAGE", selected)
			}
			if kind == "historical-default" {
				selected = filepath.Join(TheBrainBase, "TheBrain", "PithBackup")
			}
			want := selected
			data := map[string]any{"enabled_parsers": map[string]bool{"node": false}}
			if kind == "config-override" || kind == "selected-override" {
				want = filepath.Join(home, "alternate")
				data["storage_path"] = want
			}
			if kind != "missing" {
				if err := os.MkdirAll(selected, 0700); err != nil {
					t.Fatal(err)
				}
				encoded, err := json.Marshal(data)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(selected, "config.json"), encoded, 0600); err != nil {
					t.Fatal(err)
				}
			}
			for _, load := range []func() (*Config, error){LoadConfig, LoadConfigWithLegacyFallback} {
				cfg, err := load()
				if err != nil || cfg.StoragePath != want {
					t.Fatalf("storage=%#v err=%v want=%s", cfg, err, want)
				}
			}
			if kind != "historical-default" {
				if _, err := os.Stat(filepath.Join(TheBrainBase, "TheBrain")); !os.IsNotExist(err) {
					t.Fatalf("default relocated: %v", err)
				}
			}
		})
	}
}
