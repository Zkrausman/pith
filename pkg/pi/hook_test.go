package pi

import (
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
