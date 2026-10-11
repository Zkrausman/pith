package parser

import (
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	goVerboseRunLine  = regexp.MustCompile(`^=== RUN +([^[:space:]]+)$`)
	goVerbosePassLine = regexp.MustCompile(`^--- PASS: ([^[:space:]]+) \(([^()]*)\)$`)
	goVerboseOKLine   = regexp.MustCompile(`^ok {2,}([^[:space:]]+) ([^[:space:]]+)$`)
	goVerbosePackage  = regexp.MustCompile(`(?:\.|\./[A-Za-z0-9_./-]+|[A-Za-z0-9_./-]+)\z`)
	goVerboseDuration = regexp.MustCompile(`^(?:[0-9]+(?:\.[0-9]+)?(?:h|m|s|ms|us|µs|ns))+$`)
)

// ValidatedGoVerboseSuccess recognizes only the literal absolute-go,
// single-package profile used by the Codex adapter. It returns the existing
// tests parser's success summary only when the entire output is a complete,
// diagnostic-free verbose success. It does not parse shell commands.
func ValidatedGoVerboseSuccess(command, output string, exitCode int) (string, bool) {
	if exitCode != 0 || MayContainShellSyntax(command) {
		return "", false
	}
	argv := strings.Fields(command)
	if len(argv) != 4 || strings.Join(argv, " ") != command ||
		!filepath.IsAbs(argv[0]) || filepath.Base(argv[0]) != "go" ||
		argv[1] != "test" || argv[2] != "-v" || !goVerbosePackage.MatchString(argv[3]) ||
		strings.Contains(argv[3], "...") || strings.HasPrefix(argv[3], "-") {
		return "", false
	}

	lines := strings.Split(output, "\n")
	if len(lines) < 4 || lines[len(lines)-1] != "" {
		return "", false
	}
	lines = lines[:len(lines)-1] // Go's complete command output ends in one newline.
	runs := make(map[string]struct{})
	passes := make(map[string]struct{})
	var packageLine string
	passCount := 0
	for _, line := range lines {
		if strings.HasSuffix(line, "\r") {
			line = strings.TrimSuffix(line, "\r")
		}
		if line == "" || strings.TrimSpace(line) != line {
			return "", false
		}
		if match := goVerboseRunLine.FindStringSubmatch(line); match != nil {
			if _, exists := runs[match[1]]; exists || passCount != 0 {
				return "", false
			}
			runs[match[1]] = struct{}{}
			continue
		}
		if match := goVerbosePassLine.FindStringSubmatch(line); match != nil {
			if packageLine != "" {
				return "", false
			}
			if _, exists := runs[match[1]]; !exists {
				return "", false
			}
			if _, exists := passes[match[1]]; exists {
				return "", false
			}
			if !validGoDuration(match[2]) {
				return "", false
			}
			passes[match[1]] = struct{}{}
			passCount++
			continue
		}
		if line == "PASS" {
			if packageLine != "" || passCount == 0 || len(runs) != len(passes) {
				return "", false
			}
			packageLine = "PASS"
			continue
		}
		if match := goVerboseOKLine.FindStringSubmatch(line); match != nil {
			if packageLine != "PASS" || passCount != len(runs) {
				return "", false
			}
			if !validGoDuration(match[2]) {
				return "", false
			}
			packageLine = line
			continue
		}
		return "", false
	}
	if len(runs) == 0 || len(runs) != len(passes) || packageLine == "" || packageLine == "PASS" {
		return "", false
	}
	// Keep the established TestParser summary shape exactly.
	return "PASS\n" + packageLine, true
}

// validGoDuration rejects malformed, negative, and overflowing durations.
func validGoDuration(value string) bool {
	if !goVerboseDuration.MatchString(value) {
		return false
	}
	duration, err := time.ParseDuration(value)
	return err == nil && duration >= 0
}
