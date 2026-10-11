package pi

import (
	"encoding/json"
	"math"
	"path"
	"regexp"
	"strings"
	"time"

	"pith/pkg/parser"
	"pith/pkg/runner"
	"pith/pkg/telemetry"
)

// HookRequest is the JSON stdin contract for `pith pi transform`. Pith only
// transforms completed output and never executes Command. The CLI defaults omitted
// telemetryEnabled to true; Go callers must set TelemetryEnabled to opt in.
type HookRequest struct {
	Command string `json:"command"`
	// Argv is present only for a caller that already executed a validated
	// literal command directly. It avoids reparsing shell quoting for parser
	// dispatch; legacy command-only callers keep their original safeguards.
	Argv                []string        `json:"argv,omitempty"`
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
	reason := telemetry.DecisionUnknown
	commandName, args, argvOK := directParserArgs(req.Argv)
	legacyCommand := len(req.Argv) == 0
	if legacyCommand {
		parts := strings.Fields(req.Command)
		if len(parts) > 0 {
			commandName, args, argvOK = parts[0], parts[1:], true
		}
	}
	// The summary parser predates the direct-argv contract. Keep its legacy
	// command-only behavior, but derive its direct-mode command solely from the
	// validated argv profile and apply the interpreter/raw policy first.
	if argvOK {
		summaryCommand, summaryEligible := req.Command, true
		if len(req.Argv) > 0 {
			summaryCommand, summaryEligible = directGoVerboseSuccessCommand(req.Argv)
			if directCodexArgvMustStayRaw(commandName, args) {
				summaryEligible = false
			}
		}
		validatedGoSummary, validatedGo := "", false
		if summaryEligible {
			validatedGoSummary, validatedGo = (&parser.TestParser{}).ParseGoVerboseSuccess(summaryCommand, req.Output, req.ExitCode)
		}
		testsEnabled, testsConfigured := req.EnabledParsers["tests"]
		if validatedGo && !req.RawBypass && (!testsConfigured || testsEnabled) {
			parsed := maybeRedact(validatedGoSummary, cfg)
			if parsed == validatedGoSummary && len(parsed) < len(req.Output) {
				result = responseMetadata(req.Output, parsed, "parser:tests", false)
				result.Parser = "tests"
				result.Passthrough = false
				reason = telemetry.DecisionTransformed
			}
		}
	}
	directJSONRead := len(req.Argv) > 0 && argvOK && directCodexJSONRead(commandName, args)
	if result.Passthrough && !req.RawBypass &&
		!(len(req.Argv) > 0 && !argvOK) &&
		!mustPreserveOutputWithArgv(req.Command, req.Output, req.ExitCode, len(req.Argv) > 0 && argvOK, directJSONRead) &&
		!diffMarkerRegex.MatchString(req.Output) {
		if argvOK && commandName != "" {
			if (len(req.Argv) > 0 && directCodexArgvMustStayRaw(commandName, args)) ||
				directCodexMustStayRaw(commandName, args, req.Output) {
				reason = telemetry.DecisionProtectedPassthrough
			} else {
				allowedName := ""
				if directJSONRead {
					allowedName = "minify"
				}
				reason = telemetry.DecisionUnsupportedParser
				for _, candidate := range parser.GetAllParsers() {
					if len(req.Argv) > 0 && candidate.Name() == "get-content" {
						// GetContentParser has a legacy command-token grammar;
						// direct argv dispatch uses only its explicit boundary map.
						continue
					}
					if allowedName != "" && candidate.Name() != allowedName {
						continue
					}
					enabled, configured := req.EnabledParsers[candidate.Name()]
					if (!configured || enabled) && candidate.CanParse(commandName, args) {
						rawParsed := candidate.Parse(req.Output)
						if len(req.Argv) > 0 && (rawParsed == req.Output || len(rawParsed) >= len(req.Output)) {
							reason = telemetry.DecisionRejectedNonReduction
							break
						}
						// Conservative fallbacks retain passthrough provenance, before
						// mandatory redaction changes any rendered bytes.
						if rawParsed == req.Output && (candidate.Name() == "git_log" || candidate.Name() == "git_show" ||
							candidate.Name() == "git_status" || candidate.Name() == "tests" ||
							candidate.Name() == "go_cover" || candidate.Name() == "source" ||
							candidate.Name() == "ls" || candidate.Name() == "get-content") {
							reason = telemetry.DecisionUnsupportedParser
							break
						}
						parsed := maybeRedact(rawParsed, cfg)
						// Grep grouping is useful only when it shrinks the redacted
						// representation. On rejection retain passthrough provenance.
						if candidate.Name() == "grep" && (len(parsed) > len(result.Output) ||
							runner.EstimateTokensWithHeuristic(parsed, 4) >= runner.EstimateTokensWithHeuristic(result.Output, 4)) {
							reason = telemetry.DecisionRejectedNonReduction
							break
						}
						result = responseMetadata(req.Output, parsed, "parser:"+candidate.Name(), false)
						result.Parser = candidate.Name()
						if lines, bytes, known := parserReduction(req.Output, rawParsed); known {
							result.ParserNetReductionKnown = true
							result.ParserNetLineReduction = lines
							result.ParserNetByteReduction = bytes
						}
						result.Passthrough = false
						reason = telemetry.DecisionTransformed
						break
					}
				}
			}
		}
	} else if result.Passthrough {
		reason = telemetry.DecisionProtectedPassthrough
	}
	if !req.TelemetryEnabled {
		return result
	}
	if tel, err := telemetry.NewTelemetry(req.StoragePath); err == nil {
		defer tel.Close()
		cost := req.InputCostPerMillion
		if cost != nil && (*cost < 0 || math.IsNaN(*cost) || math.IsInf(*cost, 0)) {
			cost = nil
		}
		_ = tel.Record(telemetry.ExecutionRecord{DecisionReason: reason, Command: req.Command, OriginalTokens: runner.EstimateTokensWithHeuristic(req.Output, 4), CompressedTokens: runner.EstimateTokensWithHeuristic(result.Output, 4), DurationMs: time.Since(started).Milliseconds(), ParserUsed: result.Parser, IsPassthrough: result.Passthrough, Source: HarnessPi, Harness: HarnessPi, Model: req.Model, InputCostPerMillion: cost})
	}
	return result
}

func directParserArgs(argv []string) (string, []string, bool) {
	if len(argv) == 0 || len(argv) > 256 {
		return "", nil, false
	}
	totalBytes := 0
	for i, arg := range argv {
		if (i == 0 && arg == "") || strings.ContainsRune(arg, '\x00') {
			return "", nil, false
		}
		totalBytes += len(arg)
		if totalBytes > 64<<10 {
			return "", nil, false
		}
	}
	name := strings.TrimSpace(path.Base(strings.ReplaceAll(argv[0], `\`, "/")))
	if name == "." || name == "/" || name == "" {
		return "", nil, false
	}
	return name, argv[1:], true
}

func directGoVerboseSuccessCommand(argv []string) (string, bool) {
	if len(argv) != 4 || !path.IsAbs(argv[0]) ||
		path.Base(strings.ReplaceAll(argv[0], `\`, "/")) != "go" ||
		argv[1] != "test" || argv[2] != "-v" {
		return "", false
	}
	return strings.Join(argv, " "), true
}

// These commands have known lossy parser overlaps. Return raw before registry
// dispatch so an ineligible parser cannot fall through to another parser.
func directCodexMustStayRaw(command string, args []string, output string) bool {
	switch strings.ToLower(command) {
	case "ls", "dir":
		return true // filenames containing spaces or newlines are ambiguous
	case "cat", "type":
		return !directCodexJSONRead(command, args) // Source/GetContent would be lossy
	case "git":
		if parser.GitSubcommand(args) != "status" {
			return false
		}
		if strings.Contains(output, "->") {
			return true
		}
		// Porcelain -z encodes rename paths as separate NUL-delimited fields,
		// so its status byte is the only reliable cue that the parser would lose.
		for _, arg := range args {
			if arg == "-z" || arg == "--porcelain" || strings.HasPrefix(arg, "--porcelain=") {
				return strings.HasPrefix(output, "2 ") || strings.Contains(output, "\n2 ") ||
					strings.Contains(output, "R ") || strings.Contains(output, " C") || strings.Contains(output, "C ")
			}
		}
		return false
	default:
		return false
	}
}

// The adapter excludes interpreters and recursive wrappers before replacing a
// shell command. Keep the transform endpoint safe on its own as well, so a
// direct-argv request cannot route those commands through a lossy registry
// parser if it bypasses the adapter's admission check.
func directCodexArgvMustStayRaw(command string, args []string) bool {
	name := strings.TrimSuffix(strings.ToLower(command), ".exe")
	switch name {
	case "ash", "awk", "bash", "cmd", "clisp", "clojure", "csi", "dash", "deno",
		"dotnet", "elixir", "erl", "escript", "fish", "ghci", "groovy", "iex", "java",
		"js", "julia", "ksh", "lein", "lua", "node", "nodejs", "ocaml", "ocamlrun",
		"perl", "php", "powershell", "pwsh", "pypy", "python", "python2", "python3", "ruby",
		"r", "rscript", "runghc", "sbcl", "scala", "scalac", "scheme", "sh", "swift",
		"swiftc", "tclsh", "tcsh", "zsh", "pith", "env", "command", "exec", "nohup",
		"nice", "setsid", "sudo", "doas", "timeout", "xargs", "busybox":
		return true
	}
	for _, prefix := range []string{"python", "pypy", "node", "ruby", "php", "perl"} {
		if codexVersionedCommand(name, prefix) {
			return true
		}
	}
	if name == "go" && len(args) > 0 && args[0] == "run" {
		return true
	}
	if (name == "npm" || name == "npx" || name == "pnpm" || name == "yarn" || name == "bun") &&
		containsPithExecutable(args) {
		return true
	}
	if name == "find" && containsFindExec(args) && containsPithExecutable(args) {
		return true
	}
	return false
}

func codexVersionedCommand(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	suffix := strings.TrimPrefix(name, prefix)
	if suffix == "" || suffix[0] < '0' || suffix[0] > '9' || suffix[len(suffix)-1] == '.' {
		return false
	}
	for _, r := range suffix {
		if (r < '0' || r > '9') && r != '.' {
			return false
		}
	}
	return true
}

func containsPithExecutable(args []string) bool {
	for _, arg := range args {
		name := strings.ToLower(path.Base(strings.ReplaceAll(arg, `\`, "/")))
		if name == "pith" || name == "pith.exe" {
			return true
		}
	}
	return false
}

func containsFindExec(args []string) bool {
	for _, arg := range args {
		if arg == "-exec" || arg == "-execdir" || arg == "-ok" || arg == "-okdir" {
			return true
		}
	}
	return false
}

func directCodexJSONRead(command string, args []string) bool {
	if (strings.ToLower(command) != "cat" && strings.ToLower(command) != "type") || len(args) != 1 {
		return false
	}
	return strings.EqualFold(path.Ext(args[0]), ".json")
}
