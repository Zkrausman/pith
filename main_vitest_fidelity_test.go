package main

import (
	"bytes"
	"crypto/sha256"
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

type cliGoBuildCache struct {
	GOPATH     string `json:"GOPATH"`
	GOMODCACHE string `json:"GOMODCACHE"`
	GOCACHE    string `json:"GOCACHE"`
	GOENV      string `json:"GOENV"`
}

func effectiveCLIGoBuildCache(t *testing.T) cliGoBuildCache {
	t.Helper()
	cache := readCLIGoBuildCache(t, exec.Command("go", "env", "-json", "GOPATH", "GOMODCACHE", "GOCACHE", "GOENV"))
	if goenv, ok := os.LookupEnv("GOENV"); ok && goenv != "" {
		cache.GOENV = goenv
	}
	return cache
}

func readCLIGoBuildCache(t *testing.T, cmd *exec.Cmd) cliGoBuildCache {
	t.Helper()
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("resolve Go build cache before isolating CLI home: %v", err)
	}
	var cache cliGoBuildCache
	if err := json.Unmarshal(out, &cache); err != nil {
		t.Fatalf("decode Go build cache settings %q: %v", out, err)
	}
	if cache.GOPATH == "" || !filepath.IsAbs(cache.GOMODCACHE) || !filepath.IsAbs(cache.GOCACHE) {
		t.Fatalf("Go build cache paths must resolve before isolating CLI home: GOPATH=%q GOMODCACHE=%q GOCACHE=%q", cache.GOPATH, cache.GOMODCACHE, cache.GOCACHE)
	}
	return cache
}

func cliGoCommand(cache cliGoBuildCache, args ...string) *exec.Cmd {
	cmd := exec.Command("go", args...)
	var env []string
	for _, entry := range os.Environ() {
		name, _, ok := strings.Cut(entry, "=")
		if ok && (strings.EqualFold(name, "GOPATH") || strings.EqualFold(name, "GOMODCACHE") || strings.EqualFold(name, "GOCACHE") || strings.EqualFold(name, "GOENV")) {
			continue
		}
		env = append(env, entry)
	}
	cmd.Env = append(env,
		"GOPATH="+cache.GOPATH,
		"GOMODCACHE="+cache.GOMODCACHE,
		"GOCACHE="+cache.GOCACHE,
		"GOENV="+cache.GOENV,
	)
	return cmd
}

// Capture Go's resolved build locations before isolating the app home. Go
// normally derives its default GOPATH (and thus GOMODCACHE) from HOME, so child
// go builds must receive the captured settings after HOME changes.
func isolateCLIHome(t *testing.T, home string) cliGoBuildCache {
	t.Helper()
	cache := effectiveCLIGoBuildCache(t)
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return cache
}

func cliPathWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !filepath.IsAbs(rel) && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func TestCLIIsolatedHomeKeepsGoModuleCache(t *testing.T) {
	// Exercise the runner-default case that exposed the hosted failure: neither
	// GOPATH nor GOMODCACHE is explicitly provided by the environment.
	t.Setenv("GOENV", "off")
	t.Setenv("GOPATH", "")
	t.Setenv("GOMODCACHE", "")
	want := effectiveCLIGoBuildCache(t)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	got := readCLIGoBuildCache(t, cliGoCommand(want, "env", "-json", "GOPATH", "GOMODCACHE", "GOCACHE", "GOENV"))
	if got.GOPATH != want.GOPATH || got.GOMODCACHE != want.GOMODCACHE || got.GOCACHE != want.GOCACHE {
		t.Fatalf("isolated CLI home changed Go's resolved build cache: got %+v, want %+v", got, want)
	}
	if cliPathWithin(home, got.GOMODCACHE) {
		t.Fatalf("Go module cache %q is inside isolated CLI home %q", got.GOMODCACHE, home)
	}
	t.Logf("isolated CLI home %q retains Go module cache %q", home, got.GOMODCACHE)
}

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
	goCache := isolateCLIHome(t, home)
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
	} else if out, err := cliGoCommand(goCache, "build", "-o", binary, "main.go").CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, out)
	}

	helper := filepath.Join(t.TempDir(), "vitest-helper")
	if runtime.GOOS == "windows" {
		helper += ".exe"
	}
	if out, err := cliGoCommand(goCache, "build", "-buildvcs=false", "-o", helper, "./testdata/exit-status-helper").CombinedOutput(); err != nil {
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
	valueAndSourceRules := strings.Replace(fixture, `Expected: "expired"`, `Expected: "expired ⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯ source-value"`, 1)
	valueAndSourceRules = strings.Replace(valueAndSourceRules, `Received: "valid"`, "Received: \"valid\"\n  const divider = \"⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯\";", 1)
	ansiCRLFFixture := strings.ReplaceAll(valueAndSourceRules, "\n", "\r\n")
	ansiCRLFFixture = strings.Replace(ansiCRLFFixture, "FAIL checkout.test.ts", "\x1b[31mFAIL checkout.test.ts", 1)
	ansiCRLFFixture = strings.Replace(ansiCRLFFixture, "rejects expired session\r\n", "rejects expired session\x1b[0m\r\n", 1)
	for _, fixtureCase := range []struct {
		name string
		text string
	}{
		{name: "late-failure", text: fixture},
		{name: "ansi-crlf-source-excerpt", text: ansiCRLFFixture},
		{name: "ansi-fail-only", text: "\x1b[31mFAIL checkout.test.ts > rejects expired session\x1b[0m\nexpected diagnostic survives unchanged\n  at checkout.test.ts:42:7\n"},
	} {
		t.Logf("%s fixture SHA-256: %x", fixtureCase.name, sha256.Sum256([]byte(fixtureCase.text)))
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
			t.Run(fixtureCase.name+"/"+tc.name, func(t *testing.T) {
				t.Setenv("PITH_TEST_CAPTURE_FIXTURE", "1")
				t.Setenv("PITH_TEST_RAW_STDOUT", fixtureCase.text)
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
				if got := stdout.String(); got != fixtureCase.text {
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
}
