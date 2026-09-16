package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestGetConfigPath_NewDefault(t *testing.T) {
	tmpDir := t.TempDir()

	oldBase := TheBrainBase
	TheBrainBase = tmpDir
	defer func() { TheBrainBase = oldBase }()

	newDefault := filepath.Join(tmpDir, "TheBrain", "PithBackup")
	os.MkdirAll(newDefault, 0755)
	cfgFile := filepath.Join(newDefault, "config.json")
	os.WriteFile(cfgFile, []byte("{}"), 0644)

	t.Setenv("PITH_STORAGE", "")

	path, err := GetConfigPath()
	if err != nil {
		t.Fatalf("GetConfigPath failed: %v", err)
	}
	if path != cfgFile {
		t.Errorf("Expected path %s, got %s", cfgFile, path)
	}

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.StoragePath != newDefault {
		t.Errorf("Expected StoragePath %s, got %s", newDefault, cfg.StoragePath)
	}
}

func TestLoadConfig_WithDefaultsApplied(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)

	// Write config with zeros (should get defaults applied)
	cfgPath := filepath.Join(tmpDir, "config.json")
	os.WriteFile(cfgPath, []byte(`{"max_lines":0,"head_lines":0,"tail_lines":0}`), 0644)

	cfg, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if cfg.MaxLines != 500 {
		t.Errorf("Expected default MaxLines 500, got %d", cfg.MaxLines)
	}
	if cfg.HeadLines != 100 {
		t.Errorf("Expected default HeadLines 100, got %d", cfg.HeadLines)
	}
	if cfg.TailLines != 100 {
		t.Errorf("Expected default TailLines 100, got %d", cfg.TailLines)
	}
}

func TestSave_CreatesFile(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)

	cfg := &Config{
		EnabledParsers: map[string]bool{"git": true},
		MaxLines:       999,
		HeadLines:      77,
		TailLines:      77,
	}
	err := cfg.Save()
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load it back
	loaded, err := LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig after save failed: %v", err)
	}
	if loaded.MaxLines != 999 {
		t.Errorf("Expected MaxLines 999, got %d", loaded.MaxLines)
	}
}
