package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"pith/pkg/pi"
)

// Called by TestCLIExitStatus, including the native packaged-binary CI smoke.
// Every command resolves to a copied synthetic executable, never a real Git,
// test runner, coverage tool, model, or provider.
func testCLIParserFallbacks(t *testing.T, binary, helper string) {
	storage := t.TempDir()
	t.Setenv("PITH_STORAGE", storage)
	config, err := json.Marshal(map[string]any{"last_update_check": time.Now().Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storage, "config.json"), config, 0600); err != nil {
		t.Fatal(err)
	}
	aliases := t.TempDir()
	helperBytes, err := os.ReadFile(helper)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"git", "go", "npm", "pytest"} {
		if runtime.GOOS == "windows" {
			name += ".exe"
		}
		if err := os.WriteFile(filepath.Join(aliases, name), helperBytes, 0700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", aliases+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PITH_TEST_CAPTURE_FIXTURE", "1")
	t.Setenv("PITH_TEST_RAW_STDERR", "")
	run := func(input []byte, args ...string) (string, string, int) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		cmd.Stdin = bytes.NewReader(input)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			if _, ok := err.(*exec.ExitError); !ok {
				t.Fatal(err)
			}
		}
		return stdout.String(), stderr.String(), cmd.ProcessState.ExitCode()
	}
	for _, tc := range []struct{ name, command, output string }{
		{"empty-status", "git status", ""},
		{"show-short-date", "git show", "commit 0123456789abcdef0123456789abcdef01234567\nAuthor: Example <example@example.invalid>\nDate:   Sun Mar 15\n\n    Subject\n"},
		{"show-incomplete", "git show", "commit abc1234\n\n"},
		{"show-blob", "git show HEAD:fixture", "On branch main\nindex literal content\n@@ retained\n"},
		{"empty-porcelain", "git status --porcelain=v1", ""},
		{"empty-add", "git add fixture", ""},
		{"empty-coverage", "go tool cover -func missing.cov", ""},
		{"empty-tests", "go test ./...", ""},
		{"unknown-tests", "pytest", "collecting 25 cases\nconnection closed\n"},
		{"unknown-git", "git status", "  unrecognized fixture 雪\n\n"},
		{"unknown-coverage", "go tool cover -func missing.cov", "no captured coverage\n"},
		{"partial-json", "npm test --json", "{\"stats\":{\"tests\":25,\"passes\":23,\n"},
		{"json-with-summary", "npm test --json", "[\n{\"summary\": \"TOTAL: 25 passed\"},\n"},
		{"banner-before-json", "npm test --json", "\n> synthetic-fixture test\n> fixture-runner --json\n\n{\n  \"summary\": \"TOTAL: 25 passed\",\n  \"detail\":"},
		{"whitespace", "npm test", " \t\n\r\n"},
	} {
		for _, code := range []int{0, 2} {
			t.Run(tc.name+"/"+strconv.Itoa(code), func(t *testing.T) {
				t.Setenv("PITH_TEST_RAW_STDOUT", tc.output)
				t.Setenv("PITH_TEST_RAW_EXIT", strconv.Itoa(code))
				for _, route := range []string{"normal", "raw"} {
					args := append([]string{"--"}, strings.Fields(tc.command)...)
					if route == "raw" {
						args = append([]string{"raw"}, args...)
					}
					stdout, stderr, gotCode := run(nil, args...)
					if gotCode != code || stdout != tc.output || (code == 0 && stderr != "") {
						t.Fatalf("%s output=%q stderr=%q code=%d; want %q code=%d", route, stdout, stderr, gotCode, tc.output, code)
					}
				}
				request, err := json.Marshal(pi.HookRequest{Command: tc.command, Output: tc.output, ExitCode: code})
				if err != nil {
					t.Fatal(err)
				}
				stdout, stderr, gotCode := run(request, "pi", "transform")
				var got pi.HookResponse
				if err := json.Unmarshal([]byte(stdout), &got); err != nil {
					t.Fatal(err)
				}
				if gotCode != 0 || stderr != "" || got.Output != tc.output || !got.Passthrough || got.Parser != "" {
					t.Fatalf("Pi response=%#v stderr=%q code=%d", got, stderr, gotCode)
				}
				// Legacy hook has no numeric exit field; assert only captured text here.
				legacy := HookInput{ToolInput: map[string]interface{}{"command": tc.command}}
				legacy.ToolResponse.LlmContent = tc.output
				request, err = json.Marshal(legacy)
				if err != nil {
					t.Fatal(err)
				}
				stdout, stderr, gotCode = run(request, "_hook")
				var hook HookOutput
				if err := json.Unmarshal([]byte(stdout), &hook); err != nil {
					t.Fatal(err)
				}
				if gotCode != 0 || stderr != "" || (hook.Decision != "allow" && hook.Reason != tc.output) {
					t.Fatalf("legacy response=%#v stderr=%q code=%d", hook, stderr, gotCode)
				}
			})
		}
	}
	// A supported summary is still compressed through actual runner dispatch.
	t.Setenv("PITH_TEST_RAW_EXIT", "0")
	t.Setenv("PITH_TEST_RAW_STDOUT", "=== RUN TestFixture\n--- PASS: TestFixture\nPASS\nok  fixture/pkg 0.001s\n")
	stdout, stderr, code := run(nil, "--", "go", "test", "./...")
	if code != 0 || stderr != "" || stdout != "PASS\nok  fixture/pkg 0.001s" {
		t.Fatalf("recognized summary output=%q stderr=%q code=%d", stdout, stderr, code)
	}
}
