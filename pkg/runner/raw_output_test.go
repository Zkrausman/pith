package runner

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"pith/pkg/config"
	"pith/pkg/parser"
	"pith/pkg/telemetry"
)

func TestRawOutputFixtureProcess(t *testing.T) {
	if os.Getenv("PITH_TEST_RAW_PROCESS") != "1" {
		return
	}
	fmt.Fprint(os.Stdout, os.Getenv("PITH_TEST_RAW_STDOUT"))
	fmt.Fprint(os.Stderr, os.Getenv("PITH_TEST_RAW_STDERR"))
	code, err := strconv.Atoi(os.Getenv("PITH_TEST_RAW_EXIT"))
	if err != nil {
		os.Exit(99)
	}
	os.Exit(code)
}

func TestRunRawPreservesCombinedOutput(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	longOutput := strings.Repeat("ordinary line\n", 600) + "All checks passed\n\n"
	hotOutput := "head1\nhead2\nhidden3\ncontext4\nERROR failure5\ncontext6\ntail7\ntail8"
	for _, tc := range []struct {
		name   string
		stdout string
		stderr string
		code   int
	}{
		{name: "empty"},
		{name: "empty-failure", code: 42},
		{name: "unterminated-utf8", stdout: "雪 café 🙂"},
		{name: "trailing-newlines", stdout: "α\nβ\n\n\n"},
		{name: "long-success", stdout: longOutput},
		{name: "long-success-looking-failure", stdout: longOutput, code: 42},
		{name: "hot-zones", stdout: hotOutput, code: 7},
		{name: "hot-zones-trailing-newline", stdout: hotOutput + "\n"},
		{name: "combined-without-separator", stdout: "{\"artifact\":\"fixture\"}", stderr: "warning: synthetic diagnostic\n\n", code: 42},
		{name: "long-stderr-success", stderr: longOutput},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("PITH_STORAGE", t.TempDir())
			t.Setenv("PITH_TEST_RAW_PROCESS", "1")
			t.Setenv("PITH_TEST_RAW_STDOUT", tc.stdout)
			t.Setenv("PITH_TEST_RAW_STDERR", tc.stderr)
			t.Setenv("PITH_TEST_RAW_EXIT", strconv.Itoa(tc.code))
			tel, err := telemetry.NewTelemetry(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer tel.Close()
			r := NewRunner(&config.Config{MaxLines: 5, HeadLines: 2, TailLines: 2, TokenHeuristic: 4}, tel)
			// A matching parser would visibly change even short or empty output.
			r.parsers = []parser.Parser{decisionFixtureParser{}}
			output, err := os.CreateTemp(t.TempDir(), "stdout")
			if err != nil {
				t.Fatal(err)
			}
			defer output.Close()
			originalStdout := os.Stdout
			os.Stdout = output
			defer func() { os.Stdout = originalStdout }()
			err = r.RunRaw([]string{binary, "-test.run=TestRawOutputFixtureProcess", "--"})
			os.Stdout = originalStdout
			if tc.code == 0 {
				if err != nil {
					t.Fatal(err)
				}
			} else {
				var exitErr *exec.ExitError
				if !errors.As(err, &exitErr) || exitErr.ExitCode() != tc.code {
					t.Fatalf("child error = %v, want exit %d", err, tc.code)
				}
			}
			got, err := os.ReadFile(output.Name())
			if err != nil {
				t.Fatal(err)
			}
			want := tc.stdout + tc.stderr
			if string(got) != want {
				t.Fatalf("raw output = %q, want %q", got, want)
			}
			records, err := tel.GetRecentExecutions(10, "")
			if err != nil || len(records) != 1 {
				t.Fatalf("records: %#v %v", records, err)
			}
			rec := records[0]
			if rec.DecisionReason != telemetry.DecisionProtectedPassthrough || rec.ParserUsed != "none" || !rec.IsPassthrough || rec.OriginalTokens != r.EstimateTokens(want) || rec.CompressedTokens != rec.OriginalTokens {
				t.Fatalf("raw accounting: %#v", rec)
			}
		})
	}

	t.Run("no-command", func(t *testing.T) {
		if err := NewRunner(&config.Config{}, nil).RunRaw(nil); err == nil {
			t.Fatal("expected missing-command error")
		}
	})
}
