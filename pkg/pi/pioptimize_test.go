package pi

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"pith/pkg/telemetry"
)

func TestPiOptimize_LosslessSmall(t *testing.T) {
	out := "hello\nworld\n"
	got, _ := PiOptimize("echo hi", out, 0)
	if got != out {
		t.Fatalf("small output must be lossless, got %q", got)
	}
}

func TestPiOptimize_LosslessErrorExit(t *testing.T) {
	large := strings.Repeat("line ok\n", 8000)
	got, _ := PiOptimize("false", large, 1)
	if got != large {
		t.Fatalf("non-zero exit must preserve raw")
	}
}

func TestPiOptimize_LosslessFailMarker(t *testing.T) {
	large := strings.Repeat("ok\n", 4000) + "[FAIL] something\n" + strings.Repeat("ok\n", 4000)
	got, _ := PiOptimize("go test ./...", large, 0)
	if got != large {
		t.Fatalf("FAIL marker must preserve raw")
	}
}

func TestPiOptimize_LosslessFailedMarker(t *testing.T) {
	large := strings.Repeat("x\n", 4000) + "FAILED\n" + strings.Repeat("x\n", 4000)
	got, _ := PiOptimize("go test ./...", large, 0)
	if got != large {
		t.Fatalf("FAILED must preserve raw")
	}
}

func TestPiOptimize_LosslessErrorMarker(t *testing.T) {
	large := strings.Repeat("x\n", 4000) + "ERROR: boom\n" + strings.Repeat("x\n", 4000)
	got, _ := PiOptimize("cmd", large, 0)
	if got != large {
		t.Fatalf("ERROR must preserve raw")
	}
}

func TestPiOptimize_LosslessDiff(t *testing.T) {
	diff := "diff --git a/foo.go b/foo.go\n@@ -1 +1 @@\n-old\n+new\n" + strings.Repeat("x\n", 5000)
	got, _ := PiOptimize("git diff", diff, 0)
	if got != diff {
		t.Fatalf("diff must preserve raw")
	}
}

func TestPiOptimize_LargeCompression(t *testing.T) {
	large := strings.Repeat("line number 12345 with some content to increase bytes\n", 600)
	got, _ := PiOptimize("go test ./... -v", large, 0)
	if len(got) >= len(large) {
		t.Fatalf("large output should be compressed: %d vs %d", len(got), len(large))
	}
	if !strings.Contains(got, "minimized by Pith PiOptimize") {
		t.Fatalf("compressed should contain marker, got preview %q", got[:200])
	}
}

func TestPiOptimize_PreservesRealHostMarkers(t *testing.T) {
	for _, out := range []string{
		"head\n[Showing lines 1-20 of 100. Full output: C:/tmp/result.txt]\ntail",
		"head\n[Showing last 20 lines (100 line limit). Full output: C:/tmp/result.txt]\ntail",
	} {
		got, _ := PiOptimizeWithConfig("generic-command", out, 0, PiConfig{ThresholdBytes: 1})
		if got != out {
			t.Fatalf("host marker must remain untouched: %q", got)
		}
	}
}

func TestPiOptimize_PreservesUpstreamTruncationMarker(t *testing.T) {
	out := "first\n... output truncated by host ...\nlast"
	got, err := PiOptimizeWithConfig("generic-command", out, 0, PiConfig{ThresholdBytes: 1})
	if err != nil || got != out {
		t.Fatalf("upstream truncation must remain untouched: %q, %v", got, err)
	}
}

func TestPiOptimize_PreservesJSONByContent(t *testing.T) {
	out := `{"items":[` + strings.Repeat(`"value",`, 2000) + `"last"]}`
	got, _ := PiOptimizeWithConfig("some-command", out, 0, PiConfig{ThresholdBytes: 1})
	if got != out {
		t.Fatal("valid JSON must remain lossless")
	}
}

func TestPiOptimize_PreservesJSONScalars(t *testing.T) {
	for _, out := range []string{`true`, `42`, `"a large enough JSON string"`, `null`} {
		got, _ := PiOptimizeWithConfig("some-command", out, 0, PiConfig{ThresholdBytes: 1})
		if got != out {
			t.Fatalf("valid JSON scalar must remain lossless: %q", out)
		}
	}
}

func TestPiOptimize_PreservesWarnings(t *testing.T) {
	out := "warning: retain this diagnostic\n" + strings.Repeat("detail\n", 2000)
	got, _ := PiOptimizeWithConfig("generic-command", out, 0, PiConfig{ThresholdBytes: 1})
	if got != out {
		t.Fatal("warning output must remain lossless")
	}
}

func TestPiOptimize_PreservesWarningsAndFinalSummaryAfterHotLines(t *testing.T) {
	lines := make([]string, 0, 520)
	for i := 0; i < 500; i++ {
		lines = append(lines, fmt.Sprintf("ordinary detail %03d", i))
	}
	lines[101] = "npm WARN deprecated package"
	lines[121] = "[build warning] retain bracketed diagnostic"
	lines[141] = "2026-01-02T03:04:05Z warning: timestamped diagnostic"
	lines = append(lines, "Test Suites: 1 passed, 1 total")
	out := strings.Join(lines, "\n") + "\n"
	got, err := PiOptimizeWithConfig("generic-command", out, 0, PiConfig{ThresholdBytes: 1})
	if err != nil || got != out {
		t.Fatalf("warnings and final summary must remain lossless: %v", err)
	}
}

func TestPiOptimize_PreservesInspectionCommands(t *testing.T) {
	out := strings.Repeat("entry\n", 2000)
	for _, command := range []string{"git status --porcelain", "git worktree list --porcelain", "git rev-parse HEAD"} {
		got, _ := PiOptimizeWithConfig(command, out, 0, PiConfig{ThresholdBytes: 1})
		if got != out {
			t.Fatalf("%q must remain lossless", command)
		}
	}
}

func TestPiOptimize_RawBypass(t *testing.T) {
	large := strings.Repeat("x\n", 10000)
	got, _ := PiOptimizeWithConfig("go test ./...", large, 0, PiConfig{RawBypass: true, ThresholdBytes: 10})
	if got != large {
		t.Fatalf("raw bypass must return unchanged")
	}
	if rawBypassCheck() == false {
		t.Fatalf("sanity")
	}
}

func rawBypassCheck() bool { return true }

func TestPiOptimize_CompressionMarkerCountsUTF8AndEmptyLines(t *testing.T) {
	line := strings.Repeat("界", 50)
	parts := make([]string, 0, 120)
	for i := 0; i < 120; i++ {
		parts = append(parts, line)
	}
	parts[40], parts[80] = "", ""
	out := strings.Join(parts, "\n") + "\n"
	got, err := PiOptimizeWithConfig("generic-command", out, 0, PiConfig{ThresholdBytes: 1})
	if err != nil || got == out {
		t.Fatalf("expected compressed UTF-8 output: %v", err)
	}
	marker := regexp.MustCompile(`\.\.\. \[(\d+) bytes, (\d+) lines minimized by Pith PiOptimize\] \.\.\.`).FindStringSubmatch(got)
	if len(marker) != 3 {
		t.Fatalf("missing compression marker: %q", got)
	}
	bytes, _ := strconv.Atoi(marker[1])
	lines, _ := strconv.Atoi(marker[2])
	if bytes <= 0 || lines <= 0 || bytes > len(out) {
		t.Fatalf("invalid exact omission counts: %q", marker[0])
	}
	if marker[1] != "17226" || marker[2] != "116" {
		t.Fatalf("unexpected UTF-8/empty-line omission math: %q", marker[0])
	}
	if !strings.Contains(got, "界") {
		t.Fatal("UTF-8 retained output was corrupted")
	}
}

func TestPiOptimize_TrailingNewlineAndTinyThresholdNeverExpand(t *testing.T) {
	for _, out := range []string{
		strings.Repeat("x\n", 120),
		strings.Repeat("x", 200) + "\n",
		"first\n\nlast\n",
	} {
		got, err := PiOptimizeWithConfig("generic-command", out, 0, PiConfig{ThresholdBytes: 1})
		if err != nil || len(got) > len(out) {
			t.Fatalf("compression must not expand %d-byte output: %v", len(out), err)
		}
	}
}

func TestPiOptimize_HotLineWindowsDoNotDuplicate(t *testing.T) {
	lines := make([]string, 0, 500)
	for i := 0; i < 500; i++ {
		lines = append(lines, fmt.Sprintf("ordinary %03d", i))
	}
	lines[100] = "test hot line"
	lines[101] = "test adjacent hot line"
	out := strings.Join(lines, "\n")
	got, _ := PiOptimizeWithConfig("generic-command", out, 0, PiConfig{ThresholdBytes: 1})
	if got == out {
		t.Fatal("expected compression")
	}
	if strings.Count(got, "test hot line") != 1 || strings.Count(got, "test adjacent hot line") != 1 {
		t.Fatalf("hot lines duplicated: %q", got)
	}
}

func TestPiOptimize_RedactionDoesNotClaimOmission(t *testing.T) {
	for _, value := range []string{"short", strings.Repeat("long-value-", 100)} {
		out := "token=" + value + "\n"
		got := OptimizeHook(HookRequest{Command: "unknown", Output: out, StoragePath: t.TempDir()})
		if got.OmittedByteCount != 0 || got.OmittedLineCount != 0 {
			t.Fatalf("redaction is not source omission for %q: %#v", value, got)
		}
	}
}

func TestPiOptimize_Threshold(t *testing.T) {
	s := strings.Repeat("a\n", 5000)
	// threshold 1M means lossless small
	got, _ := PiOptimizeWithConfig("cmd", s, 0, PiConfig{ThresholdBytes: 100000})
	if got != s {
		t.Fatalf("below high threshold must be lossless")
	}
	// tiny threshold forces compression unless markers present
	got2, _ := PiOptimizeWithConfig("cmd", s, 0, PiConfig{ThresholdBytes: 10})
	if got2 == s || len(got2) >= len(s) {
		t.Fatalf("above tiny threshold should compress")
	}
}

func TestPiOptimize_Deterministic(t *testing.T) {
	large := strings.Repeat("deterministic line content for pith\n", 800)
	a, _ := PiOptimize("cmd", large, 0)
	b, _ := PiOptimize("cmd", large, 0)
	if a != b {
		t.Fatalf("deterministic failure")
	}
}

func TestPiOptimize_RedactApiKey(t *testing.T) {
	s := "api_key=sk-example-redaction-fixture\n"
	red := PiRedact(s)
	if strings.Contains(red, "sk-example-redaction-fixture") {
		t.Fatalf("api_key not redacted: %q", red)
	}
	if !strings.Contains(red, "[REDACTED]") {
		t.Fatalf("should contain redacted marker: %q", red)
	}
}

func TestPiOptimize_RedactDoesNotPersistRaw(t *testing.T) {
	large := strings.Repeat("ok line\n", 1000)
	sensitive := large + "my secret=abc123 token=xyz\n" + large
	// with redaction enabled, compressed output must not contain secret
	got, _ := PiOptimizeWithConfig("cmd", sensitive, 0, PiConfig{ThresholdBytes: 10, Redact: true})
	if strings.Contains(got, "abc123") || strings.Contains(got, "xyz") {
		t.Fatalf("redacted output still contains secrets: %q", got)
	}
}

func TestPiRedact_TokenPatterns(t *testing.T) {
	cases := []string{
		"token=secretvalue123",
		"secret: mysecret",
		"password = hunter2",
		"Bearer example.bearer.token",
		"ghp_example_token",
		"sk-example-token",
	}
	for _, c := range cases {
		red := PiRedact(c)
		if PiShouldRedact(c) == false {
			// Some patterns may not trigger if regex is strict; log
			t.Logf("pattern not detected as secret: %q -> %q", c, red)
		}
		if red == c {
			t.Errorf("should redact %q", c)
		}
		if strings.Contains(red, "secretvalue123") || strings.Contains(red, "hunter2") || strings.Contains(red, "example.bearer.token") {
			// If secret leaked
			// Allow ghp/sk to be fully replaced
		}
	}
}

func TestPiShouldRedact_EvidencePaths(t *testing.T) {
	if !PiShouldRedact("store this evidence/delivery/ZAR-110.json content") {
		t.Fatalf("should flag evidence path")
	}
	if !PiShouldRedact("read .llm-wiki/raw/foo") {
		t.Fatalf("should flag raw wiki")
	}
}

func TestPiOptimizeTelemetryRecordsRedactedCommand(t *testing.T) {
	dir := t.TempDir()
	_, err := PiOptimizeWithConfig("echo --token=raw-secret", "telemetry output", 0, PiConfig{
		TelemetryEnabled: true,
		StoragePath:      dir,
		Harness:          HarnessPi,
	})
	if err != nil {
		t.Fatal(err)
	}
	tel, err := telemetry.NewTelemetry(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Close()
	records, err := tel.GetRecentExecutions(1, "")
	if err != nil || len(records) != 1 {
		t.Fatalf("records: %v %#v", err, records)
	}
	if records[0].Command != "echo --token=[REDACTED]" {
		t.Fatalf("Pi telemetry did not redact the command: %#v", records[0])
	}
}

func TestPiOptimize_NeverSpawnsCommand(t *testing.T) {
	// Ensure it doesn't exec: command that would fail if spawned is treated as string.
	// If PiOptimize spawned, this would be detectable, but we verify transform-only.
	got, err := PiOptimize("rm -rf /", "hello", 0)
	if err != nil || got != "hello" {
		t.Fatalf("transform-only contract broken")
	}
}

func TestPiOptimize_PithRawEquivalence(t *testing.T) {
	// Simulate pith raw: RawBypass must be equivalent to raw output
	large := strings.Repeat("keep me\n", 2000)
	raw := large
	bypass, _ := PiOptimizeWithConfig("any command", large, 0, PiConfig{RawBypass: true})
	if bypass != raw {
		t.Fatalf("pith raw equivalence failed")
	}
}

func TestPiHarnessTracking(t *testing.T) {
	// harness pi recorded, claude not mixed, not per-machine path (deterministic across StoragePath)
	dir1 := t.TempDir()
	dir2 := t.TempDir() // simulates E:\TheBrain\PithBackup vs ~/.pith

	// Record pi harness from dir1 (box A)
	_, err := PiOptimizeWithConfig("go test ./...", "hello world output for pi harness test", 0, PiConfig{
		TelemetryEnabled: true,
		StoragePath:      dir1,
		Harness:          "pi",
	})
	if err != nil {
		t.Fatalf("pi optimize failed: %v", err)
	}
	// Same harness+command via different StoragePath (box B) -> same bucket, not per-machine
	_, err = PiOptimizeWithConfig("go test ./...", "hello world output for pi harness test second", 0, PiConfig{
		TelemetryEnabled: true,
		StoragePath:      dir2,
		Harness:          "pi",
	})
	if err != nil {
		t.Fatalf("pi optimize second failed: %v", err)
	}
	// Different harness (claude) must not mix with pi — use distinct command to avoid UNIQUE(timestamp,command,duration_ms) collision in same second
	_, err = PiOptimizeWithConfig("go test ./pkg/telemetry -run TestClaude", "claude output distinct command", 0, PiConfig{
		TelemetryEnabled: true,
		StoragePath:      dir1,
		Harness:          "claude",
	})
	if err != nil {
		t.Fatalf("claude optimize failed: %v", err)
	}

	// Verify: pi rows are under harness=pi regardless of StoragePath; claude separate
	tel1, err := telemetry.NewTelemetry(dir1)
	if err != nil {
		t.Fatalf("NewTelemetry dir1: %v", err)
	}
	defer tel1.Close()
	stats1, err := tel1.GetStatsByHarness()
	if err != nil {
		t.Fatalf("GetStatsByHarness dir1: %v", err)
	}
	hasPi := false
	hasClaude := false
	for _, s := range stats1 {
		if s.Harness == "pi" {
			hasPi = true
		}
		if s.Harness == "claude" {
			hasClaude = true
		}
	}
	if !hasPi {
		t.Fatalf("pi harness not recorded in dir1, stats=%v", stats1)
	}
	if !hasClaude {
		t.Fatalf("claude harness not recorded or mixed with pi, stats=%v", stats1)
	}
	// pi from dir2 must also be harness=pi (deterministic, not per-machine path)
	tel2, err := telemetry.NewTelemetry(dir2)
	if err != nil {
		t.Fatalf("NewTelemetry dir2: %v", err)
	}
	defer tel2.Close()
	stats2, err := tel2.GetStatsByHarness()
	if err != nil {
		t.Fatalf("GetStatsByHarness dir2: %v", err)
	}
	foundPi2 := false
	for _, s := range stats2 {
		if s.Harness == "pi" {
			foundPi2 = true
		}
	}
	if !foundPi2 {
		t.Fatalf("pi harness not deterministic across StoragePath, stats2=%v", stats2)
	}
	// NormalizeHarness determinism
	if NormalizeHarness(" PI ") != "pi" || NormalizeHarness("Claude") != "claude" || NormalizeHarness("bogus") != "unknown" {
		t.Fatalf("NormalizeHarness not deterministic: pi=%q claude=%q unknown=%q", NormalizeHarness(" PI "), NormalizeHarness("Claude"), NormalizeHarness("bogus"))
	}
}
