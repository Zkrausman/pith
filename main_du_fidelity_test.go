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
)

// Use the shared compiled-entrypoint harness, including its native package gate.
func testCLIDuPathFidelity(t *testing.T, binary, helper string) {
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
	name := "du"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	data, err := os.ReadFile(helper)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(aliases, name), data, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", aliases+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PITH_TEST_CAPTURE_FIXTURE", "1")
	longOutput := strings.Repeat("4K\t./My Project\t雪 \n", 22)
	for _, output := range []string{longOutput, "4K\t./My Project\n", "4K\t leading\t雪 café \t\n", "4K ./ambiguous path\n", "", "du: cannot access 'missing path'\n"} {
		for _, code := range []int{0, 42} {
			for _, raw := range []bool{false, true} {
				t.Run(strconv.Quote(output)+"/"+strconv.Itoa(code)+"/raw="+strconv.FormatBool(raw), func(t *testing.T) {
					t.Setenv("PITH_TEST_RAW_STDOUT", output)
					t.Setenv("PITH_TEST_RAW_STDERR", "")
					t.Setenv("PITH_TEST_RAW_EXIT", strconv.Itoa(code))
					args := []string{"--", "du", "-sh", "fixture"}
					if raw {
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
					want := output
					if output == longOutput && code == 0 && !raw {
						want = strings.TrimSuffix(strings.Repeat("4K\t./My Project\t雪 \n", 20), "\n") + "\n... (+ 2 more)"
					}
					if cmd.ProcessState.ExitCode() != code || stdout.String() != want || (code == 0 && stderr.Len() != 0) {
						t.Fatalf("code=%d stdout=%q stderr=%q; want code=%d output=%q", cmd.ProcessState.ExitCode(), stdout.String(), stderr.String(), code, want)
					}
				})
			}
		}
	}
}
