package runner

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"pith/pkg/config"
	"pith/pkg/parser"
	"pith/pkg/telemetry"
	"runtime"
	"strings"
	"time"
	"unicode/utf8"
)

type Runner struct {
	cfg                 *config.Config
	telemetry           *telemetry.Telemetry
	parsers             []parser.Parser
	Source              string
	Model               string
	InputCostPerMillion *float64
}

func LookupModelCost(model string) *float64 {
	m := strings.ToLower(strings.TrimSpace(model))
	if m == "" || m == "unknown" {
		return nil
	}
	var rate float64
	switch {
	case strings.Contains(m, "flash-lite"):
		rate = 0.075
	case strings.Contains(m, "flash") || strings.Contains(m, "-mini") || strings.Contains(m, "haiku") || strings.Contains(m, "auto"):
		rate = 0.15
	case strings.Contains(m, "pro") || strings.Contains(m, "sonnet") || strings.Contains(m, "gpt-4o"):
		rate = 2.50
	case strings.Contains(m, "opus") || strings.Contains(m, "o1") || strings.Contains(m, "o3"):
		rate = 15.00
	default:
		return nil
	}
	return &rate
}

func NewRunner(cfg *config.Config, tel *telemetry.Telemetry) *Runner {
	return &Runner{
		cfg:       cfg,
		telemetry: tel,
		parsers:   parser.GetAllParsers(),
		Source:    DetectSource(),
		Model:     "unknown",
	}
}

func getWindowsShellCmd(fullCmd string) *exec.Cmd {
	trimmed := strings.TrimSpace(fullCmd)
	if strings.HasPrefix(trimmed, "cmd ") || strings.HasPrefix(trimmed, "cmd.exe ") {
		return exec.Command("cmd", "/c", strings.TrimPrefix(strings.TrimPrefix(trimmed, "cmd.exe "), "cmd "))
	}
	if strings.HasPrefix(trimmed, "powershell ") || strings.HasPrefix(trimmed, "powershell.exe ") ||
		strings.HasPrefix(trimmed, "pwsh ") || strings.HasPrefix(trimmed, "pwsh.exe ") {
		if pwshPath, err := exec.LookPath("pwsh.exe"); err == nil && pwshPath != "" {
			return exec.Command(pwshPath, "-NoProfile", "-NonInteractive", "-Command", fullCmd)
		}
		if psPath, err := exec.LookPath("powershell.exe"); err == nil && psPath != "" {
			return exec.Command(psPath, "-NoProfile", "-NonInteractive", "-Command", fullCmd)
		}
	}
	// If it uses PowerShell specific syntax like ';' statement chaining, cmdlets, or '$' variables
	if strings.Contains(fullCmd, ";") || strings.Contains(fullCmd, "$") {
		if pwshPath, err := exec.LookPath("pwsh.exe"); err == nil && pwshPath != "" {
			return exec.Command(pwshPath, "-NoProfile", "-NonInteractive", "-Command", fullCmd)
		}
		if psPath, err := exec.LookPath("powershell.exe"); err == nil && psPath != "" {
			return exec.Command(psPath, "-NoProfile", "-NonInteractive", "-Command", fullCmd)
		}
	}
	return exec.Command("cmd", "/c", fullCmd)
}

func DetectSource() string {
	if harness := strings.TrimSpace(os.Getenv("PITH_HARNESS")); harness != "" {
		return harness
	}
	if os.Getenv("ANTIGRAVITY") != "" || os.Getenv("ANTIGRAVITY_IDE") != "" || os.Getenv("GEMINI_SESSION_ID") != "" {
		return "antigravity"
	}
	if os.Getenv("GEMINI_CLI") != "" || os.Getenv("GOOGLE_API_KEY") != "" {
		return "gemini"
	}
	if os.Getenv("CLAUDE_CODE") != "" || os.Getenv("ANTHROPIC_API_KEY") != "" {
		return "claude"
	}
	// Check parent process names could be added here later
	return "unknown"
}

func (r *Runner) Run(args []string) error {
	return r.RunWithOptions(args, false)
}

func (r *Runner) LogForSnag(cmdStr string, output string, exitCode int) {
	// Diagnostic logs may contain sensitive terminal context. They are opt-in.
	if r.cfg == nil || !r.cfg.SnagLogging {
		return
	}
	logDir := r.cfg.StoragePath
	if logDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return // Silently fail if home dir can't be found
		}
		logDir = filepath.Join(home, ".pith")
	}

	if err := os.MkdirAll(logDir, 0700); err != nil {
		return
	}
	_ = os.Chmod(logDir, 0700)

	logPath := filepath.Join(logDir, "pith.log")
	maxBytes := r.cfg.SnagLogMaxBytes
	if maxBytes <= 0 {
		maxBytes = 1 << 20
	}
	if info, err := os.Stat(logPath); err == nil && info.Size() >= maxBytes {
		_ = os.Remove(logPath + ".1")
		_ = os.Rename(logPath, logPath+".1")
	}

	f, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()

	// Redact before any persistence, then bound each entry.
	cmdStr = telemetry.RedactForStorage(cmdStr)
	output = telemetry.RedactForStorage(output)
	// Truncate output for Snag to the last 50 lines to prevent log bloat
	lines := strings.Split(output, "\n")
	if len(lines) > 50 {
		output = "[... output truncated by Pith for Snag log ...]\n" + strings.Join(lines[len(lines)-50:], "\n")
	}

	// Format: [CMD] command\n<output>\n[EXIT] code\n
	entry := fmt.Sprintf("[CMD] %s\n", cmdStr)
	if output != "" {
		// Ensure output ends with a newline so [EXIT] is on its own line
		if !strings.HasSuffix(output, "\n") {
			output += "\n"
		}
		entry += output
	}
	entry += fmt.Sprintf("[EXIT] %d\n", exitCode)

	_, _ = f.WriteString(entry)
}

// selectParser never applies a single-command parser to possible aggregate
// output. The runner's separate middle-out truncation policy is unchanged.
func (r *Runner) selectParser(command string) parser.Parser {
	if parser.MayContainShellSyntax(command) {
		return nil
	}
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil
	}
	for _, candidate := range r.parsers {
		enabled, configured := r.cfg.EnabledParsers[candidate.Name()]
		if (!configured || enabled) && candidate.CanParse(parts[0], parts[1:]) {
			return candidate
		}
	}
	return nil
}

func (r *Runner) RunWithOptions(args []string, skipParsing bool) error {
	if len(args) == 0 {
		return fmt.Errorf("no command provided")
	}

	fullCmd := strings.Join(args, " ")
	start := time.Now()

	var cmd *exec.Cmd
	// If it's a composite command or has shell redirects/operators, run through shell
	if strings.ContainsAny(fullCmd, ";|&><$") {
		if runtime.GOOS == "windows" {
			cmd = getWindowsShellCmd(fullCmd)
		} else {
			cmd = exec.Command("sh", "-c", fullCmd)
		}
	} else {
		// IMPORTANT: If 'args' has more than one element, they are the arguments.
		// If 'args' has only one element but it contains spaces, it's a combined string
		// that needs splitting (often happens when called via proxy).
		var bin string
		var binArgs []string
		if len(args) == 1 && strings.Contains(args[0], " ") {
			parts := strings.Fields(args[0])
			if len(parts) == 0 {
				return fmt.Errorf("no command provided")
			}
			bin = parts[0]
			binArgs = parts[1:]
		} else {
			bin = args[0]
			binArgs = args[1:]
		}

		if runtime.GOOS == "windows" {
			if _, err := exec.LookPath(bin); err != nil {
				cmd = getWindowsShellCmd(fullCmd)
			} else {
				cmd = exec.Command(bin, binArgs...)
			}
		} else {
			cmd = exec.Command(bin, binArgs...)
		}
	}

	var out bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	cmd.Stdin = os.Stdin

	err := cmd.Run()
	duration := time.Since(start).Milliseconds()

	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = 1 // Generic failure
		}
	}

	stdoutStr := out.String()
	stderrStr := stderr.String()
	fullOutput := stdoutStr + stderrStr

	// Log for Snag before doing any compression/truncation
	r.LogForSnag(fullCmd, fullOutput, exitCode)

	originalTokens := r.EstimateTokens(fullOutput)

	var p parser.Parser
	if !skipParsing {
		p = r.selectParser(fullCmd)
	}

	var finalOutput string
	parserUsed := "none"
	isPassthrough := true

	if p != nil {
		// Pass fullOutput to parser instead of just stdout
		finalOutput = p.Parse(fullOutput)
		parserUsed = p.Name()
		isPassthrough = false
	} else {
		finalOutput = fullOutput
	}

	// Apply Middle-Out Truncation
	finalOutput = r.ApplyMiddleOutTruncation(finalOutput)

	compressedTokens := r.EstimateTokens(finalOutput)

	// Output to stdout
	fmt.Print(finalOutput)

	modelName := r.Model
	if modelName == "" || modelName == "unknown" {
		if r.Source == "antigravity" {
			modelName = "gemini-auto"
		} else {
			modelName = "unknown"
		}
	} else if modelName == "auto" && r.Source == "antigravity" {
		modelName = "gemini-auto"
	}

	cost := r.InputCostPerMillion
	if cost == nil && modelName != "" {
		cost = LookupModelCost(modelName)
	}

	// Record telemetry
	record := telemetry.ExecutionRecord{
		Command:             strings.Join(args, " "),
		OriginalTokens:      originalTokens,
		CompressedTokens:    compressedTokens,
		DurationMs:          duration,
		ParserUsed:          parserUsed,
		IsPassthrough:       isPassthrough,
		Source:              r.Source,
		Harness:             r.Source,
		Model:               modelName,
		InputCostPerMillion: cost,
	}

	_ = r.telemetry.Record(record)

	return err
}

func (r *Runner) ApplyMiddleOutTruncation(output string) string {
	lines := strings.Split(output, "\n")
	if len(lines) <= r.cfg.MaxLines {
		return output
	}
	head := r.cfg.HeadLines
	tail := r.cfg.TailLines
	if head+tail >= len(lines) {
		return output
	}
	middleStart := head
	middleEnd := len(lines) - tail
	hotZones := []int{}
	keywords := []string{"error", "fail", "fatal", "panic", "exception", "!!!", "error:", "[exit]"}
	for i := middleStart; i < middleEnd; i++ {
		lowerLine := strings.ToLower(lines[i])
		for _, k := range keywords {
			if strings.Contains(lowerLine, k) {
				hotZones = append(hotZones, i)
				break
			}
		}
	}
	resultLines := append([]string{}, lines[:head]...)
	lastIndex := head
	for _, zone := range hotZones {
		if zone > lastIndex+2 {
			resultLines = append(resultLines, fmt.Sprintf("\n... [%d lines of non-critical output removed by Pith] ...\n", zone-lastIndex-1))
		}
		start := zone - 1
		if start < lastIndex {
			start = lastIndex
		}
		end := zone + 1
		if end >= middleEnd {
			end = middleEnd - 1
		}
		for j := start; j <= end; j++ {
			if j >= lastIndex {
				resultLines = append(resultLines, lines[j])
				lastIndex = j + 1
			}
		}
	}
	if middleEnd > lastIndex {
		resultLines = append(resultLines, fmt.Sprintf("\n... [%d lines removed by Pith middle-out truncation] ...\n", middleEnd-lastIndex))
	}
	resultLines = append(resultLines, lines[len(lines)-tail:]...)
	return strings.Join(resultLines, "\n")
}

func (r *Runner) EstimateTokens(s string) int {
	return EstimateTokensWithHeuristic(s, r.cfg.TokenHeuristic)
}

func EstimateTokensWithHeuristic(s string, heuristic float64) int {
	if heuristic <= 0 {
		heuristic = 4.0
	}
	return int(float64(utf8.RuneCountInString(s)) / heuristic)
}

// Deprecated: Use Runner.EstimateTokens or EstimateTokensWithHeuristic
func EstimateTokens(s string) int {
	return EstimateTokensWithHeuristic(s, 4.0)
}
