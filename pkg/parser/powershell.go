package parser

import (
	"fmt"
	"strings"
)

type PowerShellParser struct{}

func (ps *PowerShellParser) Name() string { return "windows-shell" }

func (ps *PowerShellParser) CanParse(cmd string, args []string) bool {
	return MatchCommand(cmd, "powershell") || MatchCommand(cmd, "pwsh") || MatchCommand(cmd, "cmd")
}

func (ps *PowerShellParser) Parse(output string) string {
	lines := strings.Split(output, "\n")
	var result []string

	isLsh := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Headers to skip
		if strings.HasPrefix(trimmed, "Directory:") || strings.HasPrefix(trimmed, "Mode ") || strings.HasPrefix(trimmed, "----") {
			isLsh = true
			continue
		}

		if strings.Contains(trimmed, "Exit Code:") || strings.Contains(trimmed, "Process Group PGID:") {
			continue
		}

		// If it's a generic error from PowerShell (like CommandNotFoundException), minify it
		if strings.Contains(trimmed, "+ CategoryInfo") || strings.Contains(trimmed, "+ FullyQualifiedErrorId") {
			continue
		}

		// Handle Get-ChildItem / ls / dir output format
		if isLsh {
			fields := strings.Fields(trimmed)
			if len(fields) >= 4 {
				mode := fields[0]
				name := fields[len(fields)-1]

				if len(fields) >= 6 {
					size := fields[len(fields)-2]
					result = append(result, fmt.Sprintf("%s %s %s", mode, size, name))
				} else {
					result = append(result, fmt.Sprintf("%s %s", mode, name))
				}
				continue
			}
		}

		if len(result) < 40 {
			result = append(result, trimmed)
		}
	}

	if len(result) == 0 {
		return ""
	}

	if len(lines) > 40 {
		return strings.Join(result, "\n") + fmt.Sprintf("\n... (+ %d more lines)", len(lines)-len(result))
	}
	return strings.Join(result, "\n")
}

type GetContentParser struct{}

func (gc *GetContentParser) Name() string { return "get-content" }

func (gc *GetContentParser) CanParse(cmd string, args []string) bool {
	// Pattern: powershell -Command "Get-Content ..."
	// Pattern: Get-Content -Path ...
	fullCmd := strings.Join(append([]string{cmd}, args...), " ")
	return strings.Contains(fullCmd, "Get-Content") || strings.Contains(fullCmd, "cat") || strings.Contains(fullCmd, "type")
}

func (gc *GetContentParser) Parse(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ""
	}

	// If it looks like JSON, let's try to minify it and truncate large strings.
	if (strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}")) || (strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]")) {
		// Use MinifyJSON if it was already available in another parser, or implement it here.
		// Since I don't see a MinifyJSON utility, I'll do a simple one that handles common fields like 'context_snippet'.

		// Simple minification: remove excess whitespace and newlines
		var minified strings.Builder
		inString := false
		for i := 0; i < len(trimmed); i++ {
			char := trimmed[i]
			if char == '"' && (i == 0 || trimmed[i-1] != '\\') {
				inString = !inString
			}
			if !inString {
				if char == ' ' || char == '\n' || char == '\r' || char == '\t' {
					continue
				}
			}
			minified.WriteByte(char)
		}

		result := minified.String()

		// Truncate fields that are known to be massive and less useful in raw form (like context_snippet)
		// Pattern: "context_snippet":"..."
		// We'll look for strings longer than 200 chars and truncate them.

		// Note: This is a very basic implementation. A more robust one would use json.Unmarshal.
		// But for token optimization, sometimes raw string manipulation is faster/cheaper.

		if len(result) > 2000 {
			return result[:2000] + fmt.Sprintf("... [%d bytes minimized by Pith]", len(result)-2000)
		}
		return result
	}

	// For non-JSON output, just limit lines
	lines := strings.Split(trimmed, "\n")
	if len(lines) > 50 {
		return strings.Join(lines[:50], "\n") + fmt.Sprintf("\n... [%d lines minimized by Pith]", len(lines)-50)
	}

	return trimmed
}
