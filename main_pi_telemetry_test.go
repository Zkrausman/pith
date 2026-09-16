package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pith/pkg/pi"
	"pith/pkg/runner"
	"pith/pkg/telemetry"
)

func TestPiTransformTelemetryJSONConsent(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		enabled     bool
	}{
		{"omitted", "", true},
		{"true", `,"telemetryEnabled":true`, true},
		{"false", `,"telemetryEnabled":false`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			storage := filepath.Join(t.TempDir(), "storage")
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("PITH_STORAGE", storage)
			cmd := NewRootCmd()
			cmd.SetArgs([]string{"pi", "transform"})
			cmd.SetIn(strings.NewReader(`{"command":"unknown","output":"token=secret"` + tc.field + `}`))
			var out bytes.Buffer
			cmd.SetOut(&out)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			var got pi.HookResponse
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatal(err)
			}
			want := pi.HookResponse{
				Output: "token=[REDACTED]", Passthrough: true, MinimizationStrategy: "passthrough",
				OriginalLineCount: 1, RetainedLineCount: 1,
				OriginalByteCount: len("token=secret"), RetainedByteCount: len("token=[REDACTED]"),
			}
			if got != want {
				t.Fatalf("output/redaction/provenance: got %#v, want %#v", got, want)
			}
			if _, err := os.Stat(filepath.Join(home, ".pith")); !os.IsNotExist(err) {
				t.Fatalf("unexpected default-home storage: %v", err)
			}
			if !tc.enabled {
				if _, err := os.Stat(storage); !os.IsNotExist(err) {
					t.Fatalf("disabled CLI telemetry created storage: %v", err)
				}
				return
			}
			if _, err := os.Stat(filepath.Join(storage, "pith.db")); err != nil {
				t.Fatalf("enabled CLI did not create database: %v", err)
			}
			tel, err := telemetry.NewTelemetry(storage)
			if err != nil {
				t.Fatal(err)
			}
			defer tel.Close()
			records, err := tel.GetRecentExecutions(10, "")
			if err != nil || len(records) != 1 {
				t.Fatalf("enabled CLI accounting: %v %#v", err, records)
			}
			rec := records[0]
			if rec.Command != "unknown" || rec.Source != pi.HarnessPi || rec.Harness != pi.HarnessPi || !rec.IsPassthrough || rec.OriginalTokens != runner.EstimateTokens("token=secret") || rec.CompressedTokens != runner.EstimateTokens(got.Output) {
				t.Fatalf("incorrect CLI accounting: %#v", rec)
			}
		})
	}
}

func TestPiTransformUnreadableLegacyConfigFailsBeforeTelemetry(t *testing.T) {
	for _, consent := range []string{"false", "true"} {
		t.Run(consent, func(t *testing.T) {
			home := t.TempDir()
			storage := filepath.Join(t.TempDir(), "storage")
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("PITH_STORAGE", storage)
			// A directory is a portable non-missing ReadFile error, unlike chmod
			// permissions which differ on Windows and privileged test runners.
			if err := os.MkdirAll(filepath.Join(home, ".pith", "config.json"), 0700); err != nil {
				t.Fatal(err)
			}
			cmd := NewRootCmd()
			cmd.SetArgs([]string{"pi", "transform"})
			cmd.SetIn(strings.NewReader(`{"command":"node","output":"result","telemetryEnabled":` + consent + `}`))
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(new(bytes.Buffer))
			if err := cmd.Execute(); err == nil {
				t.Fatal("unreadable legacy parser settings must fail closed for either consent mode")
			}
			if _, err := os.Stat(storage); !os.IsNotExist(err) {
				t.Fatalf("config error must precede migration/telemetry: %v", err)
			}
		})
	}
}
