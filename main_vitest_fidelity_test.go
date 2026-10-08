package main

import (
	"bytes"
	"encoding/json"
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

func cliVitestLateFailureFixture() string {
	lines := make([]string, 0, 68)
	for i := 1; i <= 60; i++ {
		lines = append(lines, fmt.Sprintf("routine %03d", i))
	}
	lines = append(lines,
		"FAIL checkout.test.ts > rejects expired session",
		"AssertionError: values differ",
		`Expected: "expired"`,
		`Received: "valid"`,
		"",
		"  at checkout.test.ts:42:7",
		"Test Files  1 failed (1)",
		"Tests  1 failed | 24 passed (25)",
	)
	return strings.Join(lines, "\n") + "\n"
}

// Exercise real parser dispatch through the compiled CLI with a deterministic
// synthetic Vitest executable; the test never runs Vitest or a real suite.
func TestCLIVitestDiagnosticFidelity(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	storage := t.TempDir()
	t.Setenv("PITH_STORAGE", storage)
	config, err := json.Marshal(map[string]any{"last_update_check": time.Now().Unix()})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storage, "config.json"), config, 0600); err != nil {
		t.Fatal(err)
	}

	binary := filepath.Join(t.TempDir(), "pith")
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
	} else if out, err := exec.Command("go", "build", "-o", binary, "main.go").CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}

	helper := filepath.Join(t.TempDir(), "vitest-helper")
	if runtime.GOOS == "windows" {
		helper += ".exe"
	}
	if out, err := exec.Command("go", "build", "-buildvcs=false", "-o", helper, "./testdata/exit-status-helper").CombinedOutput(); err != nil {
		t.Fatalf("build synthetic command: %v\n%s", err, out)
	}
	aliases := t.TempDir()
	name := "vitest"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	helperBytes, err := os.ReadFile(helper)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aliases, name), helperBytes, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", aliases+string(os.PathListSeparator)+os.Getenv("PATH"))

	fixture := cliVitestLateFailureFixture()
	for _, tc := range []struct {
		name string
		code int
		raw  bool
	}{
		{name: "success", code: 0},
		{name: "error", code: 42},
		{name: "raw-success", code: 0, raw: true},
		{name: "raw-error", code: 42, raw: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PITH_TEST_CAPTURE_FIXTURE", "1")
			t.Setenv("PITH_TEST_RAW_STDOUT", fixture)
			t.Setenv("PITH_TEST_RAW_STDERR", "")
			t.Setenv("PITH_TEST_RAW_EXIT", strconv.Itoa(tc.code))
			args := []string{"--", "vitest", "run"}
			if tc.raw {
				args = append([]string{"raw"}, args...)
			}
			cmd := exec.Command(binary, args...)
			var stdout, stderr bytes.Buffer
			cmd.Stdout, cmd.Stderr = &stdout, &stderr
			if err := cmd.Run(); err != nil {
				if _, ok := err.(*exec.ExitError); !ok {
					t.Fatal(err)
				}
			}
			if got := cmd.ProcessState.ExitCode(); got != tc.code {
				t.Fatalf("CLI exit code = %d, want %d; stderr=%q", got, tc.code, stderr.String())
			}
			if got := stdout.String(); got != fixture {
				t.Fatalf("CLI changed failure diagnostics; got %q", got)
			}
			if tc.code == 0 && stderr.Len() != 0 {
				t.Fatalf("unexpected CLI diagnostic: %q", stderr.String())
			}
			if tc.code != 0 && !strings.Contains(stderr.String(), "Error: exit status 42") {
				t.Fatalf("missing existing CLI error status: %q", stderr.String())
			}
		})
	}
}
