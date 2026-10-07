package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// Exercise the shipping entrypoint with synthetic capture helpers only: no HTTP
// requests, provider calls, global configuration, or real account data.
func TestCLIStructuredContentFidelity(t *testing.T) {
	binary := os.Getenv("PITH_TEST_BINARY")
	if binary == "" {
		binary = filepath.Join(t.TempDir(), "pith.exe")
		if out, err := exec.Command("go", "build", "-o", binary, "main.go").CombinedOutput(); err != nil {
			t.Fatalf("build: %v\n%s", err, out)
		}
	}
	binary, err := filepath.Abs(binary)
	if err != nil {
		t.Fatal(err)
	}
	aliases := t.TempDir()
	helper := filepath.Join(aliases, "fixture.exe")
	if out, err := exec.Command("go", "build", "-o", helper, "testdata/exit-status-helper/main.go").CombinedOutput(); err != nil {
		t.Fatalf("helper: %v\n%s", err, out)
	}
	data, err := os.ReadFile(helper)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"cat", "curl", "wget"} {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		if err := os.WriteFile(filepath.Join(aliases, name), data, 0700); err != nil {
			t.Fatal(err)
		}
	}
	storage := t.TempDir()
	cfg, _ := json.Marshal(map[string]any{"last_update_check": time.Now().Unix(), "storage_path": storage, "max_lines": 500})
	if err := os.WriteFile(filepath.Join(storage, "config.json"), cfg, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PITH_STORAGE", storage)
	t.Setenv("PATH", aliases+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PITH_TEST_CAPTURE_FIXTURE", "1")
	t.Setenv("PITH_TEST_RAW_STDERR", "")
	t.Setenv("PITH_TEST_RAW_EXIT", "0")
	invoke := func(input []byte, args ...string) (string, int) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Stdin = bytes.NewReader(input)
		var out, diagnostics bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &diagnostics
		err := cmd.Run()
		if err != nil {
			if _, ok := err.(*exec.ExitError); !ok {
				t.Fatal(err)
			}
		}
		if cmd.ProcessState.ExitCode() == 0 && diagnostics.Len() != 0 {
			t.Fatalf("unexpected diagnostics %q", diagnostics.String())
		}
		return out.String(), cmd.ProcessState.ExitCode()
	}
	for _, tc := range []struct{ name, input, want string }{
		{"precision", ` { "id": 9007199254740993, "s": "a  b\u0020\\", "n": -0, "e": 1E+99 } `, `{"id":9007199254740993,"s":"a  b\u0020\\","n":-0,"e":1E+99}`},
		{"large", ` { "s": "` + strings.Repeat("a  b", 300) + `" } `, `{"s":"` + strings.Repeat("a  b", 300) + `"}`},
		{"malformed", " {\"id\": 9007199254740993, } \n", " {\"id\": 9007199254740993, } \n"},
		{"html", "<!-- preserve -->\n<html><title>T</title><pre>a  b</pre></html>\n", "<!-- preserve -->\n<html><title>T</title><pre>a  b</pre></html>\n"},
		{"nul", " {\"s\":\"a\x00b\"} \n", " {\"s\":\"a\x00b\"} \n"},
	} {
		for _, command := range []string{"cat fixture.json", "curl fixture", "wget fixture"} {
			t.Run(tc.name+"/"+command, func(t *testing.T) {
				want := tc.want
				webHTML := tc.name == "html" && !strings.HasPrefix(command, "cat ")
				if webHTML {
					want = "HTML Content: [T] (62 chars total)"
				}

				if !strings.ContainsRune(tc.input, 0) {
					t.Setenv("PITH_TEST_RAW_STDOUT", tc.input)
					got, code := invoke(nil, append([]string{"--"}, strings.Fields(command)...)...)
					if code != 0 || got != want {
						t.Errorf("normal code=%d got %q want %q", code, got, want)
					}
					got, code = invoke(nil, append([]string{"raw", "--"}, strings.Fields(command)...)...)
					if code != 0 || got != tc.input {
						t.Errorf("raw code=%d got %q", code, got)
					}
					t.Setenv("PITH_TEST_RAW_EXIT", "7")
					got, code = invoke(nil, append([]string{"--"}, strings.Fields(command)...)...)
					if code != 7 || got != tc.input {
						t.Errorf("failure code=%d got %q", code, got)
					}
					t.Setenv("PITH_TEST_RAW_EXIT", "0")
				}
				for _, raw := range []bool{false, true} {
					req, _ := json.Marshal(map[string]any{"command": command, "output": tc.input, "rawBypass": raw, "telemetryEnabled": false})
					got, code := invoke(req, "pi", "transform")
					var resp struct {
						Output      string `json:"output"`
						Passthrough bool   `json:"passthrough"`
					}
					if err := json.Unmarshal([]byte(got), &resp); err != nil {
						t.Fatal(err)
					}
					// Pi already protects valid JSON before parser dispatch.
					piWant := tc.input
					if webHTML && !raw {
						piWant = want
					}
					if (raw || json.Valid([]byte(tc.input))) && !resp.Passthrough {
						t.Errorf("Pi protected output lost passthrough metadata")
					}
					if code != 0 || resp.Output != piWant {
						t.Errorf("pi raw=%v code=%d got %q want %q", raw, code, resp.Output, piWant)
					}
				}
				req, _ := json.Marshal(map[string]any{"tool_input": map[string]string{"command": command}, "tool_response": map[string]string{"llmContent": tc.input}})
				got, code := invoke(req, "_hook")
				var resp HookOutput
				if err := json.Unmarshal([]byte(got), &resp); err != nil {
					t.Fatal(err)
				}
				if code != 0 || resp.Decision != "deny" || resp.Reason != want {
					t.Errorf("legacy code=%d response=%#v", code, resp)
				}
			})
		}
	}
}
