package pi

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"

	"pith/pkg/parser"
	"pith/pkg/telemetry"
)

// Harness identifies the coding harness/agent environment.
// Deterministic bucket: same harness+command -> same group regardless of StoragePath.
const (
	HarnessPi      = "pi"
	HarnessClaude  = "claude"
	HarnessGemini  = "gemini"
	HarnessCodex   = "codex"
	HarnessJules   = "jules"
	HarnessUnknown = "unknown"
)

// NormalizeHarness returns a canonical harness value.
// Allowed: pi | claude | gemini | codex | jules; everything else -> unknown.
// Deterministic: casing/whitespace insensitive, never uses StoragePath or machine path.
func NormalizeHarness(h string) string {
	switch strings.ToLower(strings.TrimSpace(h)) {
	case HarnessPi:
		return HarnessPi
	case HarnessClaude:
		return HarnessClaude
	case HarnessGemini:
		return HarnessGemini
	case HarnessCodex:
		return HarnessCodex
	case HarnessJules:
		return HarnessJules
	default:
		return HarnessUnknown
	}
}

// PiConfig controls PiOptimize behavior. Zero value is valid.
type PiConfig struct {
	// ThresholdBytes: compress only outputs >= this size. 0 = default 8000.
	ThresholdBytes int
	// TelemetryEnabled controls whether telemetry is recorded for Pi.
	// Must remain false by default per ZAR-110.
	TelemetryEnabled bool
	// Redact controls whether secrets are redacted from compressed output.
	Redact bool
	// RawBypass when true returns output unchanged (explicit raw escape).
	RawBypass bool
	// Harness identifies the caller harness (pi | claude | gemini | codex | jules | unknown).
	// The Pi extension must pass harness:"pi" via PiOptimizeWithConfig.
	// Pith records harness NOT StoragePath (E:\TheBrain\PithBackup vs ~/.pith differs per box).
	Harness string
	// StoragePath optional override for telemetry DB location (testing).
	// When empty, default ~/.pith (or OS home) is used. Not used for grouping.
	StoragePath string
}

func (c PiConfig) threshold() int {
	if c.ThresholdBytes <= 0 {
		return 8000
	}
	return c.ThresholdBytes
}

// errorMarkers are checked case-insensitively; if present, output is preserved lossless.
var errorMarkerRegex = regexp.MustCompile(`(?i)\[FAIL\]|FAILED|ERROR|panic|traceback|exception|fatal`)

var diffMarkerRegex = regexp.MustCompile(`(?m)^(?:diff --git|@@ |--- |\+\+\+ )`)

// upstreamTruncationRegex recognizes omission markers already present in a
// host/tool result. Such results must remain untouched: Pith did not omit it.
var upstreamTruncationRegex = regexp.MustCompile(`(?i)(output|content|results?)\s+(was\s+)?(truncated|trimmed)|truncated\s+by\s+(the\s+)?(host|tool|runner)|\.\.\.\s*output\s+truncated|\.\.\.\s*\(truncated|\[showing\s+(lines|last)\b[^\]]*full output\s*:`)
var warningMarkerRegex = regexp.MustCompile(`(?im)^\s*(warning|warn(ing)?\s*:|npm\s+warn\b|\[[^\]]*\bwarn(?:ing)?\b[^\]]*\]|\d{4}-\d\d-\d\d[^\n]*(warning|warn)|⚠)`)
var finalSummaryRegex = regexp.MustCompile(`(?im)\b(?:test files?|tests?|suites?)\b[^\n]*\b(?:passed|failed|pass|fail)\b|\b(?:passed|failed)\b[^\n]*\b(?:tests?|suites?)\b|^\s*(?:PASS|FAIL)\b`)
var gitInspectionRegex = regexp.MustCompile(`(?i)\bgit\b.*\b(status\b[^\n]*--porcelain|worktree\s+list\b[^\n]*--porcelain|rev-parse\b)`)

func mustPreserveOutput(command, output string, exitCode int) bool {
	return parser.MayContainShellSyntax(command) || exitCode != 0 || errorMarkerRegex.MatchString(output) || warningMarkerRegex.MatchString(output) || finalSummaryRegex.MatchString(output) || upstreamTruncationRegex.MatchString(output) || gitInspectionRegex.MatchString(command) || isStructuredOutput(output)
}

// secretPatterns redacts common credential shapes before persistence/compression.
var secretPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(api[_-]?key\s*["']?\s*[:=]\s*)(["']?)[^"'\s;]+(["']?)`),
	regexp.MustCompile(`(?i)(secret\s*["']?\s*[:=]\s*)(["']?)[^"'\s;]+(["']?)`),
	regexp.MustCompile(`(?i)(password\s*["']?\s*[:=]\s*)(["']?)[^\s"']+(["']?)`),
	regexp.MustCompile(`(?i)(token\s*["']?\s*[:=]\s*)(["']?)[^\s"']+(["']?)`),
	regexp.MustCompile(`(?i)Bearer\s+[A-Za-z0-9_\-\.]+`),
	regexp.MustCompile(`(?i)ghp_[A-Za-z0-9_]+`),
	regexp.MustCompile(`(?i)gho_[A-Za-z0-9_]+`),
	regexp.MustCompile(`(?i)sk-[A-Za-z0-9\-]+`),
}

var privateSourceMarkers = []string{
	"evidence/delivery/",
	".llm-wiki/raw/",
	".llm-wiki/meta/",
	"broker_data",
	"Linear evidence",
}

// PiOptimize is the deterministic, transform-only Pith API for Pi.
// It never spawns or re-runs the original command.
func PiOptimize(command, output string, exitCode int) (string, error) {
	return PiOptimizeWithConfig(command, output, exitCode, PiConfig{})
}

// PiOptimizeWithConfig is the configurable form.
func PiOptimizeWithConfig(command, output string, exitCode int, cfg PiConfig) (string, error) {
	if cfg.RawBypass {
		return output, nil
	}
	if output == "" {
		return "", nil
	}
	// Preserve omissions made upstream and exact inspection/machine-readable
	// commands; Pith must not claim or introduce loss for these results.
	preserve := mustPreserveOutput(command, output, exitCode)
	var compressed string
	if preserve {
		compressed = maybeRedact(output, cfg)
	} else if diffMarkerRegex.MatchString(output) {
		compressed = maybeRedact(output, cfg)
	} else if len(output) < cfg.threshold() {
		compressed = maybeRedact(output, cfg)
	} else {
		compressed = maybeRedact(compressLargeOutput(output, cfg.threshold()), cfg)
	}
	if cfg.TelemetryEnabled {
		harness := NormalizeHarness(cfg.Harness)
		if tel, err := telemetry.NewTelemetry(cfg.StoragePath); err == nil {
			defer tel.Close()
			// Estimate tokens: ~4 chars per token (same heuristic as runner).
			origTokens := int(float64(utf8.RuneCountInString(output)) / 4.0)
			compTokens := int(float64(utf8.RuneCountInString(compressed)) / 4.0)
			// Telemetry.Record retains only redacted command metadata and token counts.
			_ = tel.Record(telemetry.ExecutionRecord{
				Command:          command,
				OriginalTokens:   origTokens,
				CompressedTokens: compTokens,
				Source:           harness,
				Harness:          harness,
			})
		}
	}
	return compressed, nil
}

// PiRedact redacts known secret patterns from text. Exported for telemetry-safe persistence.
func PiRedact(s string) string {
	return redactSecrets(s)
}

// PiShouldRedact reports whether the output contains likely secrets or sensitive broker evidence.
func PiShouldRedact(s string) bool {
	low := strings.ToLower(s)
	for _, m := range privateSourceMarkers {
		if strings.Contains(low, strings.ToLower(m)) {
			return true
		}
	}
	redacted := redactSecrets(s)
	return redacted != s
}

func maybeRedact(s string, cfg PiConfig) string {
	if cfg.Redact {
		return redactSecrets(s)
	}
	return s
}

func redactSecrets(s string) string {
	out := s
	for i, re := range secretPatterns {
		// The first four expressions capture the key prefix and optional
		// surrounding quote characters. Keep those captures: consuming a
		// closing quote while replacing a JSON scalar otherwise produces
		// malformed JSON (for example, "token:abc").
		if i < 4 {
			out = re.ReplaceAllString(out, `${1}${2}[REDACTED]${3}`)
		} else {
			out = re.ReplaceAllString(out, "[REDACTED]")
		}
	}
	return out
}

func compressLargeOutput(output string, threshold int) string {
	lines := strings.Split(output, "\n")
	head := 60
	tail := 60
	if len(lines) > 400 {
		head = 80
		tail = 80
	}
	if len(lines) <= head+tail+1 {
		if len(output) <= threshold {
			return output
		}
		keep := threshold - 80
		if keep < 200 {
			keep = 200
		}
		runes := []rune(output)
		half := keep / 2
		if half*2 > len(runes) {
			half = len(runes) / 2
		}
		prefix, suffix := string(runes[:half]), string(runes[len(runes)-half:])
		omitted := output[len(prefix) : len(output)-len(suffix)]
		omittedLines := lineCount(output) - lineCount(prefix) - lineCount(suffix)
		if omittedLines < 0 {
			omittedLines = 0
		}
		candidate := prefix + fmt.Sprintf("\n... [%d bytes, %d lines minimized by Pith PiOptimize] ...\n", len(omitted), omittedLines) + suffix
		if len(candidate) >= len(output) {
			return output
		}
		return candidate
	}

	middleStart := head
	middleEnd := len(lines) - tail
	hotKeywords := []string{"warn", "info", "test", "ok", "pass"}
	hotSet := make(map[int]bool)
	for i := middleStart; i < middleEnd; i++ {
		low := strings.ToLower(lines[i])
		for _, k := range hotKeywords {
			if strings.Contains(low, k) {
				hotSet[i] = true
				break
			}
		}
	}
	keep := make([]bool, len(lines))
	for i := 0; i < head; i++ {
		keep[i] = true
	}
	for i := len(lines) - tail; i < len(lines); i++ {
		keep[i] = true
	}
	keptHot := 0
	for i := middleStart; i < middleEnd && keptHot < 10; i++ {
		if !hotSet[i] {
			continue
		}
		start, end := i-1, i+1
		if start < middleStart {
			start = middleStart
		}
		if end >= middleEnd {
			end = middleEnd - 1
		}
		for j := start; j <= end; j++ {
			if !keep[j] {
				keep[j] = true
			}
		}
		keptHot++
	}
	var kept, omitted []string
	for i, line := range lines {
		if keep[i] {
			kept = append(kept, line)
		} else {
			omitted = append(omitted, line)
		}
	}
	if len(omitted) == 0 {
		return output
	}
	omittedBytes := 0
	for i, line := range lines {
		if !keep[i] {
			omittedBytes += len(line)
			if i < len(lines)-1 {
				omittedBytes++ // account for the source newline, including empty lines
			}
		}
	}
	marker := fmt.Sprintf("... [%d bytes, %d lines minimized by Pith PiOptimize] ...", omittedBytes, len(omitted))
	result := append(append([]string{}, lines[:head]...), marker)
	for i := head; i < len(lines)-tail; i++ {
		if keep[i] {
			result = append(result, lines[i])
		}
	}
	result = append(result, lines[len(lines)-tail:]...)
	candidate := strings.Join(result, "\n")
	if len(candidate) >= len(output) {
		return output
	}
	return candidate
}
