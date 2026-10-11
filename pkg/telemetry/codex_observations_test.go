package telemetry

import (
	"bytes"
	"database/sql"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func newPrivateObservationStore(t *testing.T) *Telemetry {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("private Codex observations require owner-only Unix permissions")
	}
	tel, err := NewPrivateTelemetry(filepath.Join(t.TempDir(), "private-telemetry"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = tel.Close() })
	return tel
}

func int64ptr(value int64) *int64 { return &value }

func TestCodexObservationNullZeroAllowlistPrivacyAndSummary(t *testing.T) {
	tel := newPrivateObservationStore(t)
	if err := tel.Record(ExecutionRecord{Command: "ordinary command", OriginalTokens: 20, CompressedTokens: 10, Source: "terminal"}); err != nil {
		t.Fatal(err)
	}
	beforeOriginal, beforeCompressed, err := tel.GetStats("")
	if err != nil {
		t.Fatal(err)
	}

	if err := tel.RecordCodexProposedFamily("git status"); err != nil {
		t.Fatal(err)
	}
	if err := tel.RecordCodexProposedFamily("git status --token=sentinel-command"); err == nil {
		t.Fatal("arbitrary family text should be rejected")
	}
	if err := tel.RecordCodexProposedFamily(CodexFamilyOtherCompound); err != nil {
		t.Fatal(err)
	}
	for _, observation := range []CodexCaptureObservation{
		{Outcome: CodexCaptureComplete, ElapsedMs: int64ptr(2001), Selection: CodexSelectionReduced, OriginalBytes: int64ptr(100), DeliveredBytes: int64ptr(45)},
		{Outcome: CodexCaptureComplete, ElapsedMs: int64ptr(0), Selection: CodexSelectionRaw, OriginalBytes: int64ptr(0), DeliveredBytes: int64ptr(0)},
		{Outcome: CodexCaptureDeadline, ElapsedMs: int64ptr(30000), Selection: CodexSelectionRaw, OriginalBytes: int64ptr(120), DeliveredBytes: int64ptr(120)},
		{Outcome: CodexCaptureIncomplete, Selection: CodexSelectionUnknown},
	} {
		if err := tel.RecordCodexCapture(observation); err != nil {
			t.Fatal(err)
		}
	}
	for _, invalid := range []CodexCaptureObservation{
		{Outcome: CodexCaptureIncomplete, Selection: CodexSelectionRaw, OriginalBytes: int64ptr(0), DeliveredBytes: int64ptr(0)},
		{Outcome: CodexCaptureComplete, Selection: CodexSelectionRaw, OriginalBytes: int64ptr(0)},
		{Outcome: CodexCaptureComplete, Selection: CodexSelectionRaw, OriginalBytes: int64ptr(2), DeliveredBytes: int64ptr(1)},
		{Outcome: CodexCaptureComplete, Selection: CodexSelectionReduced, OriginalBytes: int64ptr(1), DeliveredBytes: int64ptr(1)},
	} {
		if err := tel.RecordCodexCapture(invalid); err == nil {
			t.Fatalf("invalid capture should be rejected: %#v", invalid)
		}
	}

	var elapsed sql.NullInt64
	var original, delivered sql.NullInt64
	if err := tel.DB.QueryRow(`SELECT elapsed_ms, original_bytes, delivered_bytes FROM codex_wrapper_observations
		WHERE kind = 'capture' AND capture_outcome = ? AND selection = ?`, CodexCaptureComplete, CodexSelectionRaw).
		Scan(&elapsed, &original, &delivered); err != nil {
		t.Fatal(err)
	}
	if !elapsed.Valid || elapsed.Int64 != 0 || !original.Valid || original.Int64 != 0 || !delivered.Valid || delivered.Int64 != 0 {
		t.Fatalf("verified zero values must remain known: elapsed=%#v original=%#v delivered=%#v", elapsed, original, delivered)
	}
	if err := tel.DB.QueryRow(`SELECT elapsed_ms, original_bytes, delivered_bytes FROM codex_wrapper_observations
		WHERE kind = 'capture' AND capture_outcome = ?`, CodexCaptureIncomplete).
		Scan(&elapsed, &original, &delivered); err != nil {
		t.Fatal(err)
	}
	if elapsed.Valid || original.Valid || delivered.Valid {
		t.Fatalf("unknown values must stay NULL: elapsed=%#v original=%#v delivered=%#v", elapsed, original, delivered)
	}

	summary, err := tel.GetCodexObservationSummary()
	if err != nil {
		t.Fatal(err)
	}
	if summary.ProposalCounts["git status"] != 1 || summary.ProposalCounts[CodexFamilyOtherCompound] != 1 ||
		summary.CaptureRecords != 4 || summary.CaptureOutcomes[CodexCaptureComplete] != 2 ||
		summary.CaptureOutcomes[CodexCaptureDeadline] != 1 || summary.CaptureOutcomes[CodexCaptureIncomplete] != 1 ||
		summary.ElapsedKnown != 3 || summary.ElapsedUnknown != 1 || summary.ByteCountsKnown != 3 || summary.ByteCountsUnknown != 1 ||
		summary.SlowReductionCount != 1 || summary.SlowReductionBytes != 55 {
		t.Fatalf("unexpected observation summary: %#v", summary)
	}
	afterOriginal, afterCompressed, err := tel.GetStats("")
	if err != nil || beforeOriginal != afterOriginal || beforeCompressed != afterCompressed {
		t.Fatalf("observations changed gain totals: before=%d/%d after=%d/%d err=%v", beforeOriginal, beforeCompressed, afterOriginal, afterCompressed, err)
	}
	var persisted string
	if err := tel.DB.QueryRow("SELECT group_concat(COALESCE(family, '') || ' ' || COALESCE(capture_outcome, '')) FROM codex_wrapper_observations").Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	for _, sentinel := range []string{"sentinel-command", "secret/path", "private-output", "session-sentinel", "model-sentinel"} {
		if strings.Contains(persisted, sentinel) {
			t.Fatalf("observation retained privacy sentinel %q", sentinel)
		}
	}
}

func TestCodexObservationsRequirePrivateStoreAndStayOutOfExportImportFTSAndDetails(t *testing.T) {
	ordinary, err := NewTelemetry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer ordinary.Close()
	if err := ordinary.RecordCodexProposedFamily("git status"); err == nil {
		t.Fatal("ordinary telemetry handle should not record private observations")
	}

	tel := newPrivateObservationStore(t)
	if err := tel.RecordCodexProposedFamily("other/compound"); err != nil {
		t.Fatal(err)
	}
	if err := tel.RecordCodexCapture(CodexCaptureObservation{Outcome: CodexCaptureIncomplete, Selection: CodexSelectionUnknown}); err != nil {
		t.Fatal(err)
	}
	if err := tel.Record(ExecutionRecord{Command: "git status", OriginalTokens: 10, CompressedTokens: 10, IsPassthrough: true, Harness: "pi", Source: "terminal"}); err != nil {
		t.Fatal(err)
	}
	for name, export := range map[string]func(*bytes.Buffer) error{
		"full":        func(out *bytes.Buffer) error { return tel.ExportJSONL(out) },
		"incremental": func(out *bytes.Buffer) error { return tel.ExportJSONLSince(out, 0) },
	} {
		t.Run(name, func(t *testing.T) {
			var output bytes.Buffer
			if err := export(&output); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(output.String(), "other/compound") || strings.Contains(output.String(), "capture_outcome") {
				t.Fatalf("private observations were exported: %s", output.String())
			}
		})
	}
	var ftsCount int
	if err := tel.DB.QueryRow("SELECT COUNT(*) FROM executions_fts WHERE command MATCH 'compound'").Scan(&ftsCount); err != nil || ftsCount != 0 {
		t.Fatalf("observations reached FTS: count=%d err=%v", ftsCount, err)
	}
	details, err := tel.GetDiscoveryDetails("other/compound", "", 10)
	if err != nil || len(details) != 0 {
		t.Fatalf("proposal appeared as command detail: %#v err=%v", details, err)
	}
	var encoded bytes.Buffer
	if err := tel.ExportJSONL(&encoded); err != nil {
		t.Fatal(err)
	}
	copyStore, err := NewTelemetry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer copyStore.Close()
	if err := copyStore.ImportJSONL(&encoded); err != nil {
		t.Fatal(err)
	}
	if err := copyStore.ImportJSONL(strings.NewReader("{\"kind\":\"proposal\",\"family\":\"git status\"}\n{\"kind\":\"capture\",\"captureOutcome\":\"complete_within_window\"}\n")); err != nil {
		t.Fatal(err)
	}
	imported, err := copyStore.GetCodexObservationSummary()
	if err != nil || imported.CaptureRecords != 0 || len(imported.ProposalCounts) != 0 {
		t.Fatalf("import copied private observations: %#v err=%v", imported, err)
	}
	var observationShapedExecutions int
	if err := copyStore.DB.QueryRow("SELECT COUNT(*) FROM executions WHERE command = ''").Scan(&observationShapedExecutions); err != nil || observationShapedExecutions != 0 {
		t.Fatalf("observation records became execution rows: %d err=%v", observationShapedExecutions, err)
	}
}

func TestCodexObservationRetentionAndRowCapTouchOnlyObservationTable(t *testing.T) {
	tel := newPrivateObservationStore(t)
	if err := tel.Record(ExecutionRecord{Command: "keep execution", OriginalTokens: 9, CompressedTokens: 4, Source: "terminal"}); err != nil {
		t.Fatal(err)
	}
	tx, err := tel.DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	stmt, err := tx.Prepare(`INSERT INTO codex_wrapper_observations (kind, family) VALUES ('proposal', ?)`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < CodexObservationMaxRows+1; i++ {
		if _, err := stmt.Exec("git status"); err != nil {
			t.Fatal(err)
		}
	}
	_ = stmt.Close()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if _, err := tel.DB.Exec(`UPDATE codex_wrapper_observations SET timestamp = datetime('now', '-100 days') WHERE id = (SELECT MIN(id) FROM codex_wrapper_observations)`); err != nil {
		t.Fatal(err)
	}
	if err := tel.RecordCodexProposedFamily("npm test"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tel.DB.QueryRow("SELECT COUNT(*) FROM codex_wrapper_observations").Scan(&count); err != nil || count != CodexObservationMaxRows {
		t.Fatalf("observation cap = %d, err=%v; want %d", count, err, CodexObservationMaxRows)
	}
	var oldCount int
	if err := tel.DB.QueryRow("SELECT COUNT(*) FROM codex_wrapper_observations WHERE timestamp < datetime('now', '-90 days')").Scan(&oldCount); err != nil || oldCount != 0 {
		t.Fatalf("expired observations remain: %d err=%v", oldCount, err)
	}
	var executionCount int
	if err := tel.DB.QueryRow("SELECT COUNT(*) FROM executions WHERE command = 'keep execution'").Scan(&executionCount); err != nil || executionCount != 1 {
		t.Fatalf("observation pruning changed existing executions: %d err=%v", executionCount, err)
	}
}

func TestCodexObservationResetPathsClearOnlyTheirIntendedData(t *testing.T) {
	t.Run("discover reset", func(t *testing.T) {
		tel := newPrivateObservationStore(t)
		if err := tel.Record(ExecutionRecord{Command: "kept command", OriginalTokens: 9, CompressedTokens: 4, Source: "terminal"}); err != nil {
			t.Fatal(err)
		}
		if err := tel.Record(ExecutionRecord{Command: "passthrough", OriginalTokens: 9, CompressedTokens: 9, IsPassthrough: true, Source: "terminal"}); err != nil {
			t.Fatal(err)
		}
		if err := tel.RecordCodexProposedFamily("git status"); err != nil {
			t.Fatal(err)
		}
		if err := tel.ResetPassthrough(); err != nil {
			t.Fatal(err)
		}
		var executionCount, observationCount int
		if err := tel.DB.QueryRow("SELECT COUNT(*) FROM executions").Scan(&executionCount); err != nil {
			t.Fatal(err)
		}
		if err := tel.DB.QueryRow("SELECT COUNT(*) FROM codex_wrapper_observations").Scan(&observationCount); err != nil {
			t.Fatal(err)
		}
		if executionCount != 1 || observationCount != 0 {
			t.Fatalf("discover reset counts executions=%d observations=%d", executionCount, observationCount)
		}
	})
	t.Run("all reset", func(t *testing.T) {
		tel := newPrivateObservationStore(t)
		if err := tel.Record(ExecutionRecord{Command: "execution", Source: "terminal"}); err != nil {
			t.Fatal(err)
		}
		if err := tel.RecordCodexProposedFamily("git status"); err != nil {
			t.Fatal(err)
		}
		if err := tel.ResetAll(); err != nil {
			t.Fatal(err)
		}
		var executionCount, observationCount int
		if err := tel.DB.QueryRow("SELECT COUNT(*) FROM executions").Scan(&executionCount); err != nil {
			t.Fatal(err)
		}
		if err := tel.DB.QueryRow("SELECT COUNT(*) FROM codex_wrapper_observations").Scan(&observationCount); err != nil {
			t.Fatal(err)
		}
		if executionCount != 0 || observationCount != 0 {
			t.Fatalf("all reset counts executions=%d observations=%d", executionCount, observationCount)
		}
	})
}

func TestCodexObservationSummaryCountsOnlyFixedLabels(t *testing.T) {
	tel := newPrivateObservationStore(t)
	for _, family := range []string{"git status", "git status", "other/compound", "go test"} {
		if err := tel.RecordCodexProposedFamily(family); err != nil {
			t.Fatal(err)
		}
	}
	var familyCount int
	if err := tel.DB.QueryRow("SELECT COUNT(DISTINCT family) FROM codex_wrapper_observations WHERE kind = 'proposal'").Scan(&familyCount); err != nil || familyCount != 3 {
		t.Fatalf("distinct stored labels=%d err=%v", familyCount, err)
	}
	for _, label := range []string{"git status", "other/compound", "go test"} {
		if !IsCodexObservationFamilyAllowed(label) {
			t.Fatalf("known label %q was rejected", label)
		}
	}
	if IsCodexObservationFamilyAllowed(fmt.Sprintf("unknown-%d-secret", time.Now().UnixNano())) {
		t.Fatal("unknown family should not be allowlisted")
	}
}
