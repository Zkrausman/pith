package runner

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"pith/pkg/config"
	"pith/pkg/parser"
	"pith/pkg/telemetry"
)

// A subprocess fixture avoids invoking installed tools or inspecting real output.
func TestDecisionFixtureProcess(t *testing.T) {
	if os.Getenv("PITH_TEST_DECISION_PROCESS") != "1" {
		return
	}
	fmt.Print(os.Getenv("PITH_TEST_DECISION_OUTPUT"))
	os.Exit(0)
}

type decisionFixtureParser struct{ noOp bool }

func (decisionFixtureParser) Name() string                   { return "fixture" }
func (decisionFixtureParser) CanParse(string, []string) bool { return true }
func (p decisionFixtureParser) Parse(output string) string {
	if p.noOp {
		return output
	}
	return "parsed:" + output
}

func TestRunnerDecisionReasons(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                                                                 string
		skip, protected, supported, disabled, truncate, noOp, truncationNoOp bool
		want                                                                 telemetry.DecisionReason
	}{
		{name: "skip", skip: true, supported: true, want: telemetry.DecisionProtectedPassthrough},
		{name: "quoted-protection", protected: true, supported: true, want: telemetry.DecisionProtectedPassthrough},
		{name: "unsupported", want: telemetry.DecisionUnsupportedParser},
		{name: "disabled", supported: true, disabled: true, want: telemetry.DecisionUnsupportedParser},
		{name: "accepted", supported: true, want: telemetry.DecisionTransformed},
		{name: "accepted-no-op", supported: true, noOp: true, want: telemetry.DecisionTransformed},
		{name: "truncate-unsupported", truncate: true, want: telemetry.DecisionTransformed},
		{name: "truncate-protected", protected: true, truncate: true, want: telemetry.DecisionTransformed},
		{name: "truncate-skip", skip: true, truncate: true, want: telemetry.DecisionTransformed},
		{name: "truncate-parser", supported: true, truncate: true, want: telemetry.DecisionTransformed},
		{name: "truncation-no-op", truncationNoOp: true, want: telemetry.DecisionUnsupportedParser},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			input := "one\ntwo\nthree\nfour\nfive"
			t.Setenv("PITH_TEST_DECISION_PROCESS", "1")
			t.Setenv("PITH_TEST_DECISION_OUTPUT", input)
			tel, err := telemetry.NewTelemetry(t.TempDir())
			if err != nil {
				t.Fatal(err)
			}
			defer tel.Close()
			cfg := &config.Config{MaxLines: 100, HeadLines: 1, TailLines: 1, TokenHeuristic: 4}
			if tc.truncate || tc.truncationNoOp {
				cfg.MaxLines = 2
			}
			if tc.truncationNoOp {
				cfg.HeadLines, cfg.TailLines = 3, 3
			}
			if tc.disabled {
				cfg.EnabledParsers = map[string]bool{"fixture": false}
			}
			r := NewRunner(cfg, tel)
			r.parsers = nil
			if tc.supported {
				r.parsers = []parser.Parser{decisionFixtureParser{noOp: tc.noOp}}
			}
			args := []string{binary, "-test.run=TestDecisionFixtureProcess", "--"}
			if tc.protected {
				args = append(args, "'quoted'")
			}
			stdout, err := os.CreateTemp(t.TempDir(), "stdout")
			if err != nil {
				t.Fatal(err)
			}
			defer stdout.Close()
			old := os.Stdout
			os.Stdout = stdout
			defer func() { os.Stdout = old }()
			err = r.RunWithOptions(args, tc.skip)
			os.Stdout = old
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(stdout.Name())
			if err != nil {
				t.Fatal(err)
			}
			want := input
			parsed := tc.supported && !tc.disabled && !tc.skip && !tc.protected
			if parsed && !tc.noOp {
				want = "parsed:" + want
			}
			if tc.truncate {
				want = strings.Split(want, "\n")[0] + "\n\n... [3 lines removed by Pith middle-out truncation] ...\n\nfive"
			}
			if string(got) != want {
				t.Fatalf("output changed: %q != %q", got, want)
			}
			records, err := tel.GetRecentExecutions(10, "")
			if err != nil || len(records) != 1 {
				t.Fatalf("records: %#v %v", records, err)
			}
			rec := records[0]
			wantParser := "none"
			if parsed {
				wantParser = "fixture"
			}
			if rec.DecisionReason != tc.want || rec.ParserUsed != wantParser || rec.IsPassthrough == parsed || rec.OriginalTokens != r.EstimateTokens(input) || rec.CompressedTokens != r.EstimateTokens(want) {
				t.Fatalf("accounting: %#v", rec)
			}
		})
	}
}
