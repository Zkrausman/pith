package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"pith/pkg/config"
	"pith/pkg/pi"
	"pith/pkg/telemetry"
)

func TestMain(m *testing.M) {
	base, err := os.MkdirTemp("", "pith-main-default-")
	if err != nil {
		panic(err)
	}
	config.TheBrainBase = base
	code := m.Run()
	os.RemoveAll(base)
	os.Exit(code)
}

// Exercise the shipped entrypoint with synthetic active WAL storage, not just Cobra.
func TestMainStorageSafety(t *testing.T) {
	binary := filepath.Join(t.TempDir(), "pith.exe")
	// Preserve the developer's caches, not their home/storage, for the child build.
	cacheJSON, err := exec.Command("go", "env", "-json", "GOPATH", "GOMODCACHE", "GOCACHE").Output()
	if err != nil {
		t.Fatal(err)
	}
	var caches map[string]string
	if err := json.Unmarshal(cacheJSON, &caches); err != nil {
		t.Fatal(err)
	}
	for key, value := range caches {
		t.Setenv(key, value)
	}
	buildHome := t.TempDir()
	t.Setenv("HOME", buildHome)
	t.Setenv("USERPROFILE", buildHome)
	t.Setenv("PITH_STORAGE", t.TempDir())
	defaultBase := filepath.Join(t.TempDir(), "synthetic default with spaces")
	if out, err := exec.Command("go", "build", "-ldflags", "-X 'pith/pkg/config.TheBrainBase="+defaultBase+"'", "-o", binary, "main.go").CombinedOutput(); err != nil {
		t.Fatalf("build entrypoint: %v\n%s", err, out)
	}
	// A local rejecting proxy exercises update startup without contacting releases
	// or downloading/replacing any binary.
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadGateway) }))
	defer proxy.Close()
	for _, tc := range []struct {
		name, field                    string
		args                           []string
		enabled, configOnly, wantError bool
	}{
		{name: "false", field: `,"telemetryEnabled":false`, args: []string{"pi", "transform"}},
		{name: "false-leading-flag", field: `,"telemetryEnabled":false`, args: []string{"--model", "fixture", "pi", "transform"}},
		{name: "true", field: `,"telemetryEnabled":true`, args: []string{"pi", "transform"}, enabled: true},
		{name: "omitted", args: []string{"pi", "transform"}, enabled: true},
		{name: "config-only", args: []string{"pi", "transform"}, enabled: true, configOnly: true},
		{name: "config-only-false", field: `,"telemetryEnabled":false`, args: []string{"pi", "transform"}, configOnly: true},
		{name: "version", args: []string{"version"}},
		{name: "version-flag", args: []string{"--version"}},
		{name: "help", args: []string{"--help"}},
		{name: "startup", args: []string{"gain"}, enabled: true},
		{name: "update", args: []string{"update"}, wantError: true},
		{name: "config-only-version", args: []string{"version"}, configOnly: true},
		{name: "config-only-update", args: []string{"update"}, configOnly: true, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			storage := filepath.Join(t.TempDir(), "selected")
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("PITH_STORAGE", storage)
			t.Setenv("HTTPS_PROXY", proxy.URL)
			t.Setenv("HTTP_PROXY", proxy.URL)
			t.Setenv("NO_PROXY", "")
			if tc.configOnly {
				t.Setenv("PITH_STORAGE", "")
			}
			legacy := filepath.Join(home, ".pith")
			tel, err := telemetry.NewTelemetry(legacy)
			if err != nil {
				t.Fatal(err)
			}
			defer tel.Close()
			if _, err := tel.DB.Exec("PRAGMA journal_mode=WAL; PRAGMA wal_autocheckpoint=0;"); err != nil {
				t.Fatal(err)
			}
			if err := tel.Record(telemetry.ExecutionRecord{Command: "legacy-fixture"}); err != nil {
				t.Fatal(err)
			}
			// A fallback storage override must not redirect an environment selection.
			configStorage := filepath.Join(home, "unselected")
			if tc.configOnly {
				configStorage = storage
			}
			configData, err := json.Marshal(map[string]any{"enabled_parsers": map[string]bool{"node": false}, "storage_path": configStorage})
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(legacy, "config.json"), configData, 0600); err != nil {
				t.Fatal(err)
			}
			for _, name := range []string{"pith.db.bak", "config.json.bak"} {
				if err := os.WriteFile(filepath.Join(legacy, name), []byte("backup-"+name), 0600); err != nil {
					t.Fatal(err)
				}
			}
			before := map[string][]byte{}
			info := map[string]os.FileInfo{}
			entries, err := os.ReadDir(legacy)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				name := entry.Name()
				before[name], err = os.ReadFile(filepath.Join(legacy, name))
				if err != nil {
					t.Fatal(err)
				}
				info[name], err = entry.Info()
				if err != nil {
					t.Fatal(err)
				}
			}
			if _, ok := before["pith.db-wal"]; !ok {
				t.Fatal("fixture must have active WAL")
			}
			input := "fixture\n" + strings.Repeat("node output\n", 140)
			encoded, _ := json.Marshal(input)
			cmd := exec.Command(binary, tc.args...)
			cmd.Stdin = strings.NewReader(`{"command":"node","output":` + string(encoded) + tc.field + `}`)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			err = cmd.Run()
			if (err != nil) != tc.wantError {
				t.Fatalf("entrypoint err=%v stderr=%s", err, stderr.String())
			}
			if strings.Contains(stderr.String(), "Migrating") {
				t.Fatalf("migration notice: %s", stderr.String())
			}
			if strings.Contains(strings.Join(tc.args, " "), "pi transform") {
				var response pi.HookResponse
				if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
					t.Fatal(err)
				}
				want := pi.HookResponse{Output: input, Passthrough: true, MinimizationStrategy: "passthrough", OriginalLineCount: 142, RetainedLineCount: 142, OriginalByteCount: len(input), RetainedByteCount: len(input)}
				if response != want {
					t.Fatalf("consent changed output/provenance: %#v", response)
				}
			}
			afterEntries, err := os.ReadDir(legacy)
			if err != nil || len(afterEntries) != len(entries) {
				t.Fatalf("legacy entries changed: %v", err)
			}
			for name, data := range before {
				after, err := os.ReadFile(filepath.Join(legacy, name))
				if err != nil || !bytes.Equal(data, after) {
					t.Fatalf("legacy %s changed: %v", name, err)
				}
				afterInfo, err := os.Stat(filepath.Join(legacy, name))
				if err != nil || !info[name].ModTime().Equal(afterInfo.ModTime()) {
					t.Fatalf("legacy %s metadata changed: %v", name, err)
				}
			}
			if !tc.enabled {
				if _, err := os.Stat(storage); !os.IsNotExist(err) {
					t.Fatalf("unexpected target created: %v", err)
				}
				return
			}
			if _, err := os.Stat(filepath.Join(storage, "config.json")); !os.IsNotExist(err) {
				t.Fatalf("legacy config imported: %v", err)
			}
			selected, err := telemetry.NewTelemetry(storage)
			if err != nil {
				t.Fatal(err)
			}
			defer selected.Close()
			records, err := selected.GetRecentExecutions(10, "")
			wantCount := 1
			if tc.name == "startup" {
				wantCount = 0
			}
			if err != nil || len(records) != wantCount {
				t.Fatalf("selected accounting imported legacy: %#v %v", records, err)
			}
			if wantCount == 1 && records[0].Command != "node" {
				t.Fatalf("wrong record: %#v", records)
			}
		})
	}
	t.Run("legacy-selected-in-place", func(t *testing.T) {
		home := t.TempDir()
		t.Setenv("HOME", home)
		t.Setenv("USERPROFILE", home)
		t.Setenv("PITH_STORAGE", "")
		legacy := filepath.Join(home, ".pith")
		tel, err := telemetry.NewTelemetry(legacy)
		if err != nil {
			t.Fatal(err)
		}
		if err := tel.Record(telemetry.ExecutionRecord{Command: "legacy-fixture"}); err != nil {
			tel.Close()
			t.Fatal(err)
		}
		if err := tel.Close(); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(binary, "pi", "transform")
		cmd.Stdin = strings.NewReader(`{"command":"unknown","output":"fixture"}`)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("in-place startup: %v %s", err, out)
		}
		tel, err = telemetry.NewTelemetry(legacy)
		if err != nil {
			t.Fatal(err)
		}
		defer tel.Close()
		records, err := tel.GetRecentExecutions(10, "")
		if err != nil || len(records) != 2 {
			t.Fatalf("legacy history not retained in place: %#v %v", records, err)
		}
		if _, err := os.Stat(filepath.Join(defaultBase, "TheBrain")); !os.IsNotExist(err) {
			t.Fatalf("unexpected default migration target: %v", err)
		}
	})

}
