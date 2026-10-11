package telemetry

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCodexWrapperPrivateStoreAndAggregateRecord(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("owner-only telemetry permission checks are Unix-specific")
	}
	storage := filepath.Join(t.TempDir(), "new-private-store")
	tel, err := NewPrivateTelemetry(storage)
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Close()
	for path, want := range map[string]os.FileMode{storage: 0700, filepath.Join(storage, "pith.db"): 0600} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != want {
			t.Fatalf("private permissions for %s = %v, %v; want %v", path, info, err, want)
		}
	}
	stamp := time.Date(2026, 10, 11, 12, 0, 0, 123456789, time.UTC)
	record := ExecutionRecord{
		Timestamp: stamp, Command: "go test", OriginalTokens: 4, CompressedTokens: 1,
		DurationMs: 25, ParserUsed: "tests", Harness: "codex", Source: SourceCodexWrapperPrivate,
		DecisionReason: DecisionTransformed,
	}
	if err := tel.Record(record); err != nil {
		t.Fatal(err)
	}
	// The existing composite key may collide. A duplicate aggregate record is
	// ignored without claiming exactly-once delivery.
	if err := tel.Record(record); err != nil {
		t.Fatalf("timestamp collision should be harmless: %v", err)
	}
	var count int
	if err := tel.DB.QueryRow("SELECT COUNT(*) FROM executions").Scan(&count); err != nil || count != 1 {
		t.Fatalf("private records = %d, %v", count, err)
	}
	recent, err := tel.GetRecentExecutions(2, SourceCodexWrapperPrivate)
	if err != nil || len(recent) != 1 || !recent[0].Timestamp.Equal(stamp) || recent[0].Harness != "codex" || recent[0].OriginalTokens != 4 || recent[0].CompressedTokens != 1 {
		t.Fatalf("private record did not round-trip: %#v, %v", recent, err)
	}
	original, compressed, err := tel.GetStats(SourceCodexWrapperPrivate)
	if err != nil || original != 4 || compressed != 1 {
		t.Fatalf("gain stats = %d/%d, %v", original, compressed, err)
	}
	families, err := tel.GetDiscoveryFamilies(SourceCodexWrapperPrivate)
	if err != nil || len(families) != 0 {
		t.Fatalf("transformed record should not be parser discovery: %#v, %v", families, err)
	}
	if err := tel.Record(ExecutionRecord{
		Timestamp: stamp.Add(time.Nanosecond), Command: "go test", OriginalTokens: 4,
		CompressedTokens: 4, IsPassthrough: true, Harness: "codex",
		Source: SourceCodexWrapperPrivate, DecisionReason: DecisionProtectedPassthrough,
	}); err != nil {
		t.Fatal(err)
	}
	families, err = tel.GetDiscoveryFamilies(SourceCodexWrapperPrivate)
	if err != nil || len(families) != 1 || families[0].Family != "go test" || families[0].InvocationCount != 1 {
		t.Fatalf("private go-test discovery family missing: %#v, %v", families, err)
	}
	details, err := tel.GetDiscoveryDetails("go test", SourceCodexWrapperPrivate, 10)
	if err != nil || len(details) != 1 || details[0].Command != "go test" {
		t.Fatalf("private go-test detail missing: %#v, %v", details, err)
	}
}

func TestCodexWrapperPrivateSourceIsExcludedFromBothExports(t *testing.T) {
	tel, err := NewTelemetry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Close()
	stamp := time.Date(2026, 10, 11, 12, 0, 0, 987654321, time.UTC)
	for _, record := range []ExecutionRecord{
		{Timestamp: stamp, Command: "go test", OriginalTokens: 8, CompressedTokens: 2, Source: SourceCodexWrapperPrivate, Harness: "codex"},
		{Timestamp: stamp.Add(time.Nanosecond), Command: "git status", OriginalTokens: 8, CompressedTokens: 2, Source: "terminal", Harness: "pi"},
	} {
		if err := tel.Record(record); err != nil {
			t.Fatal(err)
		}
	}
	for name, export := range map[string]func(*bytes.Buffer) error{
		"full":        func(out *bytes.Buffer) error { return tel.ExportJSONL(out) },
		"incremental": func(out *bytes.Buffer) error { return tel.ExportJSONLSince(out, 0) },
	} {
		t.Run(name, func(t *testing.T) {
			var out bytes.Buffer
			if err := export(&out); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(out.String(), SourceCodexWrapperPrivate) || strings.Contains(out.String(), `"go test"`) {
				t.Fatalf("private record exported: %s", out.String())
			}
			var record ExecutionRecord
			if err := json.NewDecoder(&out).Decode(&record); err != nil || record.Command != "git status" {
				t.Fatalf("ordinary record missing from export: %#v, %v", record, err)
			}
		})
	}
}

func TestCodexWrapperTimestampFractionalJSONRoundTripAndLegacyDefault(t *testing.T) {
	tel, err := NewTelemetry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Close()
	base := time.Date(2026, 10, 11, 12, 0, 0, 100, time.UTC)
	for i, stamp := range []time.Time{base, base.Add(200 * time.Nanosecond)} {
		if err := tel.Record(ExecutionRecord{Timestamp: stamp, Command: "fractional", DurationMs: int64(i), Source: "terminal"}); err != nil {
			t.Fatal(err)
		}
	}
	if err := tel.Record(ExecutionRecord{Command: "legacy timestamp", Source: "terminal"}); err != nil {
		t.Fatal(err)
	}
	var exported bytes.Buffer
	if err := tel.ExportJSONL(&exported); err != nil {
		t.Fatal(err)
	}
	copyStore, err := NewTelemetry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer copyStore.Close()
	if err := copyStore.ImportJSONL(&exported); err != nil {
		t.Fatal(err)
	}
	records, err := copyStore.SearchExecutions("fractional", "terminal", 10)
	if err != nil || len(records) != 2 {
		t.Fatalf("fractional timestamp records: %#v, %v", records, err)
	}
	got := map[time.Time]bool{}
	for _, record := range records {
		got[record.Timestamp] = true
	}
	if !got[base] || !got[base.Add(200*time.Nanosecond)] {
		t.Fatalf("fractional timestamps changed: %#v", got)
	}
	var legacy time.Time
	if err := tel.DB.QueryRow("SELECT timestamp FROM executions WHERE command = 'legacy timestamp'").Scan(&legacy); err != nil || legacy.IsZero() {
		t.Fatalf("legacy default timestamp changed: %v, %v", legacy, err)
	}
}

func TestNewPrivateTelemetrySkipsExistingPublicStorageWithoutChangingModes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("owner-only telemetry permission checks are Unix-specific")
	}
	publicDir := filepath.Join(t.TempDir(), "public-dir")
	if err := os.Mkdir(publicDir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPrivateTelemetry(publicDir); err == nil {
		t.Fatal("public directory should be rejected")
	}
	info, err := os.Stat(publicDir)
	if err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("directory permissions changed: %v, %v", info, err)
	}

	privateDir := filepath.Join(t.TempDir(), "private-dir")
	if err := os.Mkdir(privateDir, 0700); err != nil {
		t.Fatal(err)
	}
	fixture, err := NewTelemetryWithPath(filepath.Join(privateDir, "pith.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.Close(); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(privateDir, "pith.db")
	if err := os.Chmod(dbPath, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := NewPrivateTelemetry(privateDir); err == nil {
		t.Fatal("public database should be rejected")
	}
	info, err = os.Stat(dbPath)
	if err != nil || info.Mode().Perm() != 0644 {
		t.Fatalf("database permissions changed: %v, %v", info, err)
	}
}
