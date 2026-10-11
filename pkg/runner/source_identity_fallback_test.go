package runner

import (
	"os"
	"testing"

	"pith/pkg/config"
	"pith/pkg/parser"
	"pith/pkg/telemetry"
)

type sourceIdentityParserFixture struct{}

func (sourceIdentityParserFixture) Name() string                   { return "source" }
func (sourceIdentityParserFixture) CanParse(string, []string) bool { return true }
func (sourceIdentityParserFixture) Parse(output string) string {
	return (&parser.SourceParser{}).Parse(output)
}

func TestRunnerSourceIdentityFallbackKeepsPassthroughProvenance(t *testing.T) {
	binary, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	input := "const url = \"https://example.invalid/#fragment\"; // retained\n//go:generate tool\n"
	t.Setenv("PITH_TEST_DECISION_PROCESS", "1")
	t.Setenv("PITH_TEST_DECISION_OUTPUT", input)
	tel, err := telemetry.NewTelemetry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Close()
	r := NewRunner(&config.Config{MaxLines: 100, HeadLines: 1, TailLines: 1, TokenHeuristic: 4}, tel)
	r.parsers = []parser.Parser{sourceIdentityParserFixture{}}
	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	previousStdout := os.Stdout
	os.Stdout = stdout
	defer func() { os.Stdout = previousStdout }()
	if err := r.RunWithOptions([]string{binary, "-test.run=TestDecisionFixtureProcess", "--"}, false); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(stdout.Name())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != input {
		t.Fatalf("source fallback changed output: got %q, want %q", got, input)
	}
	records, err := tel.GetRecentExecutions(10, "")
	if err != nil || len(records) != 1 {
		t.Fatalf("records: %#v %v", records, err)
	}
	rec := records[0]
	if rec.DecisionReason != telemetry.DecisionUnsupportedParser || rec.ParserUsed != "none" || !rec.IsPassthrough ||
		rec.OriginalTokens != r.EstimateTokens(input) || rec.CompressedTokens != rec.OriginalTokens {
		t.Fatalf("identity fallback has transformed provenance: %#v", rec)
	}
}
