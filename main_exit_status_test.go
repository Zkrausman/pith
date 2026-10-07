package main

import (
	"bytes"
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
	if out, err := exec.Command("go", "build", "-ldflags", "-X 'pith/pkg/config.TheBrainBase="+defaultBase+"'", "-o", binary, "main.go").CombinedOutput(); err != nil {
		t.Fatalf("build entrypoint: %v\n%s", err, out)
	}
	if out, err := exec.Command("go", "build", "-o", helper, "./testdata/exit-status-helper").CombinedOutput(); err != nil {
		t.Fatalf("build helper: %v\n%s", err, out)
	}

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
			_, stderr := runCLI(t, tc.want, tc.args...)
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
