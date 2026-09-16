package pi

import (
	"encoding/json"
	"strings"
	"testing"

	"pith/pkg/telemetry"
)

func TestOptimizeHookUsesPithParser(t *testing.T) {
	out := "On branch main\n\nChanges not staged for commit:\n\tmodified: pkg/pi/hook.go\n"
	got := OptimizeHook(HookRequest{Command: "git status", Output: strings.Repeat(out, 200)})
	if got.Parser != "git_status" || got.Passthrough {
		t.Fatalf("expected git parser, got %#v", got)
	}
}

func TestOptimizeHookPreservesGitInspectionOutput(t *testing.T) {
	entries := strings.Repeat(" M pkg/file.go\n", 21)
	cases := []string{
		"git status --porcelain=v1 -z",
		`git -C "space dir" status --porcelain=v1 -z`,
		"git worktree list --porcelain",
		"git rev-parse HEAD",
	}
	for _, command := range cases {
		got := OptimizeHook(HookRequest{Command: command, Output: entries, StoragePath: t.TempDir()})
		if got.Output != entries || !got.Passthrough || got.Parser != "" {
			t.Fatalf("inspection output changed for %q: %#v", command, got)
		}
	}
}

func TestOptimizeHookPreservesJSONArraysAndScalars(t *testing.T) {
	for _, output := range []string{`[1,"value",true]`, `true`, `42`, `"text"`, `null`} {
		got := OptimizeHook(HookRequest{Command: "tool", Output: output, StoragePath: t.TempDir()})
		if got.Output != output || !got.Passthrough {
			t.Fatalf("valid JSON must remain lossless: %q -> %#v", output, got)
		}
	}
}
func TestOptimizeHookReportsUpstreamTruncationMetadata(t *testing.T) {
	out := "first\n... output truncated by host ...\nlast"
	got := OptimizeHook(HookRequest{Command: "generic", Output: out})
	if !got.UpstreamTruncated || got.MinimizationStrategy != "upstream-host-truncation" || got.Output != out {
		t.Fatalf("upstream result was changed or misattributed: %#v", got)
	}
	if got.OriginalByteCount != got.RetainedByteCount || got.OmittedByteCount != 0 {
		t.Fatalf("upstream counts must report no Pith omission: %#v", got)
	}
}

func TestOptimizeHookRedactionIsNotOmission(t *testing.T) {
	got := OptimizeHook(HookRequest{Command: "unknown", Output: "token=short-secret"})
	if got.Output == "token=short-secret" || got.OmittedByteCount != 0 || got.OmittedLineCount != 0 {
		t.Fatalf("redaction must not be reported as omission: %#v", got)
	}
}

func TestOptimizeHookReportsFinalCountsWithoutClaimingSourceOmission(t *testing.T) {
	out := strings.Repeat("ordinary output\n", 500)
	got := OptimizeHook(HookRequest{Command: "unknown", Output: out})
	if got.OriginalByteCount != len(out) || got.RetainedByteCount != len(got.Output) || got.OmittedByteCount != 0 || got.OmittedLineCount != 0 {
		t.Fatalf("metadata must distinguish unknown source omission: %#v", got)
	}
}

func TestOptimizeHookReportsParserNetReductionSeparately(t *testing.T) {
	out := strings.Repeat("node output\n", 140)
	got := OptimizeHook(HookRequest{Command: "node", Output: out, StoragePath: t.TempDir()})
	if !got.ParserNetReductionKnown || got.ParserNetLineReduction <= 0 || got.ParserNetByteReduction <= 0 {
		t.Fatalf("parser reduction should be known independently: %#v", got)
	}
	if got.OmittedLineCount != 0 || got.OmittedByteCount != 0 || got.OriginalByteCount != len(out) || got.RetainedByteCount != len(got.Output) {
		t.Fatalf("exact source omission claims are invalid for parser prose: %#v", got)
	}
}

func TestOptimizeHookRedactedJSONRemainsValid(t *testing.T) {
	for _, out := range []string{`{"token":"abc"}`, `["token:abc"]`, `"token:abc"`} {
		got := OptimizeHook(HookRequest{Command: "unknown", Output: out, StoragePath: t.TempDir()})
		if !json.Valid([]byte(got.Output)) {
			t.Fatalf("redaction broke JSON %q -> %q", out, got.Output)
		}
		if strings.Contains(got.Output, "abc") {
			t.Fatalf("secret leaked: %q", got.Output)
		}
	}
}

func TestOptimizeHookTelemetryIsAggregateOnly(t *testing.T) {
	dir := t.TempDir()
	OptimizeHook(HookRequest{Command: "git status --token=raw-secret", Output: strings.Repeat("secret-output\\n", 1000), TelemetryEnabled: true, StoragePath: dir})
	tel, err := telemetry.NewTelemetry(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Close()
	records, err := tel.GetRecentExecutions(1, "")
	if err != nil || len(records) != 1 {
		t.Fatalf("records: %v %#v", err, records)
	}
	detail, err := tel.GetExecutionDetails(records[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(detail.Command, "raw-secret") || !strings.Contains(detail.Command, "[REDACTED]") {
		t.Fatalf("Pi telemetry did not redact command: %#v", detail)
	}
	if detail.OriginalContent != "" || detail.CompressedContent != "" {
		t.Fatalf("Pi telemetry retained content: %#v", detail)
	}
}

func TestOptimizeHookRecordsModelAndRate(t *testing.T) {
	dir := t.TempDir()
	rate := 10.0
	OptimizeHook(HookRequest{Command: "git status", Output: strings.Repeat("status\\n", 2000), TelemetryEnabled: true, StoragePath: dir, Model: "anthropic/claude", InputCostPerMillion: &rate})
	tel, err := telemetry.NewTelemetry(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Close()
	records, err := tel.GetRecentExecutions(1, "")
	if err != nil || len(records) != 1 {
		t.Fatalf("records: %v %#v", err, records)
	}
	if records[0].Model != "anthropic/claude" || records[0].InputCostPerMillion == nil || *records[0].InputCostPerMillion != rate {
		t.Fatalf("model pricing missing: %#v", records[0])
	}
}

func TestOptimizeHookRejectsInvalidInputPrice(t *testing.T) {
	dir := t.TempDir()
	rate := -1.0
	OptimizeHook(HookRequest{Command: "git status", Output: strings.Repeat("status\\n", 2000), TelemetryEnabled: true, StoragePath: dir, InputCostPerMillion: &rate})
	tel, err := telemetry.NewTelemetry(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Close()
	records, err := tel.GetRecentExecutions(1, "")
	if err != nil || len(records) != 1 {
		t.Fatalf("records: %v %#v", err, records)
	}
	if records[0].InputCostPerMillion != nil {
		t.Fatalf("invalid rate persisted: %#v", records[0])
	}
}

func TestOptimizeHookHonorsEnabledParsers(t *testing.T) {
	out := strings.Repeat("On branch main\n\nChanges not staged for commit:\n\tmodified: pkg/pi/hook.go\n", 200)
	got := OptimizeHook(HookRequest{
		Command:        "git status",
		Output:         out,
		EnabledParsers: map[string]bool{"git_status": false},
	})
	if !got.Passthrough || got.Parser != "" || got.Output != out {
		t.Fatalf("disabled parser must preserve output, got %#v", got)
	}
}

func TestOptimizeHookPreservesFailuresAndRaw(t *testing.T) {
	failure := "token=secret\nERROR: boom\n" + strings.Repeat("x\n", 5000)
	got := OptimizeHook(HookRequest{Command: "some_unknown_command", Output: failure})
	if !got.Passthrough || !strings.Contains(got.Output, "[REDACTED]") {
		t.Fatalf("failure must be redacted passthrough: %#v", got)
	}
	raw := OptimizeHook(HookRequest{Command: "git status", Output: strings.Repeat("x\n", 5000), RawBypass: true})
	if !raw.Passthrough {
		t.Fatal("raw bypass must pass through")
	}
}
