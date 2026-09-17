package pi

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"pith/pkg/runner"
	"pith/pkg/telemetry"
)

func isolateHookTelemetry(t *testing.T) (home, storage string) {
	t.Helper()
	home = t.TempDir()
	storage = filepath.Join(t.TempDir(), "storage")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PITH_STORAGE", storage)
	return home, storage
}

func TestOptimizeHookTelemetryDisabledDoesNotCreateStorage(t *testing.T) {
	for _, defaultStorage := range []bool{false, true} {
		t.Run(map[bool]string{false: "explicit-path", true: "default-path"}[defaultStorage], func(t *testing.T) {
			home, storage := isolateHookTelemetry(t)
			path := storage
			if defaultStorage {
				path = ""
			}
			// The zero-value Go bool is deliberately disabled, unlike the CLI default.
			OptimizeHook(HookRequest{Command: "unknown", Output: "token=secret", StoragePath: path})
			for _, dir := range []string{storage, filepath.Join(home, ".pith")} {
				if _, err := os.Stat(dir); !os.IsNotExist(err) {
					t.Fatalf("disabled telemetry created %s: %v", dir, err)
				}
			}
		})
	}
}

func TestOptimizeHookTelemetryDisabledDoesNotModifyExistingDatabase(t *testing.T) {
	_, storage := isolateHookTelemetry(t)
	tel, err := telemetry.NewTelemetry(storage)
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a pre-reason database. Opening it would both migrate the schema
	// and scrub these legacy contents, even without Record.
	if _, err := tel.DB.Exec("ALTER TABLE executions DROP COLUMN decision_reason"); err != nil {
		tel.Close()
		t.Fatal(err)
	}
	_, err = tel.DB.Exec(`INSERT INTO executions (command, original_content, compressed_content) VALUES ('fixture', 'synthetic legacy input', 'synthetic legacy output')`)
	if err != nil {
		tel.Close()
		t.Fatal(err)
	}
	tel.Close()
	path := filepath.Join(storage, "pith.db")
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(storage)
	if err != nil {
		t.Fatal(err)
	}
	OptimizeHook(HookRequest{Command: "unknown", Output: "result", StoragePath: storage, TelemetryEnabled: false})
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	afterEntries, err := os.ReadDir(storage)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || !info.ModTime().Equal(afterInfo.ModTime()) || !reflect.DeepEqual(entries, afterEntries) {
		t.Fatal("disabled telemetry modified the existing database or its directory")
	}
}

func TestOptimizeHookTelemetryPreservesResponseAndEnabledAccounting(t *testing.T) {
	for _, tc := range []struct {
		name string
		req  HookRequest
	}{
		{"parser", HookRequest{Command: "node", Output: "token=secret\n" + strings.Repeat("node output\n", 140)}},
		{"redacted-passthrough", HookRequest{Command: "unknown", Output: "token=secret"}},
		{"structured", HookRequest{Command: "tool", Output: `{"token":"secret"}`}},
		{"raw", HookRequest{Command: "node", Output: "token=secret\n" + strings.Repeat("node output\n", 140), RawBypass: true}},
		{"failure", HookRequest{Command: "node", Output: "token=secret\nERROR: failed", ExitCode: 1}},
		{"upstream", HookRequest{Command: "unknown", Output: "token=secret\n... output truncated by host ..."}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, storage := isolateHookTelemetry(t)
			req := tc.req
			req.StoragePath = storage
			disabled := OptimizeHook(req)
			req.TelemetryEnabled = true
			enabled := OptimizeHook(req)
			if disabled != enabled {
				t.Fatalf("consent changed output or provenance: disabled=%#v enabled=%#v", disabled, enabled)
			}
			if strings.Contains(enabled.Output, "secret") || !strings.Contains(enabled.Output, "[REDACTED]") {
				t.Fatalf("mandatory redaction missing: %q", enabled.Output)
			}
			if tc.name == "parser" && (enabled.Parser != "node" || !enabled.ParserNetReductionKnown) {
				t.Fatalf("parser provenance missing: %#v", enabled)
			}
			tel, err := telemetry.NewTelemetry(storage)
			if err != nil {
				t.Fatal(err)
			}
			defer tel.Close()
			records, err := tel.GetRecentExecutions(10, "")
			if err != nil || len(records) != 1 {
				t.Fatalf("enabled accounting: %v %#v", err, records)
			}
			rec := records[0]
			if rec.OriginalTokens != runner.EstimateTokensWithHeuristic(req.Output, 4) || rec.CompressedTokens != runner.EstimateTokensWithHeuristic(enabled.Output, 4) || rec.ParserUsed != enabled.Parser || rec.IsPassthrough != enabled.Passthrough || rec.Source != HarnessPi || rec.Harness != HarnessPi {
				t.Fatalf("incorrect enabled accounting: %#v", rec)
			}
		})
	}
}
