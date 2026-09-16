package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestLoadConfigWithLegacyFallback(t *testing.T) {
	for _, tc := range []struct {
		name, selected, legacy string
		selectedDir, legacyDir bool
		wantNode, nodeSet      bool
		wantError              bool
	}{
		{name: "legacy", legacy: `{"enabled_parsers":{"node":false}}`, nodeSet: true},
		{name: "selected-precedence", selected: `{"enabled_parsers":{"node":true}}`, legacy: `{"enabled_parsers":{"node":false}}`, nodeSet: true, wantNode: true},
		{name: "malformed-selected-precedence", selected: `{invalid`, legacy: `{"enabled_parsers":{"node":false}}`},
		{name: "missing-both"},
		{name: "malformed-legacy", legacy: `{invalid`},
		{name: "selected-read-error", selectedDir: true, legacy: `{"enabled_parsers":{"node":false}}`, wantError: true},
		{name: "legacy-read-error", legacyDir: true, wantError: true},
		{name: "selected-skips-legacy-error", selected: `{"enabled_parsers":{"node":false}}`, legacyDir: true, nodeSet: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			storage := filepath.Join(t.TempDir(), "storage")
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("PITH_STORAGE", storage)
			selected := filepath.Join(storage, "config.json")
			legacy := filepath.Join(home, ".pith", "config.json")
			for _, fixture := range []struct {
				path, data string
				directory  bool
			}{{selected, tc.selected, tc.selectedDir}, {legacy, tc.legacy, tc.legacyDir}} {
				if fixture.directory {
					if err := os.MkdirAll(fixture.path, 0700); err != nil {
						t.Fatal(err)
					}
				} else if fixture.data != "" {
					if err := os.MkdirAll(filepath.Dir(fixture.path), 0700); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(fixture.path, []byte(fixture.data), 0600); err != nil {
						t.Fatal(err)
					}
				}
			}
			cfg, err := LoadConfigWithLegacyFallback()
			if tc.wantError {
				if err == nil || cfg != nil {
					t.Fatalf("read error must fail closed: %#v, %v", cfg, err)
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				node, set := cfg.EnabledParsers["node"]
				if node != tc.wantNode || set != tc.nodeSet || cfg.StoragePath != storage || cfg.MaxLines != 500 || cfg.HeadLines != 100 || cfg.TailLines != 100 {
					t.Fatalf("unexpected resolved config: %#v", cfg)
				}
			}
			if tc.selected == "" && !tc.selectedDir {
				if _, err := os.Stat(storage); !os.IsNotExist(err) {
					t.Fatalf("read-only fallback created selected storage: %v", err)
				}
			}
			if tc.name == "legacy" {
				ordinary, err := LoadConfig()
				if err != nil || len(ordinary.EnabledParsers) != 0 {
					t.Fatalf("ordinary loader behavior changed: %#v %v", ordinary, err)
				}
			}
		})
	}
}

func TestLoadConfigWithLegacyFallbackMatchesMigratedConfig(t *testing.T) {
	home, storage := t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PITH_STORAGE", storage)
	legacy := filepath.Join(home, ".pith")
	if err := os.MkdirAll(legacy, 0700); err != nil {
		t.Fatal(err)
	}
	// An explicit storage_path keeps its normal precedence, even when read
	// from legacy config; migration's selected destination remains separate.
	data := []byte(`{"enabled_parsers":{"node":false},"storage_path":"fixture-override","max_lines":123}`)
	if err := os.WriteFile(filepath.Join(legacy, "config.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	before, err := LoadConfigWithLegacyFallback()
	if err != nil {
		t.Fatal(err)
	}
	if before.StoragePath != "fixture-override" || before.MaxLines != 123 || before.USDPerMillionTokens != 3 || before.TokenHeuristic != 4 {
		t.Fatalf("legacy values/defaults not preserved: %#v", before)
	}
	// Copy only synthetic config to compare against the existing loader. No DB.
	if err := os.WriteFile(filepath.Join(storage, "config.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	after, err := LoadConfig()
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatalf("fallback differs from migrated config: before=%#v after=%#v err=%v", before, after, err)
	}
}
