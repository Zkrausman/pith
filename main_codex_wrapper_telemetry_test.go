package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"pith/pkg/parser"
	"pith/pkg/runner"
	"pith/pkg/telemetry"
)

func runWrapperRecorderForTest(t *testing.T, request string) error {
	t.Helper()
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"_record-codex-wrapper"})
	cmd.SetIn(bytes.NewBufferString(request))
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	return cmd.Execute()
}

func TestCodexWrapperBypassReasonsValidateBeforeStorage(t *testing.T) {
	storage := filepath.Join(t.TempDir(), "must-not-exist")
	setWrapperRecorderEnvironment(t, storage)
	for _, request := range []string{
		`{"kind":"proposal","family":"python","bypassReason":"private-sentinel"}`,
		`{"kind":"capture","captureOutcome":"incomplete","selection":"unknown","bypassReason":"shell_syntax"}`,
		`{"kind":"execution","family":"python","originalRunes":1,"deliveredRunes":1,"durationMs":0,"outcome":"raw_fallback","bypassReason":"interpreter"}`,
		`{"originalRunes":1,"deliveredRunes":1,"durationMs":0,"outcome":"raw_fallback","bypassReason":"interpreter"}`,
	} {
		if err := runWrapperRecorderForTest(t, request); err == nil {
			t.Fatalf("invalid reason accepted: %s", request)
		}
	}
	if _, err := os.Stat(storage); !os.IsNotExist(err) {
		t.Fatalf("invalid reason created storage: %v", err)
	}
	if err := runWrapperRecorderForTest(t, `{"kind":"proposal","family":"other/compound","bypassReason":"shell_syntax"}`); err != nil {
		t.Fatal(err)
	}
	tel, err := telemetry.NewPrivateTelemetry(storage)
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Close()
	summary, err := tel.GetCodexObservationSummary()
	if err != nil || summary.BypassCounts["shell_syntax"] != 1 {
		t.Fatalf("reason was not persisted: %#v, %v", summary, err)
	}
	var output bytes.Buffer
	cmd := NewRootCmd()
	cmd.SetOut(&output)
	printCodexObservationSummary(cmd, summary)
	if !strings.Contains(output.String(), "shell_syntax") || strings.Contains(output.String(), "private-sentinel") {
		t.Fatalf("unsafe or missing summary: %s", output.String())
	}
}

func setWrapperRecorderEnvironment(t *testing.T, storage string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("PITH_STORAGE", storage)
}

func TestCodexWrapperRecorderStoresOnlyBoundedAggregateMetadata(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("owner-only telemetry permission checks are Unix-specific")
	}
	storage := filepath.Join(t.TempDir(), "private")
	setWrapperRecorderEnvironment(t, storage)
	if err := runWrapperRecorderForTest(t, `{"originalRunes":8,"deliveredRunes":4,"durationMs":12,"outcome":"transformed"}`); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(storage)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("new telemetry directory mode: %v, %v", info, err)
	}
	info, err = os.Stat(filepath.Join(storage, "pith.db"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("new telemetry database mode: %v, %v", info, err)
	}
	tel, err := telemetry.NewTelemetryWithPath(filepath.Join(storage, "pith.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Close()
	records, err := tel.GetRecentExecutions(1, telemetry.SourceCodexWrapperPrivate)
	if err != nil || len(records) != 1 {
		t.Fatalf("recorded rows: %#v, %v", records, err)
	}
	record := records[0]
	if record.Command != "go test" || record.Harness != "codex" || record.Source != telemetry.SourceCodexWrapperPrivate ||
		record.OriginalTokens != 2 || record.CompressedTokens != 1 || record.DurationMs != 12 ||
		record.DecisionReason != telemetry.DecisionTransformed || record.IsPassthrough || record.ParserUsed != "tests" ||
		record.Model != "unknown" || record.InputCostPerMillion != nil || record.OriginalContent != "" || record.CompressedContent != "" {
		t.Fatalf("unexpected aggregate record: %#v", record)
	}
	if !record.Timestamp.After(time.Now().Add(-time.Minute)) {
		t.Fatalf("timestamp not set by the recorder: %v", record.Timestamp)
	}
}

func TestCodexWrapperRecorderRejectsMalformedRequestsBeforeStorage(t *testing.T) {
	storage := filepath.Join(t.TempDir(), "must-not-exist")
	setWrapperRecorderEnvironment(t, storage)
	for _, request := range []string{
		`{"originalRunes":1,"deliveredRunes":1,"durationMs":0,"outcome":"raw_fallback","extra":true}`,
		`{"originalRunes":1,"deliveredRunes":1,"durationMs":0,"outcome":"raw_fallback"} {}`,
		`{"deliveredRunes":1,"durationMs":0,"outcome":"raw_fallback"}`,
		`{"originalRunes":null,"deliveredRunes":1,"durationMs":0,"outcome":"raw_fallback"}`,
		`{"originalRunes":1,"durationMs":0,"outcome":"raw_fallback"}`,
		`{"originalRunes":1,"deliveredRunes":null,"durationMs":0,"outcome":"raw_fallback"}`,
		`{"originalRunes":1,"deliveredRunes":1,"outcome":"raw_fallback"}`,
		`{"originalRunes":1,"deliveredRunes":1,"durationMs":null,"outcome":"raw_fallback"}`,
		`{"originalRunes":-1,"deliveredRunes":-1,"durationMs":0,"outcome":"raw_fallback"}`,
		`{"originalRunes":1,"deliveredRunes":0,"durationMs":0,"outcome":"raw_fallback"}`,
		`{"originalRunes":1,"deliveredRunes":2,"durationMs":0,"outcome":"transformed"}`,
		`{"originalRunes":1,"deliveredRunes":1,"durationMs":999999999,"outcome":"raw_fallback"}`,
		`{"originalRunes":1,"deliveredRunes":1,"durationMs":0,"outcome":"other"}`,
	} {
		if err := runWrapperRecorderForTest(t, request); err == nil {
			t.Fatalf("request should fail: %s", request)
		}
	}
	if _, err := os.Stat(storage); !os.IsNotExist(err) {
		t.Fatalf("invalid requests created telemetry storage: %v", err)
	}
}

func TestCodexWrapperRecorderAcceptsExplicitZeroCounts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("owner-only telemetry permission checks are Unix-specific")
	}
	storage := filepath.Join(t.TempDir(), "private")
	setWrapperRecorderEnvironment(t, storage)
	if err := runWrapperRecorderForTest(t, `{"originalRunes":0,"deliveredRunes":0,"durationMs":0,"outcome":"raw_fallback"}`); err != nil {
		t.Fatal(err)
	}
	tel, err := telemetry.NewTelemetryWithPath(filepath.Join(storage, "pith.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Close()
	records, err := tel.GetRecentExecutions(1, telemetry.SourceCodexWrapperPrivate)
	if err != nil || len(records) != 1 {
		t.Fatalf("recorded zero-count rows: %#v, %v", records, err)
	}
	if records[0].OriginalTokens != 0 || records[0].CompressedTokens != 0 || records[0].DurationMs != 0 || !records[0].IsPassthrough {
		t.Fatalf("unexpected zero-count aggregate: %#v", records[0])
	}
}

func TestCodexWrapperRecorderRejectsMalformedConfigBeforeStorage(t *testing.T) {
	storage := filepath.Join(t.TempDir(), "invalid-config")
	if err := os.Mkdir(storage, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(storage, "config.json"), []byte(`{"storage_path":`), 0600); err != nil {
		t.Fatal(err)
	}
	setWrapperRecorderEnvironment(t, storage)
	if err := runWrapperRecorderForTest(t, `{"originalRunes":8,"deliveredRunes":4,"durationMs":12,"outcome":"transformed"}`); err == nil {
		t.Fatal("malformed config should fail")
	}
	if _, err := os.Stat(filepath.Join(storage, "pith.db")); !os.IsNotExist(err) {
		t.Fatalf("malformed config opened telemetry storage: %v", err)
	}
}

func TestCodexWrapperRuneEstimatorMatchesTextEstimator(t *testing.T) {
	for _, value := range []string{"🙂abcd", "", "🙂🙂🙂🙂🙂🙂🙂🙂🙂"} {
		want := len([]rune(value)) / 4
		got := runner.EstimateTokensWithHeuristic(value, 4)
		fromCount := runner.EstimateTokensFromRuneCount(len([]rune(value)), 4)
		if got != want || fromCount != want {
			t.Fatalf("rune estimate for %q: text=%d count=%d want=%d", value, got, fromCount, want)
		}
	}
}

func TestCodexWrapperFallbackStoresEqualCounts(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("owner-only telemetry permission checks are Unix-specific")
	}
	storage := filepath.Join(t.TempDir(), "private")
	setWrapperRecorderEnvironment(t, storage)
	if err := runWrapperRecorderForTest(t, `{"originalRunes":7,"deliveredRunes":7,"durationMs":9,"outcome":"raw_fallback"}`); err != nil {
		t.Fatal(err)
	}
	tel, err := telemetry.NewTelemetryWithPath(filepath.Join(storage, "pith.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Close()
	records, err := tel.GetRecentExecutions(1, telemetry.SourceCodexWrapperPrivate)
	if err != nil || len(records) != 1 || records[0].OriginalTokens != records[0].CompressedTokens || !records[0].IsPassthrough || records[0].DecisionReason != telemetry.DecisionProtectedPassthrough {
		encoded, _ := json.Marshal(records)
		t.Fatalf("fallback accounting: %s, %v", encoded, err)
	}
}

func TestCodexWrapperExecutionMetadataIsPrivateAndTruthful(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("owner-only telemetry permission checks are Unix-specific")
	}
	storage := filepath.Join(t.TempDir(), "private")
	setWrapperRecorderEnvironment(t, storage)
	for _, request := range []string{
		`{"kind":"execution","family":"rg","parser":"grep","originalRunes":80,"deliveredRunes":10,"durationMs":12,"outcome":"transformed"}`,
		`{"kind":"execution","family":"git status","parser":"","originalRunes":10,"deliveredRunes":10,"durationMs":20,"outcome":"raw_fallback"}`,
	} {
		if err := runWrapperRecorderForTest(t, request); err != nil {
			t.Fatalf("record execution metadata: %v", err)
		}
	}
	tel, err := telemetry.NewTelemetryWithPath(filepath.Join(storage, "pith.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Close()
	records, err := tel.GetRecentExecutions(10, telemetry.SourceCodexWrapperPrivate)
	if err != nil || len(records) != 2 {
		t.Fatalf("private wrapper rows: %#v, %v", records, err)
	}
	byFamily := map[string]telemetry.ExecutionRecord{}
	for _, record := range records {
		byFamily[record.Command] = record
		if record.Source != telemetry.SourceCodexWrapperPrivate || record.Harness != "codex" ||
			record.Model != "unknown" || record.InputCostPerMillion != nil ||
			record.OriginalContent != "" || record.CompressedContent != "" {
			t.Fatalf("execution metadata retained a nonaggregate detail: %#v", record)
		}
	}
	transformed := byFamily["rg"]
	if transformed.ParserUsed != "grep" || transformed.DecisionReason != telemetry.DecisionTransformed || transformed.IsPassthrough ||
		transformed.OriginalTokens <= transformed.CompressedTokens {
		t.Fatalf("transformed execution metadata is inaccurate: %#v", transformed)
	}
	fallback := byFamily["git status"]
	if fallback.ParserUsed != "" || fallback.DecisionReason != telemetry.DecisionProtectedPassthrough || !fallback.IsPassthrough ||
		fallback.OriginalTokens != fallback.CompressedTokens {
		t.Fatalf("raw fallback must be truthful passthrough accounting: %#v", fallback)
	}
	var exported bytes.Buffer
	if err := tel.ExportJSONL(&exported); err != nil {
		t.Fatal(err)
	}
	if exported.Len() != 0 {
		t.Fatalf("private wrapper execution leaked into export: %s", exported.String())
	}
}

func TestCodexWrapperExecutionLabelsMatchRegisteredParsers(t *testing.T) {
	for _, candidate := range parser.GetAllParsers() {
		if !codexParserNameAllowed(candidate.Name()) {
			t.Errorf("registered parser %q is rejected by wrapper metadata validation", candidate.Name())
		}
	}
	if codexParserNameAllowed("parser-sentinel-secret") {
		t.Fatal("unknown parser label must be rejected")
	}
}

func TestCodexWrapperExecutionRejectsUnregisteredLabelsBeforeStorage(t *testing.T) {
	storage := filepath.Join(t.TempDir(), "must-not-exist")
	setWrapperRecorderEnvironment(t, storage)
	for _, request := range []string{
		`{"kind":"execution","family":"rg","parser":"parser-sentinel-secret","originalRunes":8,"deliveredRunes":4,"durationMs":1,"outcome":"transformed"}`,
		`{"kind":"execution","family":"command-sentinel-secret","parser":"grep","originalRunes":8,"deliveredRunes":4,"durationMs":1,"outcome":"transformed"}`,
		`{"kind":"execution","family":"rg","parser":"grep","originalRunes":8,"deliveredRunes":8,"durationMs":1,"outcome":"transformed"}`,
		`{"kind":"execution","family":"rg","parser":"grep","originalRunes":8,"deliveredRunes":4,"durationMs":1,"outcome":"raw_fallback"}`,
	} {
		if err := runWrapperRecorderForTest(t, request); err == nil {
			t.Errorf("invalid execution metadata was accepted: %s", request)
		}
	}
	if _, err := os.Stat(storage); !os.IsNotExist(err) {
		t.Fatalf("invalid execution metadata created storage: %v", err)
	}
}

func TestCodexWrapperProposalAndCaptureRequestsStoreOnlyPrivateObservations(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("owner-only telemetry permission checks are Unix-specific")
	}
	storage := filepath.Join(t.TempDir(), "private")
	setWrapperRecorderEnvironment(t, storage)
	for _, request := range []string{
		`{"kind":"proposal","family":"git status"}`,
		`{"kind":"proposal","family":"other/compound"}`,
		`{"kind":"capture","captureOutcome":"complete_within_window","elapsedMs":0,"selection":"raw","originalBytes":0,"deliveredBytes":0}`,
		`{"kind":"capture","captureOutcome":"incomplete","elapsedMs":null,"selection":"unknown","originalBytes":null,"deliveredBytes":null}`,
	} {
		if err := runWrapperRecorderForTest(t, request); err != nil {
			t.Fatalf("record %s: %v", request, err)
		}
	}
	tel, err := telemetry.NewTelemetryWithPath(filepath.Join(storage, "pith.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Close()
	summary, err := tel.GetCodexObservationSummary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.ProposalCounts["git status"] != 1 || summary.ProposalCounts["other/compound"] != 1 ||
		summary.CaptureRecords != 2 || summary.CaptureOutcomes[telemetry.CodexCaptureComplete] != 1 ||
		summary.CaptureOutcomes[telemetry.CodexCaptureIncomplete] != 1 || summary.ElapsedKnown != 1 || summary.ElapsedUnknown != 1 ||
		summary.ByteCountsKnown != 1 || summary.ByteCountsUnknown != 1 {
		t.Fatalf("unexpected stored observations: %#v", summary)
	}
	var executions int
	if err := tel.DB.QueryRow("SELECT COUNT(*) FROM executions").Scan(&executions); err != nil || executions != 0 {
		t.Fatalf("observation calls created execution rows: %d err=%v", executions, err)
	}
}

func TestCodexWrapperObservationRequestsRejectUnknownLabelsAndInconsistentUnknowns(t *testing.T) {
	storage := filepath.Join(t.TempDir(), "must-not-exist")
	setWrapperRecorderEnvironment(t, storage)
	for _, request := range []string{
		`{"kind":"proposal","family":"git status --token=sentinel-secret"}`,
		`{"kind":"capture","captureOutcome":"incomplete","selection":"raw","originalBytes":0,"deliveredBytes":0}`,
		`{"kind":"capture","captureOutcome":"complete_within_window","selection":"raw","originalBytes":0}`,
		`{"kind":"capture","captureOutcome":"unknown","selection":"raw","originalBytes":0,"deliveredBytes":0}`,
		`{"kind":"capture","captureOutcome":"complete_within_window","selection":"reduced","originalBytes":10,"deliveredBytes":10}`,
	} {
		if err := runWrapperRecorderForTest(t, request); err == nil {
			t.Fatalf("request should be rejected: %s", request)
		}
	}
	if _, err := os.Stat(storage); !os.IsNotExist(err) {
		t.Fatalf("invalid observations created telemetry storage: %v", err)
	}
}

func TestDiscoverIncludesPrivateCodexSummaryWithoutAddingGain(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("owner-only telemetry permission checks are Unix-specific")
	}
	storage := filepath.Join(t.TempDir(), "private")
	setWrapperRecorderEnvironment(t, storage)
	tel, err := telemetry.NewPrivateTelemetry(storage)
	if err != nil {
		t.Fatal(err)
	}
	if err := tel.RecordCodexProposedFamily("npm test"); err != nil {
		t.Fatal(err)
	}
	if err := tel.RecordCodexCapture(telemetry.CodexCaptureObservation{
		Outcome: telemetry.CodexCaptureComplete, ElapsedMs: int64Pointer(2401), Selection: telemetry.CodexSelectionReduced,
		OriginalBytes: int64Pointer(500), DeliveredBytes: int64Pointer(200),
	}); err != nil {
		t.Fatal(err)
	}
	if err := tel.Close(); err != nil {
		t.Fatal(err)
	}
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"discover"})
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"Private Codex Observations", "npm test", "complete_within_window",
		"native elapsed ms known/unknown 1/0", "byte counts known/unknown 1/0",
		"Recorded reductions after captures longer than 2 seconds: 1 captures, 300 bytes",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Fatalf("discover output missing %q: %s", expected, output.String())
		}
	}
}

func int64Pointer(value int64) *int64 { return &value }
