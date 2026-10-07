package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestExitCode(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want int
	}{
		{name: "success", want: 0},
		{name: "ordinary-error", err: errors.New("configuration failed"), want: 1},
		{name: "wrapped-ordinary-error", err: fmt.Errorf("command: %w", errors.New("invalid argument")), want: 1},
		{name: "missing-process-state", err: &exec.ExitError{}, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := exitCode(tc.err); got != tc.want {
				t.Fatalf("exitCode(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

// TestCLIExitStatus invokes the compiled shipped entrypoint, not just Cobra or
// the runner, so a regression to os.Exit(1) in main cannot pass unnoticed.
// PITH_TEST_BINARY explicitly selects the actual native dist binary in CI. A
// missing/invalid requested binary fails; there is no silent source-build fallback.
func TestCLIExitStatus(t *testing.T) {
	// Keep build caches while isolating configuration, telemetry, and defaults.
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
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PITH_STORAGE", t.TempDir())
	defaultBase := filepath.Join(t.TempDir(), "synthetic default with spaces")
	binary := filepath.Join(t.TempDir(), "pith.exe")
	helper := filepath.Join(t.TempDir(), "exit-status-helper.exe")
	if prebuilt, requested := os.LookupEnv("PITH_TEST_BINARY"); requested {
		if prebuilt == "" {
			t.Fatal("PITH_TEST_BINARY was requested but is empty")
		}
		binary, err = filepath.Abs(prebuilt)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Lstat(binary)
		if err != nil || !info.Mode().IsRegular() {
			t.Fatalf("requested package binary must be a regular file: %s (%v)", binary, err)
		}
		data, err := os.ReadFile(binary)
		if err != nil || len(data) == 0 {
			t.Fatalf("read requested package binary: %v", err)
		}
		t.Logf("Testing packaged binary %s SHA-256 %x", binary, sha256.Sum256(data))
	} else if out, err := exec.Command("go", "build", "-ldflags", "-X 'pith/pkg/config.TheBrainBase="+defaultBase+"'", "-o", binary, "main.go").CombinedOutput(); err != nil {
		t.Fatalf("build entrypoint: %v\n%s", err, out)
	}
	if out, err := exec.Command("go", "build", "-o", helper, "./testdata/exit-status-helper").CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v\n%s", err, out)
	}

	t.Run("parser-fallback-evidence", func(t *testing.T) {
		testCLIParserFallbacks(t, binary, helper)
	})

	prepareStorage := func(t *testing.T) string {
		t.Helper()
		storage := t.TempDir()
		t.Setenv("PITH_STORAGE", storage)
		// Avoid the normal route's asynchronous update request without changing
		// production startup behavior or relying on network availability.
		data, err := json.Marshal(map[string]any{"last_update_check": time.Now().Unix()})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(storage, "config.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		return storage
	}
	runCLI := func(t *testing.T, wantCode int, args ...string) (string, string) {
		t.Helper()
		cmd := exec.Command(binary, args...)
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		err := cmd.Run()
		if err != nil {
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("start CLI: %v", err)
			}
		}
		// Observe the process independently of the production exitCode helper.
		if got := cmd.ProcessState.ExitCode(); got != wantCode {
			t.Fatalf("CLI status = %d, want %d; stdout=%q stderr=%q", got, wantCode, stdout.String(), stderr.String())
		}
		return stdout.String(), stderr.String()
	}
	codes := []int{0, 1, 2, 7, 42, 127}
	if runtime.GOOS == "windows" {
		// Windows exposes wider exit codes; do not truncate them to Unix's byte.
		codes = append(codes, 301)
	}
	for _, route := range []string{"normal", "raw"} {
		for _, code := range codes {
			for _, mode := range []string{"silent", "success-text", "stderr-only", "streams"} {
				t.Run(fmt.Sprintf("%s/%d/%s", route, code, mode), func(t *testing.T) {
					prepareStorage(t)
					args := []string{"--", helper, strconv.Itoa(code), mode}
					if route == "raw" {
						args = append([]string{"raw"}, args...)
					}
					stdout, stderr := runCLI(t, code, args...)
					wantOutput := ""
					if mode == "success-text" || mode == "streams" {
						wantOutput += "All checks passed\n"
					}
					if mode == "stderr-only" || mode == "streams" {
						wantOutput += "diagnostic marker\n"
					}
					// Preserve current runner behavior: child stderr joins stdout.
					if stdout != wantOutput {
						t.Fatalf("stdout = %q, want %q", stdout, wantOutput)
					}
					if code == 0 && stderr != "" {
						t.Fatalf("unexpected CLI diagnostic: %q", stderr)
					}
					if code != 0 && (!strings.Contains(stderr, fmt.Sprintf("Error: exit status %d", code)) || !strings.Contains(stderr, "Usage:")) {
						t.Fatalf("missing existing Cobra diagnostics: %q", stderr)
					}
				})
			}
		}
	}
	t.Run("raw-output", func(t *testing.T) {
		longOutput := strings.Repeat("ordinary line\n", 600) + "All checks passed\n\n"
		for _, tc := range []struct {
			name   string
			stdout string
			stderr string
		}{
			{name: "empty"},
			{name: "unterminated-utf8", stdout: "雪 café 🙂"},
			{name: "trailing-newlines", stdout: "α\nβ\n\n\n"},
			{name: "above-default-limit", stdout: longOutput},
			{name: "above-default-limit-stderr", stderr: longOutput},
			{name: "above-default-limit-combined", stdout: longOutput, stderr: "雪 café 🙂\n" + longOutput},
			{name: "combined-without-separator", stdout: "{\"artifact\":\"fixture\"}", stderr: "warning: synthetic diagnostic\n\n"},
		} {
			for _, code := range []int{0, 42} {
				t.Run(fmt.Sprintf("%s/%d", tc.name, code), func(t *testing.T) {
					prepareStorage(t)
					t.Setenv("PITH_TEST_RAW_STDOUT", tc.stdout)
					t.Setenv("PITH_TEST_RAW_STDERR", tc.stderr)
					stdout, stderr := runCLI(t, code, "raw", "--", helper, strconv.Itoa(code), "raw-output")
					if want := tc.stdout + tc.stderr; stdout != want {
						t.Fatalf("raw output = %q, want %q", stdout, want)
					}
					if code == 0 && stderr != "" {
						t.Fatalf("unexpected CLI diagnostic: %q", stderr)
					}
					if code != 0 && (!strings.Contains(stderr, fmt.Sprintf("Error: exit status %d", code)) || !strings.Contains(stderr, "Usage:")) {
						t.Fatalf("missing existing Cobra diagnostics: %q", stderr)
					}
				})
			}
		}
	})
	t.Run("normal-failure-output", func(t *testing.T) {
		prepareStorage(t)
		input := strings.Repeat("ordinary progress\n", 600) + "All checks passed\n"
		diagnostic := "\n" + strings.Repeat("expected/received context 雪\n", 300) + "checkout.test.ts:42:7\nTests: 1 failed, 24 passed, 25 total\n\n"
		t.Setenv("PITH_TEST_RAW_STDOUT", input)
		t.Setenv("PITH_TEST_RAW_STDERR", diagnostic)
		stdout, stderr := runCLI(t, 42, "--", helper, "42", "raw-output")
		if stdout != input+diagnostic {
			t.Fatalf("normal failure output changed: got %d bytes, want %d", len(stdout), len(input+diagnostic))
		}
		if !strings.Contains(stderr, "Error: exit status 42") || !strings.Contains(stderr, "Usage:") {
			t.Fatalf("missing existing Cobra diagnostics: %q", stderr)
		}
	})

	t.Run("normal-output-still-truncated", func(t *testing.T) {
		storage := prepareStorage(t)
		data, err := json.Marshal(map[string]any{
			"last_update_check": time.Now().Unix(),
			"max_lines":         5,
			"head_lines":        2,
			"tail_lines":        2,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(storage, "config.json"), data, 0600); err != nil {
			t.Fatal(err)
		}
		for _, tc := range []struct {
			input string
			want  string
		}{
			{
				input: "head1\nhead2\nordinary3\nordinary4\nordinary5\nordinary6\ntail7\ntail8",
				want:  "head1\nhead2\n\n... [4 lines removed by Pith middle-out truncation] ...\n\ntail7\ntail8",
			},
			{
				input: "head1\nhead2\nhidden3\ncontext4\nERROR failure5\ncontext6\ntail7\ntail8",
				want:  "head1\nhead2\n\n... [1 lines of non-critical output removed by Pith] ...\n\ncontext4\nERROR failure5\ncontext6\ntail7\ntail8",
			},
		} {
			for _, suffix := range []string{"", "\n"} {
				input, want := tc.input+suffix, tc.want+suffix
				t.Setenv("PITH_TEST_RAW_STDOUT", input)
				t.Setenv("PITH_TEST_RAW_STDERR", "")
				stdout, stderr := runCLI(t, 0, "--", helper, "0", "raw-output")
				if stdout != want || stderr != "" {
					t.Fatalf("normal output = %q, stderr = %q; want %q", stdout, stderr, want)
				}
				stdout, stderr = runCLI(t, 0, "raw", "--", helper, "0", "raw-output")
				if stdout != input || stderr != "" {
					t.Fatalf("raw output = %q, stderr = %q; want %q", stdout, stderr, input)
				}
			}
		}
	})
	t.Run("wrapped-child-error", func(t *testing.T) {
		err := exec.Command(helper, "42", "silent").Run()
		if got := exitCode(fmt.Errorf("wrapped: %w", err)); got != 42 {
			t.Fatalf("wrapped child status = %d, want 42 (error %v)", got, err)
		}
	})
	t.Run("signal-fallback", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("Windows process termination does not have Unix signal semantics")
		}
		prepareStorage(t)
		err := exec.Command(helper, "0", "kill").Run()
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != -1 {
			t.Fatalf("helper did not terminate by signal: %v", err)
		}
		if got := exitCode(err); got != 1 {
			t.Fatalf("signal fallback = %d, want 1", got)
		}
		runCLI(t, 1, "--", helper, "0", "kill")
		runCLI(t, 1, "raw", "--", helper, "0", "kill")
	})
	for _, tc := range []struct {
		name string
		args []string
		want int
	}{
		{name: "no-args", want: 0},
		{name: "help", args: []string{"--help"}, want: 0},
		{name: "version", args: []string{"version"}, want: 0},
		{name: "unknown-flag", args: []string{"--not-a-pith-flag"}, want: 1},
		{name: "invalid-subcommand-args", args: []string{"pi", "transform", "extra"}, want: 1},
		{name: "missing-executable", args: []string{"--", filepath.Join(t.TempDir(), "missing.exe")}, want: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			prepareStorage(t)
			stdout, stderr := runCLI(t, tc.want, tc.args...)
			if tc.name == "version" {
				// Cobra's existing Printf default writes version to stderr.
				// Validate exact bytes without changing that production behavior.
				if stdout != "" || stderr != "Pith "+version+"\n" {
					t.Fatalf("version stdout=%q stderr=%q, want exact %q", stdout, stderr, "Pith "+version+"\n")
				}
				if tag := os.Getenv("PITH_TEST_RELEASE_TAG"); tag != "" {
					if tag != version {
						t.Fatalf("release tag %q differs from binary/source version %q", tag, version)
					}
					changelog, err := os.ReadFile("CHANGELOG.md")
					if err != nil {
						t.Fatal(err)
					}
					if !strings.Contains(string(changelog), "\n## ["+strings.TrimPrefix(tag, "v")+"] - ") {
						t.Fatalf("missing matching changelog heading for %s", tag)
					}
				}
			}
			if tc.want != 0 && !strings.Contains(stderr, "Error:") {
				t.Fatalf("missing CLI error diagnostic: %q", stderr)
			}
		})
	}
	t.Run("config-read-error", func(t *testing.T) {
		storage := t.TempDir()
		t.Setenv("PITH_STORAGE", storage)
		if err := os.Mkdir(filepath.Join(storage, "config.json"), 0700); err != nil {
			t.Fatal(err)
		}
		runCLI(t, 1, "--", helper, "0", "silent")
	})
	t.Run("telemetry-start-error", func(t *testing.T) {
		storage := prepareStorage(t)
		if err := os.Mkdir(filepath.Join(storage, "pith.db"), 0700); err != nil {
			t.Fatal(err)
		}
		runCLI(t, 1, "--", helper, "0", "silent")
	})
}
