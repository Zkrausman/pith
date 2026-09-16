package pi

import (
	"strings"
	"testing"

	"pith/pkg/parser"
	"pith/pkg/runner"
	"pith/pkg/telemetry"
)

func TestOptimizeHookGrepNonExpansion(t *testing.T) {
	for _, tc := range []struct {
		name, input string
		accepted    bool
	}{
		{"beneficial-grouping", strings.Repeat("synthetic/long/path/file.go:10:match\n", 20), true},
		{"expansion", "a:1:x\nb:2:y\nc:3:z\n", false},
		{"equal-estimate", "abcd\n", false},
		{"unicode-byte-reduction-token-growth", "界界:1:x\n界界:2:y\n", false},
		{"unicode-beneficial-grouping", strings.Repeat("界界界界界界界界:10:match\n", 20), true},
		{"redaction-not-parser-savings", "token=" + strings.Repeat("s", 80), false},
		{"redaction-growth-fallback", "token=x\n", false},
		{"redacted-beneficial-grouping", strings.Repeat("synthetic/long/path/file.go:10:token=synthetic-secret\n", 20), true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			redacted := PiRedact(tc.input)
			parsed := PiRedact((&parser.GrepParser{}).Parse(tc.input))
			if tc.name == "equal-estimate" && (parsed == redacted || runner.EstimateTokens(parsed) != runner.EstimateTokens(redacted)) {
				t.Fatal("fixture must change bytes without changing token estimate")
			}
			if tc.name == "unicode-byte-reduction-token-growth" && (len(parsed) >= len(redacted) || runner.EstimateTokens(parsed) <= runner.EstimateTokens(redacted)) {
				t.Fatal("fixture must distinguish byte length from Unicode token estimate")
			}
			wantOutput, wantParser, strategy := redacted, "", "passthrough"
			if tc.accepted {
				wantOutput, wantParser, strategy = parsed, "grep", "parser:grep"
				if runner.EstimateTokens(parsed) >= runner.EstimateTokens(redacted) || len(parsed) > len(redacted) {
					t.Fatal("accepted fixture must reduce estimates without byte growth")
				}
			}
			got := OptimizeHook(HookRequest{Command: "rg pattern fixture", Output: tc.input, StoragePath: t.TempDir()})
			want := HookResponse{
				Output: wantOutput, Parser: wantParser, Passthrough: !tc.accepted,
				OriginalLineCount: lineCount(tc.input), RetainedLineCount: lineCount(wantOutput),
				OriginalByteCount: len(tc.input), RetainedByteCount: len(wantOutput),
				MinimizationStrategy: strategy,
			}
			if got != want {
				t.Fatalf("got %#v, want %#v", got, want)
			}
		})
	}
}

func TestOptimizeHookGrepDisabled(t *testing.T) {
	input := strings.Repeat("synthetic/long/path/file.go:10:match\n", 20)
	got := OptimizeHook(HookRequest{
		Command: "rg pattern fixture", Output: input, StoragePath: t.TempDir(),
		EnabledParsers: map[string]bool{"grep": false},
	})
	if got.Output != input || got.Parser != "" || !got.Passthrough || got.MinimizationStrategy != "passthrough" {
		t.Fatalf("disabled grep must remain passthrough: %#v", got)
	}
}

func TestOptimizeHookGrepFallbackTelemetry(t *testing.T) {
	dir := t.TempDir()
	input := "a:1:x\nb:2:y\nc:3:z\n"
	OptimizeHook(HookRequest{Command: "rg pattern fixture", Output: input, StoragePath: dir, TelemetryEnabled: true})
	tel, err := telemetry.NewTelemetry(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Close()
	records, err := tel.GetRecentExecutions(1, "")
	if err != nil || len(records) != 1 {
		t.Fatalf("records: %v %#v", err, records)
	}
	got := records[0]
	if !got.IsPassthrough || got.ParserUsed != "" || got.OriginalTokens != runner.EstimateTokens(input) || got.CompressedTokens != got.OriginalTokens {
		t.Fatalf("fallback must retain passthrough accounting: %#v", got)
	}
}
