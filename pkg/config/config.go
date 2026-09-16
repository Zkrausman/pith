package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	EnabledParsers      map[string]bool `json:"enabled_parsers"`
	LastUpdateCheck     int64           `json:"last_update_check"`
	MaxLines            int             `json:"max_lines"`
	HeadLines           int             `json:"head_lines"`
	TailLines           int             `json:"tail_lines"`
	StoragePath         string          `json:"storage_path"`
	USDPerMillionTokens float64         `json:"usd_per_million_tokens"`
	TokenHeuristic      float64         `json:"token_heuristic"`
	SyncServerURL       string          `json:"sync_server_url"`
	// SnagLogging retains redacted diagnostic logs only when explicitly enabled.
	SnagLogging bool `json:"snag_logging"`
	// SnagLogMaxBytes bounds retained diagnostic logs; zero uses 1 MiB.
	SnagLogMaxBytes int64 `json:"snag_log_max_bytes"`
}

var TheBrainBase = "E:\\"

func GetConfigPath() (string, error) {
	// 1. Check environment variable
	if env := os.Getenv("PITH_STORAGE"); env != "" {
		return filepath.Join(env, "config.json"), nil
	}

	// 2. Check if config exists in the new default location
	newDefault := filepath.Join(TheBrainBase, "TheBrain", "PithBackup")
	newConfig := filepath.Join(newDefault, "config.json")
	if _, err := os.Stat(newConfig); err == nil {
		return newConfig, nil
	}

	// 3. Fallback to old default
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".pith", "config.json"), nil
}

func LoadConfig() (*Config, error) {
	return loadConfig(false)
}

// LoadConfigWithLegacyFallback resolves the config migration would supply without
// writing storage. Existing selected configs remain authoritative, and read errors
// fail closed rather than silently enabling parsers disabled by legacy settings.
// Pi transform uses this before deciding whether consent permits migration.
func LoadConfigWithLegacyFallback() (*Config, error) {
	return loadConfig(true)
}

func loadConfig(legacyFallback bool) (*Config, error) {
	path, err := GetConfigPath()
	if err != nil {
		return nil, err
	}

	cfg := &Config{
		EnabledParsers: make(map[string]bool),
		MaxLines:       500,
		HeadLines:      100,
		TailLines:      100,
		StoragePath:    filepath.Dir(path),
	}

	// If we are using the old default, but newDefault is the intended new default,
	// we should set StoragePath to the new default to trigger migration.
	newDefault := filepath.Join(TheBrainBase, "TheBrain", "PithBackup")
	if env := os.Getenv("PITH_STORAGE"); env != "" {
		cfg.StoragePath = env
	} else if _, err := os.Stat(filepath.Join(newDefault, "config.json")); err == nil {
		cfg.StoragePath = newDefault
	} else if strings.Contains(path, ".pith") {
		// If we are currently in .pith and newDefault doesn't exist, we WANT to go to newDefault
		cfg.StoragePath = newDefault
	}

	data, err := os.ReadFile(path)
	if legacyFallback && os.IsNotExist(err) {
		home, homeErr := os.UserHomeDir()
		if homeErr != nil {
			return nil, homeErr
		}
		data, err = os.ReadFile(filepath.Join(home, ".pith", "config.json"))
	}
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, cfg); err != nil {
		return cfg, nil
	}

	// Ensure defaults if missing from JSON
	if cfg.MaxLines == 0 {
		cfg.MaxLines = 500
	}
	if cfg.HeadLines == 0 {
		cfg.HeadLines = 100
	}
	if cfg.TailLines == 0 {
		cfg.TailLines = 100
	}
	if cfg.USDPerMillionTokens == 0 {
		cfg.USDPerMillionTokens = 3.0 // Default to Claude 3.5 Sonnet Rate
	}
	if cfg.TokenHeuristic == 0 {
		cfg.TokenHeuristic = 4.0
	}
	if cfg.SyncServerURL == "" {
		cfg.SyncServerURL = "http://localhost:8080"
	}

	return cfg, nil
}

func (c *Config) Save() error {
	path, err := GetConfigPath()
	if err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}
