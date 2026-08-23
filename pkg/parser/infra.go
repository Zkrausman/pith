package parser

import (
	"fmt"
	"regexp"
	"strings"
)

// EnvParser (Existing)
type EnvParser struct{}

func (e *EnvParser) Name() string { return "env" }
func (e *EnvParser) CanParse(cmd string, args []string) bool {
	return MatchCommand(cmd, "env") || MatchCommand(cmd, "set") || MatchCommand(cmd, "export")
}

var skipEnvRegex = regexp.MustCompile(`(?i)(token|key|secret|password|auth|ssh_auth_sock|ls_colors|color)`)

func (e *EnvParser) Parse(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		if line == "" || !strings.Contains(line, "=") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		key := parts[0]
		if skipEnvRegex.MatchString(key) {
			continue
		}
		val := parts[1]
		if len(val) > 100 {
			val = val[:100] + "..."
		}
		result = append(result, key+"="+val)
	}
	return strings.Join(result, "\n")
}

// DockerPsParser (NEW)
type DockerPsParser struct{}

func (d *DockerPsParser) Name() string { return "docker_ps" }
func (d *DockerPsParser) CanParse(cmd string, args []string) bool {
	return MatchCommand(cmd, "docker") && len(args) > 0 && (args[0] == "ps" || args[0] == "images")
}
func (d *DockerPsParser) Parse(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Docker output is column-based. We extract the first and last few columns.
		fields := strings.Fields(trimmed)
		if len(fields) >= 2 {
			// ID + Name/Repo
			result = append(result, fmt.Sprintf("%s\t%s", fields[0], fields[len(fields)-1]))
		}
	}
	return strings.Join(result, "\n")
}

// DependencyParser (NEW)
type DependencyParser struct{}

func (d *DependencyParser) Name() string { return "dependencies" }
func (d *DependencyParser) CanParse(cmd string, args []string) bool {
	return (MatchCommand(cmd, "npm") && len(args) > 0 && args[0] == "list") ||
		(MatchCommand(cmd, "pip") && len(args) > 0 && args[0] == "list")
}
func (d *DependencyParser) Parse(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	count := 0

	// Characters to strip from the beginning of dependency lines
	replacer := strings.NewReplacer("├── ", "", "└── ", "", "│   ", "", "├─ ", "", "└─ ", "")

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.Contains(trimmed, "empty") {
			continue
		}

		cleaned := replacer.Replace(trimmed)
		if cleaned == trimmed && (strings.Contains(trimmed, "├──") || strings.Contains(trimmed, "└──")) {
			// Fallback if formatting is slightly different
			cleaned = strings.Map(func(r rune) rune {
				if r == '├' || r == '─' || r == '└' || r == '│' {
					return -1
				}
				return r
			}, trimmed)
		}

		result = append(result, strings.TrimSpace(cleaned))
		count++
		if count > 30 {
			break
		}
	}
	res := strings.Join(result, "\n")
	if len(lines) > 30 {
		res += "\n... (truncated dependency list)"
	}
	return res
}

// TestParser (NEW)
type TestParser struct{}

func (t *TestParser) Name() string { return "tests" }
func (t *TestParser) CanParse(cmd string, args []string) bool {
	// Only match if it's a test command and doesn't have build-only or list-only flags
	isTest := (MatchCommand(cmd, "npm") && len(args) > 0 && args[0] == "test") ||
		(MatchCommand(cmd, "go") && len(args) > 0 && args[0] == "test") ||
		MatchCommand(cmd, "pytest")

	if !isTest {
		return false
	}

	// Exclude go test flags that don't execute tests
	for _, arg := range args {
		if arg == "-c" || arg == "-i" || arg == "-list" {
			return false
		}
	}

	return true
}
func (t *TestParser) Parse(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	isFailureBlock := false

	// Failure keywords to trigger failure block tracking
	failKeywords := regexp.MustCompile(`(?i)(FAIL|Error:|panic|exception|traceback|at\s|\[ERROR\]|!!!)`)
	// Summary keywords to capture aggregate results (e.g., "10 passed", "TOTAL: 5", "ok  pkg/path")
	// We avoid matching individual "PASS: test_name" lines
	summaryKeywords := regexp.MustCompile(`(?i)(^\d+\s+(passed|failed)|TOTAL|DONE|^ok\s+|^PASS\s*$)`)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Capture summary lines (always keep)
		if summaryKeywords.MatchString(trimmed) && !strings.Contains(trimmed, "PASS:") {
			result = append(result, trimmed)
			isFailureBlock = false // Reset on summary
			continue
		}

		// Heuristic to detect failure starts
		if failKeywords.MatchString(line) {
			isFailureBlock = true
		}

		// Reset failure block if we see an individual PASS line
		if strings.Contains(trimmed, "PASS:") {
			isFailureBlock = false
		}

		// Within a failure block, we are more permissive
		if isFailureBlock {
			result = append(result, line)

			// Stop the failure block if we hit a known "pass" indicator or a summary marker
			// We no longer stop just at empty lines to avoid truncating multi-line errors
			if strings.HasPrefix(trimmed, "--- PASS") || strings.HasPrefix(trimmed, "ok  ") {
				isFailureBlock = false
			}
		}
	}

	if len(result) == 0 {
		return "Tests finished. (No summary captured)"
	}
	return strings.Join(result, "\n")
}

// GoToolCoverParser (NEW)
type GoToolCoverParser struct{}

func (g *GoToolCoverParser) Name() string { return "go_cover" }
func (g *GoToolCoverParser) CanParse(cmd string, args []string) bool {
	return MatchCommand(cmd, "go") && len(args) > 2 && args[0] == "tool" && args[1] == "cover"
}
func (g *GoToolCoverParser) Parse(output string) string {
	lines := strings.Split(output, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Always keep the total line
		if strings.HasPrefix(trimmed, "total:") {
			result = append(result, trimmed)
			continue
		}

		// For function breakdown, only keep lines that are NOT 100.0%
		// This highlights areas that need work.
		if strings.Contains(trimmed, "%") && !strings.Contains(trimmed, "100.0%") {
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		// If everything is 100%, just show the total if we found it, or a success message
		for _, line := range lines {
			if strings.HasPrefix(strings.TrimSpace(line), "total:") {
				return line
			}
		}
		return "Coverage: 100.0% across all functions."
	}

	// Limit to top 30 "problem" functions
	if len(result) > 30 {
		result = append(result[:30], "... (truncated coverage list)")
	}

	return strings.Join(result, "\n")
}

// GitHubParser (UPDATED)
type GitHubParser struct{}

func (g *GitHubParser) Name() string { return "github" }
func (g *GitHubParser) CanParse(cmd string, args []string) bool {
	if !MatchCommand(cmd, "gh") {
		return false
	}
	joined := strings.Join(args, " ")
	return strings.Contains(joined, " list") || 
	       strings.Contains(joined, " search") || 
	       strings.Contains(joined, " checks") || 
	       strings.Contains(joined, " status")
}
func (g *GitHubParser) Parse(output string) string {
	lines := strings.Split(output, "\n")
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip spinner/progress lines
		if strings.Contains(trimmed, "Resolving deltas") || strings.Contains(trimmed, "remote: Compressing") {
			continue
		}

		// Strictly skip actual table headers, not paragraph text
		if (strings.HasPrefix(trimmed, "Showing ") && strings.Contains(trimmed, "results")) || 
		   strings.HasPrefix(trimmed, "TITLE \t") || strings.HasPrefix(trimmed, "TITLE  ") ||
		   strings.HasPrefix(trimmed, "NAME \t") || strings.HasPrefix(trimmed, "NAME  ") {
			continue
		}

		result = append(result, line)

		if len(result) > 100 {
			result = append(result, "... (truncated gh output)")
			break
		}
	}

	return strings.Join(result, "\n")
}
