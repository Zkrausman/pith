package pi

import (
	"encoding/json"
	"math"
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
	Output      string `json:"output"`
	Parser      string `json:"parser"`
	Passthrough bool   `json:"passthrough"`
}

func isStructuredOutput(output string) bool {
	trimmed := strings.TrimSpace(output)
	if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
		return false
	}
	return json.Valid([]byte(trimmed))
}

// OptimizeHook uses the Pith parser registry for safe successful results.
// Raw requests, errors, and diffs are lossless except mandatory redaction.
func OptimizeHook(req HookRequest) HookResponse {
	started := time.Now()
	cfg := PiConfig{Redact: true, RawBypass: req.RawBypass, Harness: HarnessPi}
	result := HookResponse{Output: maybeRedact(req.Output, cfg), Passthrough: true}
	if !req.RawBypass && req.ExitCode == 0 && !diffMarkerRegex.MatchString(req.Output) && !isStructuredOutput(req.Output) {
		parts := strings.Fields(req.Command)
		if len(parts) > 0 {
			for _, candidate := range parser.GetAllParsers() {
				enabled, configured := req.EnabledParsers[candidate.Name()]
				if (!configured || enabled) && candidate.CanParse(parts[0], parts[1:]) {
					result = HookResponse{Output: maybeRedact(candidate.Parse(req.Output), cfg), Parser: candidate.Name(), Passthrough: false}
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
