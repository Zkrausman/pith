package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

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
