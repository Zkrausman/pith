package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"pith/pkg/config"
	"pith/pkg/runner"
	"pith/pkg/telemetry"
)

func TestLegacyHookDecisionReasons(t *testing.T) {
	for _, tc := range []struct {
		name, command, parser              string
		truncate, truncationNoOp, disabled bool
		reason                             telemetry.DecisionReason
	}{
		{name: "protected", command: "node; echo second", reason: telemetry.DecisionProtectedPassthrough},
		{name: "unsupported", command: "unknown", reason: telemetry.DecisionUnsupportedParser},
		{name: "disabled", command: "node", disabled: true, reason: telemetry.DecisionUnsupportedParser},
		{name: "accepted-no-op", command: "node", parser: "node", reason: telemetry.DecisionTransformed},
		{name: "truncate-protected", command: "node; echo second", truncate: true, reason: telemetry.DecisionTransformed},
		{name: "truncate-unsupported", command: "unknown", truncate: true, reason: telemetry.DecisionTransformed},
		{name: "truncate-parser", command: "node", parser: "node", truncate: true, reason: telemetry.DecisionTransformed},
		{name: "truncation-no-op", command: "unknown", truncationNoOp: true, reason: telemetry.DecisionUnsupportedParser},
		{name: "empty-command-unrecorded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, storage := t.TempDir(), t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("PITH_STORAGE", storage)
			cfg := &config.Config{StoragePath: storage, LastUpdateCheck: time.Now().Unix(), MaxLines: 100, HeadLines: 1, TailLines: 1, TokenHeuristic: 4}
			if tc.truncate || tc.truncationNoOp {
				cfg.MaxLines = 2
			}
			if tc.truncationNoOp {
				cfg.HeadLines, cfg.TailLines = 3, 3
			}
			if tc.disabled {
				cfg.EnabledParsers = map[string]bool{"node": false}
			}
			if err := cfg.Save(); err != nil {
				t.Fatal(err)
			}
			input := HookInput{ToolInput: map[string]interface{}{"command": tc.command}}
			original := "one\ntwo\nthree\nfour\nfive"
			input.ToolResponse.LlmContent = "Output: " + original
			data, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			stdin, err := os.CreateTemp(t.TempDir(), "stdin")
			if err != nil {
				t.Fatal(err)
			}
			defer stdin.Close()
			if _, err := stdin.Write(data); err != nil {
				t.Fatal(err)
			}
			if _, err := stdin.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			old := os.Stdin
			os.Stdin = stdin
			defer func() { os.Stdin = old }()
			cmd := NewRootCmd()
			cmd.SetArgs([]string{"_hook"})
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			var response HookOutput
			if err := json.Unmarshal(out.Bytes(), &response); err != nil {
				t.Fatalf("%s: %v", &out, err)
			}
			wantOutput := original
			if tc.truncate {
				wantOutput = "one\n\n... [3 lines removed by Pith middle-out truncation] ...\n\nfive"
			}
			if tc.parser != "" || tc.truncate {
				if response.Decision != "deny" || response.Reason != "Output: "+wantOutput {
					t.Fatalf("response changed: %#v", response)
				}
			} else if response.Decision != "allow" || response.Reason != "" {
				t.Fatalf("passthrough changed: %#v", response)
			}
			if strings.Contains(out.String(), "decision_reason") {
				t.Fatal("reason changed response schema")
			}
			tel, err := telemetry.NewTelemetry(storage)
			if err != nil {
				t.Fatal(err)
			}
			defer tel.Close()
			records, err := tel.GetRecentExecutions(10, "")
			if err != nil {
				t.Fatal(err)
			}
			if tc.command == "" {
				if len(records) != 0 {
					t.Fatalf("previously unrecorded request was recorded: %#v", records)
				}
				return
			}
			if len(records) != 1 {
				t.Fatalf("records: %#v", records)
			}
			rec := records[0]
			wantParser := tc.parser
			if wantParser == "" {
				wantParser = "none"
			}
			if rec.DecisionReason != tc.reason || rec.ParserUsed != wantParser || rec.IsPassthrough != (tc.parser == "") || rec.OriginalTokens != runner.EstimateTokens(original) || rec.CompressedTokens != runner.EstimateTokens(wantOutput) {
				t.Fatalf("accounting: %#v", rec)
			}
		})
	}
}

func TestPiCLIOptOutDoesNotMigrateDecisionSchema(t *testing.T) {
	for _, explicitPath := range []bool{false, true} {
		t.Run(map[bool]string{false: "configured-storage", true: "request-storage"}[explicitPath], func(t *testing.T) {
			home, storage := t.TempDir(), t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("PITH_STORAGE", storage)
			path := filepath.Join(storage, "pith.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			_, err = db.Exec(`CREATE TABLE executions (id INTEGER PRIMARY KEY, command TEXT, original_content TEXT, compressed_content TEXT);
    INSERT INTO executions VALUES (1, 'synthetic', 'legacy input', 'legacy output');`)
			if err != nil {
				db.Close()
				t.Fatal(err)
			}
			db.Close()
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			fields := map[string]any{"command": "node", "output": "ordinary output", "telemetryEnabled": false}
			if explicitPath {
				fields["storagePath"] = storage
			}
			data, err := json.Marshal(fields)
			if err != nil {
				t.Fatal(err)
			}
			cmd := NewRootCmd()
			cmd.SetArgs([]string{"pi", "transform"})
			cmd.SetIn(bytes.NewReader(data))
			cmd.SetOut(new(bytes.Buffer))
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(before, after) {
				t.Fatal("opt-out migrated or modified database")
			}
			// Do not verify via a migrating telemetry constructor.
			db, err = sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			var columns int
			if err := db.QueryRow("SELECT COUNT(*) FROM pragma_table_info('executions') WHERE name='decision_reason'").Scan(&columns); err != nil {
				t.Fatal(err)
			}
			if columns != 0 {
				t.Fatal("opt-out added reason column")
			}
		})
	}
}
