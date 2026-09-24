package parser

import (
	"regexp"
	"strings"
)

// GrepParser (NEW)
type GrepParser struct{}

func (g *GrepParser) Name() string { return "grep" }
func (g *GrepParser) CanParse(cmd string, args []string) bool {
	return cmd == "grep" || cmd == "rg" || cmd == "ripgrep"
}
func (g *GrepParser) Parse(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	currentFile := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		// Try to detect file:line:content
		parts := strings.SplitN(trimmed, ":", 3)
		// A Windows drive colon belongs to the filename, not the rg line
		// delimiter. Split the remainder so content may still contain colons.
		if len(trimmed) >= 3 && ((trimmed[0] >= 'A' && trimmed[0] <= 'Z') || (trimmed[0] >= 'a' && trimmed[0] <= 'z')) && trimmed[1] == ':' && (trimmed[2] == '\\' || trimmed[2] == '/') {
			remainder := strings.SplitN(trimmed[2:], ":", 3)
			if len(remainder) >= 2 {
				parts = append([]string{trimmed[:2] + remainder[0]}, remainder[1:]...)
			}
		}
		if len(parts) >= 2 {
			file := parts[0]
			if file != currentFile {
				result = append(result, "\n"+file+":")
				currentFile = file
			}
			if len(parts) == 3 {
				result = append(result, "  "+parts[1]+": "+parts[2])
			} else {
				result = append(result, "  "+parts[1])
			}
		} else {
			result = append(result, trimmed)
		}
	}
	return strings.TrimSpace(strings.Join(result, "\n"))
}

// MinifyParser (NEW)
type MinifyParser struct{}

func (m *MinifyParser) Name() string { return "minify" }
func (m *MinifyParser) CanParse(cmd string, args []string) bool {
	if cmd != "cat" && cmd != "type" {
		return false
	}
	if len(args) == 0 {
		return false
	}
	file := args[0]
	return strings.HasSuffix(file, ".json") || strings.HasSuffix(file, ".xml") ||
		strings.HasSuffix(file, ".html") || strings.HasSuffix(file, ".css")
}

var whitespaceRegex = regexp.MustCompile(`\s+`)

func (m *MinifyParser) Parse(output string) string {
	// Moderated minification:
	// 1. Strip comments (simple heuristic)
	// 2. Collapse extreme whitespace, but keep some structure if large
	// 3. Keep first 50 and last 50 lines to avoid model confusion (middle-out is handled by runner)

	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		// Skip typical comments for common file types
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "/*") ||
			strings.HasPrefix(trimmed, "<!--") || strings.HasPrefix(trimmed, "#") {
			continue
		}

		// Collapse excessive spaces within the line
		collapsed := whitespaceRegex.ReplaceAllString(trimmed, " ")
		result = append(result, collapsed)
	}

	// If it's still huge, we'll let the middle-out truncation handle it,
	// but we've at least removed the metadata/comment noise.
	return strings.Join(result, "\n")
}
