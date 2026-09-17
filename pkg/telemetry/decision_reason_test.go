package telemetry

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
)

func decisionTestStore(t *testing.T) *Telemetry {
	t.Helper()
	tel, err := NewTelemetry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { tel.Close() })
	return tel
}

func TestDecisionReasonBoundaries(t *testing.T) {
	for _, tc := range []struct{ input, want DecisionReason }{
		{DecisionTransformed, DecisionTransformed},
		{DecisionProtectedPassthrough, DecisionProtectedPassthrough},
		{DecisionUnsupportedParser, DecisionUnsupportedParser},
		{DecisionRejectedNonReduction, DecisionRejectedNonReduction},
		{DecisionUnknown, DecisionUnknown},
		{"", DecisionUnknown}, {"arbitrary synthetic text", DecisionUnknown},
		{"TRANSFORMED", DecisionUnknown}, {" transformed ", DecisionUnknown},
	} {
		t.Run(string(tc.input), func(t *testing.T) {
			if got := NormalizeDecisionReason(tc.input); got != tc.want {
				t.Fatalf("normalize = %q", got)
			}
			tel := decisionTestStore(t)
			if err := tel.Record(ExecutionRecord{Command: "fixture --token=synthetic", OriginalContent: "raw fixture", CompressedContent: "parsed fixture", DecisionReason: tc.input}); err != nil {
				t.Fatal(err)
			}
			// Inspect SQL directly: API read normalization must not hide an invalid write.
			var reason, original, compressed, command string
			if err := tel.DB.QueryRow("SELECT decision_reason, original_content, compressed_content, command FROM executions").Scan(&reason, &original, &compressed, &command); err != nil {
				t.Fatal(err)
			}
			if reason != string(tc.want) || original != "" || compressed != "" || command != "fixture --token=[REDACTED]" {
				t.Fatalf("stored = %q %q %q %q", reason, original, compressed, command)
			}
			recent, err := tel.GetRecentExecutions(10, "")
			if err != nil || len(recent) != 1 || recent[0].DecisionReason != tc.want {
				t.Fatalf("recent: %#v %v", recent, err)
			}
			detail, err := tel.GetExecutionDetails(recent[0].ID)
			if err != nil || detail.DecisionReason != tc.want {
				t.Fatalf("detail: %#v %v", detail, err)
			}
			for _, fallback := range []bool{false, true} {
				if fallback { // Force the existing LIKE fallback without depending on FTS token syntax.
					if _, err := tel.DB.Exec("DROP TABLE executions_fts"); err != nil {
						t.Fatal(err)
					}
				}
				found, err := tel.SearchExecutions("fixture", "", 10)
				if err != nil || len(found) != 1 || found[0].DecisionReason != tc.want {
					t.Fatalf("search fallback=%v: %#v %v", fallback, found, err)
				}
			}
		})
	}
}

func TestDecisionReasonJSONLRoundTrip(t *testing.T) {
	tel := decisionTestStore(t)
	values := []string{`"transformed"`, `"protected_passthrough"`, `"unsupported_parser"`, `"rejected_non_reduction"`, `"unknown"`, `""`, `"synthetic arbitrary text"`, `null`, `42`, `{ "text": "synthetic" }`, `[]`, ""}
	for i, value := range values {
		field := ""
		if value != "" {
			field = `,"decision_reason":` + value
		}
		line := fmt.Sprintf(`{"Timestamp":"2026-01-01T00:00:00Z","Command":"fixture-%d --token=[REDACTED]","DurationMs":7,"OriginalContent":"raw fixture","CompressedContent":"parsed fixture"%s}`, i, field)
		if err := tel.ImportJSONL(strings.NewReader(line)); err != nil {
			t.Fatal(err)
		}
	}
	var invalid, content int
	if err := tel.DB.QueryRow("SELECT COUNT(*) FROM executions WHERE decision_reason NOT IN ('transformed','protected_passthrough','unsupported_parser','rejected_non_reduction','unknown')").Scan(&invalid); err != nil {
		t.Fatal(err)
	}
	if err := tel.DB.QueryRow("SELECT COUNT(*) FROM executions WHERE original_content != '' OR compressed_content != ''").Scan(&content); err != nil {
		t.Fatal(err)
	}
	if invalid != 0 || content != 0 {
		t.Fatalf("invalid=%d retained=%d", invalid, content)
	}
	for _, since := range []bool{false, true} {
		var out bytes.Buffer
		var err error
		if since {
			err = tel.ExportJSONLSince(&out, 0)
		} else {
			err = tel.ExportJSONL(&out)
		}
		if err != nil {
			t.Fatal(err)
		}
		decoder := json.NewDecoder(bytes.NewReader(out.Bytes()))
		for i := range values {
			var fields map[string]any
			if err := decoder.Decode(&fields); err != nil {
				t.Fatal(err)
			}
			want := "unknown"
			if i < 5 {
				if err := json.Unmarshal([]byte(values[i]), &want); err != nil {
					t.Fatal(err)
				}
			}
			if fields["decision_reason"] != want || fields["OriginalContent"] != "" || fields["CompressedContent"] != "" || fields["Command"] != fmt.Sprintf("fixture-%d --token=[REDACTED]", i) {
				t.Fatalf("export fields: %#v", fields)
			}
		}
		copy := decisionTestStore(t)
		for range 2 {
			if err := copy.ImportJSONL(bytes.NewReader(out.Bytes())); err != nil {
				t.Fatal(err)
			}
		}
		// Same timestamp/command/duration but a different reason must still be ignored.
		conflict := `{"Timestamp":"2026-01-01T00:00:00Z","Command":"fixture-0 --token=[REDACTED]","DurationMs":7,"decision_reason":"protected_passthrough"}`
		if err := copy.ImportJSONL(strings.NewReader(conflict)); err != nil {
			t.Fatal(err)
		}
		var count int
		var reason string
		if err := copy.DB.QueryRow("SELECT COUNT(*) FROM executions").Scan(&count); err != nil {
			t.Fatal(err)
		}
		if err := copy.DB.QueryRow("SELECT decision_reason FROM executions WHERE command = 'fixture-0 --token=[REDACTED]'").Scan(&reason); err != nil {
			t.Fatal(err)
		}
		if count != len(values) || reason != "transformed" {
			t.Fatalf("duplicate identity changed: %d %q", count, reason)
		}
	}
}

func TestDecisionReasonLegacyMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE executions (
 id INTEGER PRIMARY KEY, timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
 command TEXT, original_tokens INTEGER, compressed_tokens INTEGER, duration_ms INTEGER,
 parser_used TEXT, is_passthrough BOOLEAN);
 INSERT INTO executions VALUES (1, '2026-01-01 00:00:00', 'git status', 100, 1, 7, 'git_status', 0);`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	for range 2 {
		tel, err := NewTelemetryWithPath(path)
		if err != nil {
			t.Fatal(err)
		}
		rec, err := tel.GetExecutionDetails(1)
		if err != nil || rec.DecisionReason != DecisionUnknown {
			t.Fatalf("legacy inferred: %#v %v", rec, err)
		}
		var indexSQL string
		if err := tel.DB.QueryRow("SELECT sql FROM sqlite_master WHERE name = 'idx_executions_unique'").Scan(&indexSQL); err != nil {
			t.Fatal(err)
		}
		if indexSQL != "CREATE UNIQUE INDEX idx_executions_unique ON executions(timestamp, command, duration_ms)" {
			t.Fatalf("identity changed: %s", indexSQL)
		}
		tel.Close()
	}
	// Read-only old schema: genuine additive migration failure must not be swallowed.
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec("ALTER TABLE executions DROP COLUMN decision_reason"); err != nil {
		t.Fatal(err)
	}
	db.Close()
	if tel, err := NewTelemetryWithPath("file:" + filepath.ToSlash(path) + "?mode=ro"); err == nil {
		tel.Close()
		t.Fatal("read-only migration unexpectedly succeeded")
	} else if !strings.Contains(err.Error(), "add decision_reason column") {
		t.Fatalf("wrong failure: %v", err)
	}
}

func TestDecisionReasonLegacyReadAndExportNormalization(t *testing.T) {
	tel := decisionTestStore(t)
	for i, reason := range []string{"transformed", "arbitrary synthetic text"} {
		if err := tel.Record(ExecutionRecord{Command: fmt.Sprintf("fixture-%d", i)}); err != nil {
			t.Fatal(err)
		}
		// Simulate legacy/cross-version content arriving after initialization.
		if _, err := tel.DB.Exec("UPDATE executions SET original_content='raw fixture', compressed_content='parsed fixture', decision_reason=? WHERE command=?", reason, fmt.Sprintf("fixture-%d", i)); err != nil {
			t.Fatal(err)
		}
	}
	recent, err := tel.GetRecentExecutions(10, "")
	if err != nil || len(recent) != 2 {
		t.Fatalf("recent: %#v %v", recent, err)
	}
	for _, rec := range recent {
		want := DecisionUnknown
		if rec.Command == "fixture-0" {
			want = DecisionTransformed
		}
		if rec.DecisionReason != want {
			t.Fatalf("read normalization: %#v", rec)
		}
		detail, err := tel.GetExecutionDetails(rec.ID)
		if err != nil || detail.DecisionReason != want {
			t.Fatalf("detail normalization: %#v %v", detail, err)
		}
		found, err := tel.SearchExecutions(rec.Command, "", 10)
		if err != nil || len(found) != 1 || found[0].DecisionReason != want {
			t.Fatalf("search normalization: %#v %v", found, err)
		}
	}
	for _, since := range []int64{-1, 0, 1} {
		var out bytes.Buffer
		var err error
		if since == -1 {
			err = tel.ExportJSONL(&out)
		} else {
			err = tel.ExportJSONLSince(&out, since)
		}
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(out.String(), "raw fixture") || strings.Contains(out.String(), "parsed fixture") || strings.Contains(out.String(), "arbitrary synthetic text") {
			t.Fatalf("export leaked content/reason: %s", &out)
		}
		decoder := json.NewDecoder(&out)
		count := 0
		for {
			var rec ExecutionRecord
			err := decoder.Decode(&rec)
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Fatal(err)
			}
			count++
			if rec.OriginalContent != "" || rec.CompressedContent != "" || rec.ID <= since {
				t.Fatalf("unsafe export: %#v", rec)
			}
		}
		want := 2
		if since == 1 {
			want = 1
		}
		if count != want {
			t.Fatalf("export count %d, want %d", count, want)
		}
	}
}
