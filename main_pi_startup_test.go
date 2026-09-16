package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"pith/pkg/pi"
	"pith/pkg/telemetry"
)

// Exercise the shipped entrypoint: NewRootCmd alone skips startup migration.
func TestMainPiTransformTelemetryConsentBeforeMigration(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "pith.exe")
	// Build before changing HOME so default Go cache/module paths are preserved.
	if out, err := exec.Command("go", "build", "-o", binary, "main.go").CombinedOutput(); err != nil {
		t.Fatalf("build entrypoint: %v\n%s", err, out)
	}
	input := "token=secret\n" + strings.Repeat("node output\n", 140)
	redacted := "token=[REDACTED]\n" + strings.Repeat("node output\n", 140)
	want := pi.HookResponse{
		Output: redacted, Passthrough: true, MinimizationStrategy: "passthrough",
		OriginalLineCount: 142, RetainedLineCount: 142,
		OriginalByteCount: len(input), RetainedByteCount: len(redacted),
	}
	for _, tc := range []struct {
		name, field string
		args        []string
		migrate     bool
	}{
		{"false", `,"telemetryEnabled":false`, []string{"pi", "transform"}, false},
		{"false-leading-flag", `,"telemetryEnabled":false`, []string{"--model", "fixture", "pi", "transform"}, false},
		{"true", `,"telemetryEnabled":true`, []string{"pi", "transform"}, true},
		{"omitted", "", []string{"pi", "transform"}, true},
		{"other-command", "", []string{"version"}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			storage := filepath.Join(t.TempDir(), "storage")
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("PITH_STORAGE", storage)
			legacy := filepath.Join(home, ".pith")
			tel, err := telemetry.NewTelemetry(legacy)
			if err != nil {
				t.Fatal(err)
			}
			if err := tel.Record(telemetry.ExecutionRecord{Command: "legacy-fixture"}); err != nil {
				tel.Close()
				t.Fatal(err)
			}
			tel.Close()
			configData := []byte(`{"enabled_parsers":{"node":false}}`)
			if err := os.WriteFile(filepath.Join(legacy, "config.json"), configData, 0600); err != nil {
				t.Fatal(err)
			}
			before := make(map[string][]byte)
			info := make(map[string]os.FileInfo)
			for _, name := range []string{"pith.db", "config.json"} {
				before[name], err = os.ReadFile(filepath.Join(legacy, name))
				if err != nil {
					t.Fatal(err)
				}
				info[name], err = os.Stat(filepath.Join(legacy, name))
				if err != nil {
					t.Fatal(err)
				}
			}
			cmd := exec.Command(binary, tc.args...)
			encodedOutput, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			cmd.Stdin = bytes.NewBufferString(`{"command":"node","output":` + string(encodedOutput) + tc.field + `}`)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				t.Fatalf("entrypoint: %v\n%s", err, stderr.String())
			}
			if tc.name != "other-command" {
				var response pi.HookResponse
				if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
					t.Fatalf("invalid response %q: %v", stdout.String(), err)
				}
				if response != want {
					t.Fatalf("consent changed full output or provenance: got %#v, want %#v", response, want)
				}
			}
			if !tc.migrate {
				if _, err := os.Stat(storage); !os.IsNotExist(err) {
					t.Fatalf("disabled startup created migration target: %v", err)
				}
				if stderr.Len() != 0 {
					t.Fatalf("disabled startup emitted migration notices: %s", stderr.String())
				}
				entries, err := os.ReadDir(legacy)
				if err != nil || len(entries) != 2 {
					t.Fatalf("legacy directory changed: %v %v", entries, err)
				}
				for name, content := range before {
					path := filepath.Join(legacy, name)
					after, err := os.ReadFile(path)
					if err != nil || !bytes.Equal(content, after) {
						t.Fatalf("legacy %s changed or renamed: %v", name, err)
					}
					afterInfo, err := os.Stat(path)
					if err != nil || !info[name].ModTime().Equal(afterInfo.ModTime()) {
						t.Fatalf("legacy %s modification time changed: %v", name, err)
					}
				}
				return
			}
			for name, content := range before {
				if _, err := os.Stat(filepath.Join(legacy, name)); !os.IsNotExist(err) {
					t.Fatalf("enabled startup did not rename legacy %s: %v", name, err)
				}
				backup, err := os.ReadFile(filepath.Join(legacy, name+".bak"))
				if err != nil || !bytes.Equal(content, backup) {
					t.Fatalf("legacy backup %s incorrect: %v", name, err)
				}
			}
			migratedConfig, err := os.ReadFile(filepath.Join(storage, "config.json"))
			if err != nil || !bytes.Equal(configData, migratedConfig) {
				t.Fatalf("config not migrated: %v", err)
			}
			tel, err = telemetry.NewTelemetry(storage)
			if err != nil {
				t.Fatal(err)
			}
			defer tel.Close()
			records, err := tel.GetRecentExecutions(10, "")
			wantCount := 2
			if tc.name == "other-command" {
				wantCount = 1
			}
			if err != nil || len(records) != wantCount {
				t.Fatalf("legacy records and enabled accounting: %v %#v", err, records)
			}
		})
	}
}
