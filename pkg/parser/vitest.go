package parser

import (
	"fmt"
	"strings"
)

type VitestParser struct{}

func (v *VitestParser) Name() string { return "vitest" }

func (v *VitestParser) CanParse(cmd string, args []string) bool {
	// npx vitest run or vitest run
	isNpx := MatchCommand(cmd, "npx") && len(args) > 0 && (args[0] == "vitest" || MatchCommand(args[0], "vitest"))
	isVitest := MatchCommand(cmd, "vitest")
	return isNpx || isVitest
}

func (v *VitestParser) Parse(output string) string {
	lines := strings.Split(output, "\n")
	if vitestOutputHasFailure(lines) {
		return preserveVitestFailure(output, lines)
	}
	var result []string

	hasSummary := false

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Skip decorative lines
		if strings.Contains(trimmed, "⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯") {
			continue
		}

		// Keep summary lines
		if strings.Contains(trimmed, "Test Files") || strings.Contains(trimmed, "Tests") ||
			strings.Contains(trimmed, "Duration") || strings.Contains(trimmed, "Start at") {
			result = append(result, trimmed)
			hasSummary = true
			continue
		}

		// Keep failure headers
		if strings.HasPrefix(trimmed, "FAIL") {
			result = append(result, trimmed)
			continue
		}

		// Keep test results summary (e.g., ❯ server.test.js (7 tests | 3 failed) 118ms)
		if strings.HasPrefix(trimmed, "❯") || (strings.Contains(trimmed, "tests") && strings.Contains(trimmed, "failed")) {
			result = append(result, trimmed)
			continue
		}

		// Keep error messages
		if strings.Contains(trimmed, "Error:") || strings.Contains(trimmed, "AssertionError") || strings.Contains(trimmed, "TypeError:") {
			result = append(result, trimmed)
			continue
		}

		// If we're in a failure block (identified by FAIL), keep some context
		if len(result) > 0 && strings.HasPrefix(result[len(result)-1], "FAIL") && len(result) < 30 {
			result = append(result, trimmed)
			continue
		}

		// General fallback for short outputs or if we haven't reached a limit
		if len(result) < 20 {
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		return "No summary found in vitest output."
	}

	if len(lines) > 50 && !hasSummary {
		return strings.Join(result, "\n") + fmt.Sprintf("\n... (+ %d more lines, no summary found)", len(lines)-len(result))
	}

	return strings.Join(result, "\n")
}

// Keep failure-bearing output intact: the compact summary rules below may
// otherwise omit expected/received values and stack locations around a failure.
func vitestOutputHasFailure(lines []string) bool {
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		lower := strings.ToLower(trimmed)
		if strings.HasPrefix(trimmed, "FAIL") ||
			strings.Contains(trimmed, "AssertionError") ||
			strings.Contains(trimmed, "TypeError:") ||
			strings.Contains(trimmed, "Error:") ||
			(strings.Contains(lower, "failed") &&
				(strings.Contains(lower, "tests") || strings.Contains(lower, "test files") || strings.HasPrefix(trimmed, "❯"))) {
			return true
		}
	}
	return false
}

func preserveVitestFailure(output string, lines []string) string {
	for _, line := range lines {
		if strings.Contains(strings.TrimSpace(line), "⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯") {
			// Keep the parser's existing removal of decorative rules while
			// retaining diagnostic text and blank separators byte-for-byte.
			kept := make([]string, 0, len(lines))
			for _, candidate := range lines {
				if !strings.Contains(strings.TrimSpace(candidate), "⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯") {
					kept = append(kept, candidate)
				}
			}
			return strings.Join(kept, "\n")
		}
	}
	return output
}
