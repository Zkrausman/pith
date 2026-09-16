package pi

import (
	"encoding/json"
	"math"
	"regexp"
	"strings"
	"time"

	"pith/pkg/parser"
	"pith/pkg/runner"
	"pith/pkg/telemetry"
)

// HookRequest is the JSON stdin contract for `pith pi transform`. Pith only
// transforms completed output and never executes Command.
type HookRequest struct {
	Command             string          `json:"command"`
	Output              string          `json:"output"`
	ExitCode            int             `json:"exitCode"`
	RawBypass           bool            `json:"rawBypass"`
	TelemetryEnabled    bool            `json:"telemetryEnabled"`
	StoragePath         string          `json:"storagePath"`
	Model               string          `json:"model"`
	InputCostPerMillion *float64        `json:"inputCostPerMillion"`
	EnabledParsers      map[string]bool `json:"-"`
}

type HookResponse struct {
	Output            string `json:"output"`
	Parser            string `json:"parser"`
	Passthrough       bool   `json:"passthrough"`
	OriginalLineCount int    `json:"originalLineCount"`
	RetainedLineCount int    `json:"retainedLineCount"`
	OriginalByteCount int    `json:"originalByteCount"`
	RetainedByteCount int    `json:"retainedByteCount"`
	// Omitted* describe exact source bytes/lines omitted by Pith. They remain
	// zero for parser representations and redaction, whose source mapping is
	// not knowable from arbitrary prose.
	OmittedLineCount int `json:"omittedLineCount"`
	OmittedByteCount int `json:"omittedByteCount"`
	// ParserNetReduction* are separate from exact-source omission. Known is
	// true only when the parser emitted its own minimization marker; values
	// exclude the marker and mandatory redaction.
	ParserNetReductionKnown bool   `json:"parserNetReductionKnown"`
	ParserNetLineReduction  int    `json:"parserNetLineReduction"`
	ParserNetByteReduction  int    `json:"parserNetByteReduction"`
	MinimizationStrategy    string `json:"minimizationStrategy"`
	UpstreamTruncated       bool   `json:"upstreamTruncated"`
}

func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return len(strings.Split(s, "\n"))
}

func responseMetadata(original, retained string, strategy string, upstream bool) HookResponse {
	originalLines, retainedLines := lineCount(original), lineCount(retained)
	// Redaction changes rendered byte counts, but is not Pith omission. Parser
	// prose also has no source-line correspondence, so exact source omissions
	// are intentionally unknown and represented as zero; parser net reduction
	// is populated separately below when its marker makes the mapping honest.
	omittedLines, omittedBytes := 0, 0
	return HookResponse{
		Output: retained, OriginalLineCount: originalLines, RetainedLineCount: retainedLines,
		OriginalByteCount: len(original), RetainedByteCount: len(retained),
		OmittedLineCount: omittedLines, OmittedByteCount: omittedBytes,
		MinimizationStrategy: strategy, UpstreamTruncated: upstream,
	}
}

var parserMarkerRegex = regexp.MustCompile(`\.\.\. \[\d+(?: bytes, \d+ lines| bytes| lines) minimized by Pith(?: PiOptimize)?\](?: \.\.\.)?`)

func parserReduction(original, parsed string) (int, int, bool) {
	marker := parserMarkerRegex.FindString(parsed)
	if marker == "" {
		return 0, 0, false
	}
	withoutMarker := strings.Replace(parsed, marker, "", 1)
	lines := lineCount(original) - lineCount(withoutMarker)
	bytes := len(original) - len(withoutMarker)
	if lines < 0 {
		lines = 0
	}
	if bytes < 0 {
		bytes = 0
	}
	return lines, bytes, true
}

func isStructuredOutput(output string) bool {
	trimmed := strings.TrimSpace(output)
	// json.Valid also covers scalar JSON (string, number, true/false, null),
	// not only objects and arrays. Valid machine-readable output is lossless.
	return trimmed != "" && json.Valid([]byte(trimmed))
}

// OptimizeHook uses the Pith parser registry for safe successful results.
// Raw requests, errors, and diffs are lossless except mandatory redaction.
func OptimizeHook(req HookRequest) HookResponse {
	started := time.Now()
	cfg := PiConfig{Redact: true, RawBypass: req.RawBypass, Harness: HarnessPi}
	upstream := upstreamTruncationRegex.MatchString(req.Output)
	strategy := "passthrough"
	if upstream {
		strategy = "upstream-host-truncation"
	}
	result := responseMetadata(req.Output, maybeRedact(req.Output, cfg), strategy, upstream)
	result.Passthrough = true
	if !req.RawBypass && !mustPreserveOutput(req.Command, req.Output, req.ExitCode) && !diffMarkerRegex.MatchString(req.Output) {
		parts := strings.Fields(req.Command)
		if len(parts) > 0 {
			for _, candidate := range parser.GetAllParsers() {
				enabled, configured := req.EnabledParsers[candidate.Name()]
				if (!configured || enabled) && candidate.CanParse(parts[0], parts[1:]) {
					rawParsed := candidate.Parse(req.Output)
					parsed := maybeRedact(rawParsed, cfg)
					result = responseMetadata(req.Output, parsed, "parser:"+candidate.Name(), false)
					result.Parser = candidate.Name()
					if lines, bytes, known := parserReduction(req.Output, rawParsed); known {
						result.ParserNetReductionKnown = true
						result.ParserNetLineReduction = lines
						result.ParserNetByteReduction = bytes
					}
					result.Passthrough = false
					break
				}
			}
		}
	}
	if tel, err := telemetry.NewTelemetry(req.StoragePath); err == nil {
		defer tel.Close()
		cost := req.InputCostPerMillion
		if cost != nil && (*cost < 0 || math.IsNaN(*cost) || math.IsInf(*cost, 0)) {
			cost = nil
		}
		_ = tel.Record(telemetry.ExecutionRecord{Command: req.Command, OriginalTokens: runner.EstimateTokensWithHeuristic(req.Output, 4), CompressedTokens: runner.EstimateTokensWithHeuristic(result.Output, 4), DurationMs: time.Since(started).Milliseconds(), ParserUsed: result.Parser, IsPassthrough: result.Passthrough, Source: HarnessPi, Harness: HarnessPi, Model: req.Model, InputCostPerMillion: cost})
	}
	return result
}
