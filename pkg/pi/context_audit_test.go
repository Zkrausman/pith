package pi

import (
	"strings"
	"testing"

	"pith/pkg/parser"
	"pith/pkg/runner"
)

// These chain fixtures characterize the audit baseline, not desired future
// behavior. Every hook call uses an isolated database: OptimizeHook records
// even when TelemetryEnabled is false.
func TestContextAuditChainDispatchCharacterization(t *testing.T) {
	for _, tc := range []struct {
		name, command, output, wantParser, wantOutput string
	}{
		{"prefix-chain", "cd fixture && git status", "On branch main\n M file.go\n", "chain", "On branch main\n M file.go\n"},
		{"quoted-operator", "unknown 'a|b'", "ordinary output\n", "chain", "ordinary output\n"},
		{"leading-parser-consumes-mixed-output", "git log --oneline; echo second", "abc1234 subject\nsecond\n", "git_log", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := OptimizeHook(HookRequest{Command: tc.command, Output: tc.output, StoragePath: t.TempDir()})
			if got.Parser != tc.wantParser || got.Output != tc.wantOutput || got.Passthrough {
				t.Fatalf("unexpected baseline dispatch: %#v", got)
			}
		})
	}
}

func TestContextAuditGrepExpansionFallback(t *testing.T) {
	input := "a:1:x\nb:2:y\nc:3:z\n"
	got := OptimizeHook(HookRequest{Command: "rg pattern fixture", Output: input, StoragePath: t.TempDir()})
	if got.Parser != "" || !got.Passthrough || got.Output != input {
		t.Fatalf("expanding grep must fall back: %#v", got)
	}
	if runner.EstimateTokens((&parser.GrepParser{}).Parse(input)) <= runner.EstimateTokens(input) {
		t.Fatal("fixture must characterize token expansion")
	}
	// Grouping can also reduce repeated long filenames; blanket disabling
	// would discard this benefit. This is a synthetic case, not a forecast.
	repeated := strings.Repeat("synthetic/long/path/file.go:10:match\n", 20)
	grouped := (&parser.GrepParser{}).Parse(repeated)
	if runner.EstimateTokens(grouped) >= runner.EstimateTokens(repeated) {
		t.Fatal("fixture must characterize beneficial grouping")
	}
}

func TestContextAuditHookPreservationBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, output string
		exitCode     int
		raw          bool
	}{
		{"failure", "a:1:ordinary\n", 1, false},
		{"error-marker", "a:1:ERROR detail\n", 0, false},
		{"warning", "warning: inspect this\n", 0, false},
		{"summary", "Tests 2 passed\n", 0, false},
		{"diff", "diff --git a/x b/x\n@@ -1 +1 @@\n-old\n+new\n", 0, false},
		{"structured", "{\"ordinary\":42}", 0, false},
		{"upstream-truncated", "... output truncated by host ...\n", 0, false},
		{"raw", "a:1:ordinary\n", 0, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := OptimizeHook(HookRequest{Command: "rg pattern fixture", Output: tc.output, ExitCode: tc.exitCode, RawBypass: tc.raw, StoragePath: t.TempDir()})
			if got.Output != tc.output || !got.Passthrough || got.Parser != "" {
				t.Fatalf("preservation boundary changed: %#v", got)
			}
		})
	}
}
