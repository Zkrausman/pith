package telemetry

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DecisionReason is bounded accounting metadata, never a free-form explanation.
type DecisionReason string

const (
	DecisionTransformed          DecisionReason = "transformed"
	DecisionProtectedPassthrough DecisionReason = "protected_passthrough"
	DecisionUnsupportedParser    DecisionReason = "unsupported_parser"
	DecisionRejectedNonReduction DecisionReason = "rejected_non_reduction"
	DecisionUnknown              DecisionReason = "unknown"
)

// NormalizeDecisionReason does not infer decisions from other execution fields.
func NormalizeDecisionReason(reason DecisionReason) DecisionReason {
	switch reason {
	case DecisionTransformed, DecisionProtectedPassthrough, DecisionUnsupportedParser, DecisionRejectedNonReduction, DecisionUnknown:
		return reason
	default:
		return DecisionUnknown
	}
}

// UnmarshalJSON treats invalid imported reason values as unknown, including
// non-string values, without retaining their contents.
func (r *DecisionReason) UnmarshalJSON(data []byte) error {
	var value string
	if err := json.Unmarshal(data, &value); err != nil {
		*r = DecisionUnknown
		return nil
	}
	*r = NormalizeDecisionReason(DecisionReason(value))
	return nil
}

type ExecutionRecord struct {
	DecisionReason      DecisionReason `json:"decision_reason"`
	ID                  int64
	Timestamp           time.Time
	Command             string
	OriginalTokens      int
	CompressedTokens    int
	OriginalContent     string
	CompressedContent   string
	DurationMs          int64
	ParserUsed          string
	IsPassthrough       bool
	Source              string
	Harness             string
	Model               string   `json:"model"`
	InputCostPerMillion *float64 `json:"inputCostPerMillion,omitempty"`
}

// ModelSavings separates execution-time prices from records that need a
// configured fallback (including telemetry created before model pricing).
type ModelSavings struct {
	Model               string
	Original            int
	Compressed          int
	RecordedUSD         float64
	FallbackTokens      int
	InputCostPerMillion *float64
}

// normalizeHarness returns canonical harness: pi|claude|gemini|codex|jules|unknown.
// Deterministic: case/space insensitive, never uses StoragePath.
func normalizeHarness(h string) string {
	switch strings.ToLower(strings.TrimSpace(h)) {
	case "pi":
		return "pi"
	case "claude":
		return "claude"
	case "gemini":
		return "gemini"
	case "codex":
		return "codex"
	case "jules":
		return "jules"
	case "antigravity":
		return "antigravity"
	default:
		return "unknown"
	}
}

type Telemetry struct {
	DB      *sql.DB
	private bool
}

// SourceCodexWrapperPrivate identifies aggregate-only records created by the
// completed-output Codex wrapper. These local rows are excluded from JSONL
// export and sync.
const SourceCodexWrapperPrivate = "codex-wrapper-private"

// Codex wrapper observations are local-only, aggregate measurements. Their
// bounded row count and age apply only to this table, never executions.
const (
	CodexObservationMaxRows             = 5000
	CodexObservationRetentionDays       = 90
	CodexObservationMaxElapsedMs  int64 = 24 * 60 * 60 * 1000
	CodexObservationMaxBytes      int64 = 1 << 40
)

const (
	CodexCaptureComplete   = "complete_within_window"
	CodexCaptureDeadline   = "deadline_stream"
	CodexCaptureCap        = "cap_stream"
	CodexCaptureIncomplete = "incomplete"

	CodexSelectionReduced = "reduced"
	CodexSelectionRaw     = "raw"
	CodexSelectionUnknown = "unknown"

	CodexFamilyOtherCompound = "other/compound"
)

var codexObservationFamilies = map[string]struct{}{
	"git status": {}, "git diff": {}, "git log": {}, "git show": {}, "git branch": {},
	"go test": {}, "go build": {}, "go run": {}, "go vet": {},
	"npm test": {}, "npm run": {}, "npm install": {}, "npm ci": {},
	"pnpm test": {}, "pnpm run": {}, "pnpm install": {},
	"yarn test": {}, "yarn run": {}, "yarn install": {},
	"bun test": {}, "bun run": {}, "bun install": {},
	"python": {}, "pytest": {}, "node": {}, "npx": {}, "cargo": {}, "rustc": {},
	"make": {}, "cmake": {}, "docker": {}, "kubectl": {}, "terraform": {},
	"rg": {}, "grep": {}, "find": {}, "ls": {}, "cat": {}, CodexFamilyOtherCompound: {},
}

// CodexCaptureObservation contains only closed-set labels and numeric
// measurements. Nil pointers represent unknown values; zero represents a
// verified empty measurement.
type CodexCaptureObservation struct {
	Outcome        string
	ElapsedMs      *int64 // Native command wall-clock time from start to observed exit.
	Selection      string
	OriginalBytes  *int64
	DeliveredBytes *int64
}

// CodexObservationSummary is a small local summary of retained wrapper
// observations. It is deliberately independent from execution gain totals.
type CodexObservationSummary struct {
	ProposalCounts     map[string]int
	CaptureOutcomes    map[string]int
	SelectionCounts    map[string]int
	CaptureRecords     int
	ElapsedKnown       int
	ElapsedUnknown     int
	ByteCountsKnown    int
	ByteCountsUnknown  int
	SlowReductionCount int
	SlowReductionBytes int64
}

var sensitiveCommandValue = regexp.MustCompile(`(?i)((?:--?(?:api[-_]?key|token|secret|password)(?:=|\s+)|(?:api[-_]?key|token|secret|password)\s*=\s*|authorization:\s*bearer\s+))[^\s"']+`)

func redactCommand(command string) string {
	return sensitiveCommandValue.ReplaceAllString(command, `${1}[REDACTED]`)
}

// RedactForStorage removes common credential values before text is persisted.
func RedactForStorage(text string) string {
	return redactCommand(text)
}

func NewTelemetry(storagePath string) (*Telemetry, error) {
	if storagePath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		storagePath = filepath.Join(home, ".pith")
	}

	if err := os.MkdirAll(storagePath, 0755); err != nil {
		return nil, err
	}

	dbPath := filepath.Join(storagePath, "pith.db")
	return NewTelemetryWithPath(dbPath)
}

func NewTelemetryWithPath(dbPath string) (*Telemetry, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}

	t := &Telemetry{DB: db}
	if err := t.init(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return t, nil
}

// NewPrivateTelemetry opens a telemetry store only when both its directory and
// existing database are private. It creates a missing final directory and
// database with owner-only permissions and never changes existing permissions.
func NewPrivateTelemetry(storagePath string) (*Telemetry, error) {
	if runtime.GOOS == "windows" {
		return nil, fmt.Errorf("private telemetry permission checks are unsupported on Windows")
	}
	if strings.TrimSpace(storagePath) == "" {
		return nil, fmt.Errorf("private telemetry storage path is empty")
	}
	absolute, err := filepath.Abs(storagePath)
	if err != nil {
		return nil, fmt.Errorf("resolve private telemetry storage path: %w", err)
	}
	if err := ensurePrivateDirectory(absolute); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(absolute, "pith.db")
	createdDB, err := ensurePrivateFile(dbPath)
	if err != nil {
		return nil, err
	}
	for _, sidecar := range []string{dbPath + "-wal", dbPath + "-shm"} {
		if err := requirePrivateIfPresent(sidecar); err != nil {
			if createdDB {
				_ = os.Remove(dbPath)
			}
			return nil, err
		}
	}
	tel, err := NewTelemetryWithPath(dbPath)
	if err != nil {
		if createdDB {
			_ = os.Remove(dbPath)
		}
		return nil, err
	}
	tel.private = true
	return tel, nil
}

func ensurePrivateDirectory(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		err = os.Mkdir(path, 0700)
		if err != nil && !os.IsExist(err) {
			return fmt.Errorf("create private telemetry directory: %w", err)
		}
		info, err = os.Lstat(path)
	}
	if err != nil {
		return fmt.Errorf("inspect private telemetry directory: %w", err)
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("telemetry directory is not private")
	}
	return nil
}

// ensurePrivateFile returns true only when it created the file itself.
func ensurePrivateFile(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err == nil {
		if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
			return false, fmt.Errorf("telemetry file is not private")
		}
		return false, nil
	}
	if !os.IsNotExist(err) {
		return false, fmt.Errorf("inspect private telemetry file: %w", err)
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		if os.IsExist(err) {
			return ensurePrivateFile(path)
		}
		return false, fmt.Errorf("create private telemetry file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return false, fmt.Errorf("close private telemetry file: %w", err)
	}
	return true, nil
}

func requirePrivateIfPresent(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect private telemetry file: %w", err)
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0077 != 0 {
		return fmt.Errorf("telemetry file is not private")
	}
	return nil
}

func (t *Telemetry) init() error {
	query := `
	CREATE TABLE IF NOT EXISTS executions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		command TEXT,
		original_tokens INTEGER,
		compressed_tokens INTEGER,
		original_content TEXT,
		compressed_content TEXT,
		duration_ms INTEGER,
		parser_used TEXT,
		is_passthrough BOOLEAN,
		source TEXT DEFAULT 'unknown',
		harness TEXT DEFAULT 'unknown',
		model TEXT DEFAULT 'unknown',
		input_cost_per_million REAL,
		decision_reason TEXT NOT NULL DEFAULT 'unknown'
	);`

	_, err := t.DB.Exec(query)
	if err != nil {
		return err
	}

	_, err = t.DB.Exec(`
	CREATE TABLE IF NOT EXISTS codex_wrapper_observations (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		timestamp DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		kind TEXT NOT NULL CHECK (kind IN ('proposal', 'capture')),
		family TEXT CHECK (family IS NULL OR family IN (
			'git status', 'git diff', 'git log', 'git show', 'git branch',
			'go test', 'go build', 'go run', 'go vet',
			'npm test', 'npm run', 'npm install', 'npm ci',
			'pnpm test', 'pnpm run', 'pnpm install',
			'yarn test', 'yarn run', 'yarn install',
			'bun test', 'bun run', 'bun install',
			'python', 'pytest', 'node', 'npx', 'cargo', 'rustc',
			'make', 'cmake', 'docker', 'kubectl', 'terraform',
			'rg', 'grep', 'find', 'ls', 'cat', 'other/compound'
		)),
		capture_outcome TEXT CHECK (capture_outcome IS NULL OR capture_outcome IN (
			'complete_within_window', 'deadline_stream', 'cap_stream', 'incomplete'
		)),
		elapsed_ms INTEGER CHECK (elapsed_ms IS NULL OR elapsed_ms BETWEEN 0 AND 86400000),
		selection TEXT CHECK (selection IS NULL OR selection IN ('reduced', 'raw', 'unknown')),
		original_bytes INTEGER CHECK (original_bytes IS NULL OR original_bytes BETWEEN 0 AND 1099511627776),
		delivered_bytes INTEGER CHECK (delivered_bytes IS NULL OR delivered_bytes BETWEEN 0 AND 1099511627776),
		CHECK (
			(kind = 'proposal' AND family IS NOT NULL AND capture_outcome IS NULL AND elapsed_ms IS NULL AND selection IS NULL AND original_bytes IS NULL AND delivered_bytes IS NULL)
			OR
			(kind = 'capture' AND family IS NULL AND capture_outcome IS NOT NULL AND selection IS NOT NULL AND
			 ((original_bytes IS NULL AND delivered_bytes IS NULL) OR (original_bytes IS NOT NULL AND delivered_bytes IS NOT NULL)))
		)
	);`)
	if err != nil {
		return fmt.Errorf("create Codex wrapper observation table: %w", err)
	}
	if _, err := t.DB.Exec("CREATE INDEX IF NOT EXISTS idx_codex_wrapper_observations_timestamp ON codex_wrapper_observations(timestamp)"); err != nil {
		return fmt.Errorf("index Codex wrapper observations: %w", err)
	}

	// Add only the bounded metadata column. Legacy decisions stay unknown;
	// unlike duplicate-column errors, genuine migration failures must surface.
	var reasonColumns int
	if err := t.DB.QueryRow("SELECT COUNT(*) FROM pragma_table_info('executions') WHERE name = 'decision_reason'").Scan(&reasonColumns); err != nil {
		return fmt.Errorf("inspect decision_reason column: %w", err)
	}
	if reasonColumns == 0 {
		if _, err := t.DB.Exec("ALTER TABLE executions ADD COLUMN decision_reason TEXT NOT NULL DEFAULT 'unknown'"); err != nil {
			return fmt.Errorf("add decision_reason column: %w", err)
		}
	}

	// Migrations for columns
	_, _ = t.DB.Exec("ALTER TABLE executions ADD COLUMN original_content TEXT")
	_, _ = t.DB.Exec("ALTER TABLE executions ADD COLUMN compressed_content TEXT")
	_, _ = t.DB.Exec("ALTER TABLE executions ADD COLUMN source TEXT DEFAULT 'unknown'")
	_, _ = t.DB.Exec("ALTER TABLE executions ADD COLUMN harness TEXT DEFAULT 'unknown'")
	// Remove historical output retention when opening an existing telemetry store.
	_, _ = t.DB.Exec("UPDATE executions SET original_content = '', compressed_content = '' WHERE original_content != '' OR compressed_content != ''")
	_, _ = t.DB.Exec("ALTER TABLE executions ADD COLUMN model TEXT DEFAULT 'unknown'")
	_, _ = t.DB.Exec("ALTER TABLE executions ADD COLUMN input_cost_per_million REAL")

	// Uniqueness constraint to prevent duplication on sync
	_, _ = t.DB.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_executions_unique ON executions(timestamp, command, duration_ms)")

	// FTS5 Setup
	_, err = t.DB.Exec(`
	CREATE VIRTUAL TABLE IF NOT EXISTS executions_fts USING fts5(
		command,
		original_content,
		compressed_content,
		content='executions',
		content_rowid='id'
	);`)
	if err != nil {
		return fmt.Errorf("failed to create FTS5 table: %w", err)
	}

	// Triggers to keep FTS5 synchronized
	_, _ = t.DB.Exec(`
	CREATE TRIGGER IF NOT EXISTS executions_ai AFTER INSERT ON executions BEGIN
		INSERT INTO executions_fts(rowid, command, original_content, compressed_content)
		VALUES (new.id, new.command, new.original_content, new.compressed_content);
	END;`)

	_, _ = t.DB.Exec(`
	CREATE TRIGGER IF NOT EXISTS executions_ad AFTER DELETE ON executions BEGIN
		INSERT INTO executions_fts(executions_fts, rowid, command, original_content, compressed_content)
		VALUES ('delete', old.id, old.command, old.original_content, old.compressed_content);
	END;`)

	_, _ = t.DB.Exec(`
	CREATE TRIGGER IF NOT EXISTS executions_au AFTER UPDATE ON executions BEGIN
		INSERT INTO executions_fts(executions_fts, rowid, command, original_content, compressed_content)
		VALUES ('delete', old.id, old.command, old.original_content, old.compressed_content);
		INSERT INTO executions_fts(rowid, command, original_content, compressed_content)
		VALUES (new.id, new.command, new.original_content, new.compressed_content);
	END;`)

	// Backfill existing records into FTS index if any
	_, _ = t.DB.Exec(`
	INSERT INTO executions_fts(rowid, command, original_content, compressed_content)
	SELECT id, command, original_content, compressed_content FROM executions
	WHERE id NOT IN (SELECT rowid FROM executions_fts);`)

	return nil
}

func (t *Telemetry) Record(rec ExecutionRecord) error {
	rec.DecisionReason = NormalizeDecisionReason(rec.DecisionReason)
	rec.Command = redactCommand(rec.Command)
	// Telemetry supports accounting and parser discovery, not output retention.
	rec.OriginalContent = ""
	rec.CompressedContent = ""
	if rec.Harness != "" {
		rec.Harness = normalizeHarness(rec.Harness)
	}
	if rec.Harness == "" {
		rec.Harness = "unknown"
	}
	if rec.Source == "" {
		rec.Source = "unknown"
	}
	if strings.TrimSpace(rec.Model) == "" {
		rec.Model = "unknown"
	}
	verb := "INSERT"
	if rec.Source == SourceCodexWrapperPrivate {
		// A timestamp/command/duration collision is harmless for aggregate
		// telemetry. This is not an exactly-once guarantee.
		verb = "INSERT OR IGNORE"
	}
	columns := "command, original_tokens, compressed_tokens, original_content, compressed_content, duration_ms, parser_used, is_passthrough, source, harness, model, input_cost_per_million, decision_reason"
	values := "?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?"
	args := []interface{}{rec.Command, rec.OriginalTokens, rec.CompressedTokens, rec.OriginalContent, rec.CompressedContent, rec.DurationMs, rec.ParserUsed, rec.IsPassthrough, rec.Source, rec.Harness, rec.Model, rec.InputCostPerMillion, rec.DecisionReason}
	if !rec.Timestamp.IsZero() {
		columns = "timestamp, " + columns
		values = "?, " + values
		args = append([]interface{}{rec.Timestamp.UTC()}, args...)
	}
	_, err := t.DB.Exec(verb+" INTO executions ("+columns+") VALUES ("+values+")", args...)
	return err
}

func (t *Telemetry) GetStatsByDay(source string) ([]struct {
	Date       string
	Original   int
	Compressed int
}, error) {
	whereClause := ""
	var args []interface{}
	if source != "" && source != "all" {
		whereClause = "WHERE source = ?"
		args = append(args, source)
	}

	query := fmt.Sprintf(`
	SELECT strftime('%%Y-%%m-%%d', timestamp) as date,
	       SUM(original_tokens),
	       SUM(compressed_tokens)
	FROM executions
	%s
	GROUP BY date
	ORDER BY date ASC`, whereClause)

	rows, err := t.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []struct {
		Date       string
		Original   int
		Compressed int
	}

	for rows.Next() {
		var r struct {
			Date       string
			Original   int
			Compressed int
		}
		if err := rows.Scan(&r.Date, &r.Original, &r.Compressed); err != nil {
			return nil, err
		}
		results = append(results, r)
	}

	return results, nil
}

func (t *Telemetry) Close() error {
	return t.DB.Close()
}

type DiscoveryFamily struct {
	Family          string
	InvocationCount int
	TotalRawTokens  int
	Harness         string
}

type DiscoveryDetail struct {
	Timestamp      time.Time
	Command        string
	OriginalTokens int
	Harness        string
}

// CommandFamily returns a safe, argument-free label for discovery reporting.
// It intentionally does not attempt to parse arbitrary shell syntax.
func CommandFamily(command string) (string, bool) {
	compact := strings.Join(strings.Fields(command), " ")
	if compact == "" {
		return "", false
	}
	fields := strings.Fields(compact)
	first := strings.ToLower(filepath.Base(fields[0]))
	if first == "pith" || first == "pith.exe" || (first == "go" && len(fields) >= 3 && fields[1] == "run" && fields[2] == "main.go") {
		return "", false
	}
	if first == "go" && len(fields) >= 2 && (fields[1] == "test" || fields[1] == "install") {
		return "", false
	}
	if strings.ContainsAny(command, "\n;|") || strings.Contains(command, "&&") || first == "set" || first == "if" || first == "for" || first == "while" || strings.Contains(fields[0], "=") {
		return "shell script", true
	}
	if (first == "git" || first == "go" || first == "npm") && len(fields) >= 2 {
		return first + " " + fields[1], true
	}
	return first, true
}

// CommandPreview is used only by the explicit details view. It never emits
// newlines and caps the stored, already-redacted command metadata.
func CommandPreview(command string) string {
	const maxRunes = 160
	compact := strings.Join(strings.Fields(command), " ")
	runes := []rune(compact)
	if len(runes) <= maxRunes {
		return compact
	}
	return string(runes[:maxRunes-1]) + "…"
}

func discoveryFamily(command, source string) (string, bool) {
	if source == SourceCodexWrapperPrivate && command == "go test" {
		return "go test", true
	}
	return CommandFamily(command)
}

// GetDiscoveryFamilies aggregates safe command-family labels for passthrough
// executions. It leaves GetUnparsedCommands intact for API compatibility.
func (t *Telemetry) GetDiscoveryFamilies(source string) ([]DiscoveryFamily, error) {
	query := "SELECT command, original_tokens, COALESCE(harness, 'unknown'), COALESCE(source, 'unknown') FROM executions WHERE is_passthrough = 1"
	var args []interface{}
	if source != "" && source != "all" {
		query += " AND source = ?"
		args = append(args, source)
	}
	rows, err := t.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	families := make(map[string]*DiscoveryFamily)
	for rows.Next() {
		var command, harness, recordSource string
		var tokens int
		if err := rows.Scan(&command, &tokens, &harness, &recordSource); err != nil {
			return nil, err
		}
		family, include := discoveryFamily(command, recordSource)
		if !include {
			continue
		}
		key := family + "\x00" + harness
		if families[key] == nil {
			families[key] = &DiscoveryFamily{Family: family, Harness: harness}
		}
		families[key].InvocationCount++
		families[key].TotalRawTokens += tokens
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	results := make([]DiscoveryFamily, 0, len(families))
	for _, family := range families {
		results = append(results, *family)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].TotalRawTokens == results[j].TotalRawTokens {
			if results[i].Family == results[j].Family {
				return results[i].Harness < results[j].Harness
			}
			return results[i].Family < results[j].Family
		}
		return results[i].TotalRawTokens > results[j].TotalRawTokens
	})
	return results, nil
}

// GetDiscoveryDetails returns bounded, redacted command samples for one family.
func (t *Telemetry) GetDiscoveryDetails(family, source string, limit int) ([]DiscoveryDetail, error) {
	if limit <= 0 {
		limit = 10
	}
	query := "SELECT timestamp, command, original_tokens, COALESCE(harness, 'unknown'), COALESCE(source, 'unknown') FROM executions WHERE is_passthrough = 1"
	var args []interface{}
	if source != "" && source != "all" {
		query += " AND source = ?"
		args = append(args, source)
	}
	query += " ORDER BY timestamp DESC"
	rows, err := t.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []DiscoveryDetail
	for rows.Next() && len(results) < limit {
		var detail DiscoveryDetail
		var recordSource string
		if err := rows.Scan(&detail.Timestamp, &detail.Command, &detail.OriginalTokens, &detail.Harness, &recordSource); err != nil {
			return nil, err
		}
		candidate, include := discoveryFamily(detail.Command, recordSource)
		if include && candidate == family {
			detail.Command = CommandPreview(detail.Command)
			results = append(results, detail)
		}
	}
	return results, rows.Err()
}

func (t *Telemetry) GetUnparsedCommands(source string) ([]struct {
	Pattern         string
	InvocationCount int
	TotalRawTokens  int
	Source          string
	Harness         string
}, error) {
	whereClause := "WHERE is_passthrough = 1"
	var args []interface{}
	if source != "" && source != "all" {
		whereClause += " AND source = ?"
		args = append(args, source)
	}

	query := fmt.Sprintf(`
	SELECT command, COUNT(*), SUM(original_tokens), MAX(source), COALESCE(harness,'unknown')
	FROM executions
	%s
	GROUP BY command, harness
	ORDER BY SUM(original_tokens) DESC
	`, whereClause)

	rows, err := t.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []struct {
		Pattern         string
		InvocationCount int
		TotalRawTokens  int
		Source          string
		Harness         string
	}

	for rows.Next() {
		var r struct {
			Pattern         string
			InvocationCount int
			TotalRawTokens  int
			Source          string
			Harness         string
		}
		if err := rows.Scan(&r.Pattern, &r.InvocationCount, &r.TotalRawTokens, &r.Source, &r.Harness); err != nil {
			return nil, err
		}
		results = append(results, r)
	}

	return results, nil
}

func (t *Telemetry) GetStats(source string) (totalOriginal, totalCompressed int, err error) {
	whereClause := ""
	var args []interface{}
	if source != "" && source != "all" {
		whereClause = "WHERE source = ?"
		args = append(args, source)
	}

	query := fmt.Sprintf(`SELECT COALESCE(SUM(original_tokens), 0), COALESCE(SUM(compressed_tokens), 0) FROM executions %s`, whereClause)
	err = t.DB.QueryRow(query, args...).Scan(&totalOriginal, &totalCompressed)
	if err == sql.ErrNoRows {
		return 0, 0, nil
	}
	return
}

func (t *Telemetry) GetStatsByCommand(source string) ([]struct {
	Command    string
	Original   int
	Compressed int
	Source     string
}, error) {
	whereClause := ""
	var args []interface{}
	if source != "" && source != "all" {
		whereClause = "WHERE source = ?"
		args = append(args, source)
	}

	query := fmt.Sprintf(`SELECT command, SUM(original_tokens), SUM(compressed_tokens), MAX(source) FROM executions %s GROUP BY command ORDER BY SUM(original_tokens) DESC`, whereClause)

	rows, err := t.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []struct {
		Command    string
		Original   int
		Compressed int
		Source     string
	}

	for rows.Next() {
		var r struct {
			Command    string
			Original   int
			Compressed int
			Source     string
		}
		if err := rows.Scan(&r.Command, &r.Original, &r.Compressed, &r.Source); err != nil {
			return nil, err
		}
		results = append(results, r)
	}

	return results, nil
}

func (t *Telemetry) GetSavingsByModel() ([]ModelSavings, error) {
	query := `SELECT COALESCE(NULLIF(model, ''), 'unknown'),
		COALESCE(SUM(original_tokens), 0), COALESCE(SUM(compressed_tokens), 0),
		COALESCE(SUM(CASE WHEN input_cost_per_million IS NOT NULL
			THEN (original_tokens - compressed_tokens) * input_cost_per_million / 1000000.0 ELSE 0 END), 0),
		COALESCE(SUM(CASE WHEN input_cost_per_million IS NULL
			THEN original_tokens - compressed_tokens ELSE 0 END), 0),
		CASE WHEN COUNT(DISTINCT input_cost_per_million) = 1 THEN MIN(input_cost_per_million) END
		FROM executions GROUP BY COALESCE(NULLIF(model, ''), 'unknown')
		ORDER BY SUM(original_tokens) DESC`
	rows, err := t.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []ModelSavings
	for rows.Next() {
		var r ModelSavings
		if err := rows.Scan(&r.Model, &r.Original, &r.Compressed, &r.RecordedUSD, &r.FallbackTokens, &r.InputCostPerMillion); err != nil {
			return nil, err
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (t *Telemetry) ResetAll() error {
	tx, err := t.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM executions"); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM codex_wrapper_observations"); err != nil {
		return err
	}
	return tx.Commit()
}

func (t *Telemetry) ResetPassthrough() error {
	tx, err := t.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec("DELETE FROM executions WHERE is_passthrough = 1"); err != nil {
		return err
	}
	if _, err := tx.Exec("DELETE FROM codex_wrapper_observations"); err != nil {
		return err
	}
	return tx.Commit()
}

func (t *Telemetry) GetRecentExecutions(limit int, source string) ([]ExecutionRecord, error) {
	whereClause := ""
	var args []interface{}
	if source != "" && source != "all" {
		whereClause = "WHERE source = ?"
		args = append(args, source)
	}
	args = append(args, limit)

	query := fmt.Sprintf(`
	SELECT id, timestamp, command, original_tokens, compressed_tokens, duration_ms, COALESCE(parser_used,''), COALESCE(is_passthrough,0), COALESCE(source,'unknown'), COALESCE(harness,'unknown'), COALESCE(model,'unknown'), input_cost_per_million, COALESCE(decision_reason,'unknown')
	FROM executions
	%s
	ORDER BY timestamp DESC
	LIMIT ?`, whereClause)

	rows, err := t.DB.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []ExecutionRecord

	for rows.Next() {
		var r ExecutionRecord
		if err := rows.Scan(&r.ID, &r.Timestamp, &r.Command, &r.OriginalTokens, &r.CompressedTokens, &r.DurationMs, &r.ParserUsed, &r.IsPassthrough, &r.Source, &r.Harness, &r.Model, &r.InputCostPerMillion, &r.DecisionReason); err != nil {
			return nil, err
		}
		if r.Harness == "" {
			r.Harness = "unknown"
		}
		r.DecisionReason = NormalizeDecisionReason(r.DecisionReason)
		results = append(results, r)
	}

	return results, nil
}

func (t *Telemetry) GetHarnesses() ([]string, error) {
	query := `SELECT DISTINCT COALESCE(harness,'unknown') FROM executions WHERE COALESCE(harness,'unknown') != 'unknown' AND COALESCE(harness,'unknown') != '' ORDER BY COALESCE(harness,'unknown') ASC`
	rows, err := t.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		results = append(results, s)
	}
	return results, nil
}

func (t *Telemetry) GetStatsByHarness() ([]struct {
	Harness    string
	Original   int
	Compressed int
}, error) {
	query := `SELECT COALESCE(harness,'unknown') as harness, COALESCE(SUM(original_tokens),0), COALESCE(SUM(compressed_tokens),0) FROM executions GROUP BY harness ORDER BY SUM(original_tokens) DESC`
	rows, err := t.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var results []struct {
		Harness    string
		Original   int
		Compressed int
	}
	for rows.Next() {
		var r struct {
			Harness    string
			Original   int
			Compressed int
		}
		if err := rows.Scan(&r.Harness, &r.Original, &r.Compressed); err != nil {
			return nil, err
		}
		if r.Harness == "" {
			r.Harness = "unknown"
		}
		results = append(results, r)
	}
	return results, nil
}

func (t *Telemetry) GetSources() ([]string, error) {
	query := `SELECT DISTINCT source FROM executions WHERE source != 'unknown' AND source != '' ORDER BY source ASC`
	rows, err := t.DB.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		results = append(results, s)
	}

	return results, nil
}

func (t *Telemetry) GetExecutionDetails(id int64) (*ExecutionRecord, error) {
	query := `
	SELECT id, timestamp, command, original_tokens, compressed_tokens, COALESCE(original_content,''), COALESCE(compressed_content,''), duration_ms, COALESCE(parser_used,''), COALESCE(is_passthrough,0), COALESCE(source,'unknown'), COALESCE(harness,'unknown'), COALESCE(model,'unknown'), input_cost_per_million, COALESCE(decision_reason,'unknown')
	FROM executions
	WHERE id = ?`

	var r ExecutionRecord
	err := t.DB.QueryRow(query, id).Scan(&r.ID, &r.Timestamp, &r.Command, &r.OriginalTokens, &r.CompressedTokens, &r.OriginalContent, &r.CompressedContent, &r.DurationMs, &r.ParserUsed, &r.IsPassthrough, &r.Source, &r.Harness, &r.Model, &r.InputCostPerMillion, &r.DecisionReason)
	if err != nil {
		return nil, err
	}

	r.DecisionReason = NormalizeDecisionReason(r.DecisionReason)
	return &r, nil
}

func (t *Telemetry) SearchExecutions(queryStr string, source string, limit int) ([]ExecutionRecord, error) {
	if queryStr == "" {
		return nil, nil
	}

	ftsQuery := queryStr
	// If it doesn't contain any advanced syntax operators (*, AND, OR, MATCH), we can support suffix wildcard matching:
	if !strings.ContainsAny(queryStr, `*"`+"'") && !strings.Contains(queryStr, "AND") && !strings.Contains(queryStr, "OR") {
		terms := strings.Fields(queryStr)
		if len(terms) > 0 {
			terms[len(terms)-1] = terms[len(terms)-1] + "*"
			ftsQuery = strings.Join(terms, " ")
		}
	}

	whereClause := "WHERE executions.id IN (SELECT rowid FROM executions_fts WHERE executions_fts MATCH ?)"
	args := []interface{}{ftsQuery}

	if source != "" && source != "all" {
		whereClause += " AND source = ?"
		args = append(args, source)
	}
	args = append(args, limit)

	sqlQuery := fmt.Sprintf(`
	SELECT id, timestamp, command, original_tokens, compressed_tokens, duration_ms, COALESCE(parser_used,''), COALESCE(is_passthrough,0), COALESCE(source,'unknown'), COALESCE(harness,'unknown'), COALESCE(model,'unknown'), input_cost_per_million, COALESCE(decision_reason,'unknown')
	FROM executions
	%s
	ORDER BY timestamp DESC
	LIMIT ?`, whereClause)

	rows, err := t.DB.Query(sqlQuery, args...)
	if err != nil {
		// Fallback to traditional LIKE search if FTS syntax error
		fallbackClause := "WHERE (command LIKE ? OR original_content LIKE ? OR compressed_content LIKE ?)"
		fallbackArgs := []interface{}{"%" + queryStr + "%", "%" + queryStr + "%", "%" + queryStr + "%"}
		if source != "" && source != "all" {
			fallbackClause += " AND source = ?"
			fallbackArgs = append(fallbackArgs, source)
		}
		fallbackArgs = append(fallbackArgs, limit)

		fallbackSqlQuery := fmt.Sprintf(`
		SELECT id, timestamp, command, original_tokens, compressed_tokens, duration_ms, COALESCE(parser_used,''), COALESCE(is_passthrough,0), COALESCE(source,'unknown'), COALESCE(harness,'unknown'), COALESCE(model,'unknown'), input_cost_per_million, COALESCE(decision_reason,'unknown')
		FROM executions
		%s
		ORDER BY timestamp DESC
		LIMIT ?`, fallbackClause)

		rows, err = t.DB.Query(fallbackSqlQuery, fallbackArgs...)
		if err != nil {
			return nil, err
		}
	}
	defer rows.Close()

	var results []ExecutionRecord
	for rows.Next() {
		var r ExecutionRecord
		if err := rows.Scan(&r.ID, &r.Timestamp, &r.Command, &r.OriginalTokens, &r.CompressedTokens, &r.DurationMs, &r.ParserUsed, &r.IsPassthrough, &r.Source, &r.Harness, &r.Model, &r.InputCostPerMillion, &r.DecisionReason); err != nil {
			return nil, err
		}
		if r.Harness == "" {
			r.Harness = "unknown"
		}
		r.DecisionReason = NormalizeDecisionReason(r.DecisionReason)
		results = append(results, r)
	}

	return results, nil
}

func (t *Telemetry) ExportJSONL(w io.Writer) error {
	query := `
	SELECT id, timestamp, command, original_tokens, compressed_tokens, COALESCE(original_content,''), COALESCE(compressed_content,''), duration_ms, COALESCE(parser_used,''), COALESCE(is_passthrough,0), COALESCE(source,'unknown'), COALESCE(harness,'unknown'), COALESCE(model,'unknown'), input_cost_per_million, COALESCE(decision_reason,'unknown')
	FROM executions
	WHERE COALESCE(source, 'unknown') != ?
	ORDER BY id ASC`

	rows, err := t.DB.Query(query, SourceCodexWrapperPrivate)
	if err != nil {
		return err
	}
	defer rows.Close()

	encoder := json.NewEncoder(w)
	for rows.Next() {
		var r ExecutionRecord
		if err := rows.Scan(&r.ID, &r.Timestamp, &r.Command, &r.OriginalTokens, &r.CompressedTokens, &r.OriginalContent, &r.CompressedContent, &r.DurationMs, &r.ParserUsed, &r.IsPassthrough, &r.Source, &r.Harness, &r.Model, &r.InputCostPerMillion, &r.DecisionReason); err != nil {
			return err
		}
		r.DecisionReason = NormalizeDecisionReason(r.DecisionReason)
		r.OriginalContent, r.CompressedContent = "", ""
		if err := encoder.Encode(r); err != nil {
			return err
		}
	}

	return nil
}

func (t *Telemetry) ImportJSONL(r io.Reader) error {
	tx, err := t.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	decoder := json.NewDecoder(r)
	query := `
	INSERT OR IGNORE INTO executions (timestamp, command, original_tokens, compressed_tokens, original_content, compressed_content, duration_ms, parser_used, is_passthrough, source, harness, model, input_cost_per_million, decision_reason)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	stmt, err := tx.Prepare(query)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for {
		var imported struct {
			ExecutionRecord
			Kind string `json:"kind"`
		}
		if err := decoder.Decode(&imported); err == io.EOF {
			break
		} else if err != nil {
			return err
		}
		if imported.Kind == "proposal" || imported.Kind == "capture" {
			continue
		}
		rec := imported.ExecutionRecord

		rec.DecisionReason = NormalizeDecisionReason(rec.DecisionReason)
		rec.OriginalContent, rec.CompressedContent = "", ""
		tVal := rec.Timestamp
		if tVal.IsZero() {
			tVal = time.Now()
		}

		if rec.Harness != "" {
			rec.Harness = normalizeHarness(rec.Harness)
		} else {
			rec.Harness = "unknown"
		}
		if rec.Source == "" {
			rec.Source = "unknown"
		}
		if strings.TrimSpace(rec.Model) == "" {
			rec.Model = "unknown"
		}
		_, err = stmt.Exec(tVal, rec.Command, rec.OriginalTokens, rec.CompressedTokens, rec.OriginalContent, rec.CompressedContent, rec.DurationMs, rec.ParserUsed, rec.IsPassthrough, rec.Source, rec.Harness, rec.Model, rec.InputCostPerMillion, rec.DecisionReason)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (t *Telemetry) ExportJSONLSince(w io.Writer, sinceID int64) error {
	query := `
	SELECT id, timestamp, command, original_tokens, compressed_tokens, COALESCE(original_content,''), COALESCE(compressed_content,''), duration_ms, COALESCE(parser_used,''), COALESCE(is_passthrough,0), COALESCE(source,'unknown'), COALESCE(harness,'unknown'), COALESCE(model,'unknown'), input_cost_per_million, COALESCE(decision_reason,'unknown')
	FROM executions
	WHERE id > ? AND COALESCE(source, 'unknown') != ?
	ORDER BY id ASC`

	rows, err := t.DB.Query(query, sinceID, SourceCodexWrapperPrivate)
	if err != nil {
		return err
	}
	defer rows.Close()

	encoder := json.NewEncoder(w)
	for rows.Next() {
		var r ExecutionRecord
		if err := rows.Scan(&r.ID, &r.Timestamp, &r.Command, &r.OriginalTokens, &r.CompressedTokens, &r.OriginalContent, &r.CompressedContent, &r.DurationMs, &r.ParserUsed, &r.IsPassthrough, &r.Source, &r.Harness, &r.Model, &r.InputCostPerMillion, &r.DecisionReason); err != nil {
			return err
		}
		r.DecisionReason = NormalizeDecisionReason(r.DecisionReason)
		r.OriginalContent, r.CompressedContent = "", ""
		if err := encoder.Encode(r); err != nil {
			return err
		}
	}

	return nil
}
