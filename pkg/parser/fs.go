package parser

import (
	"fmt"
	"regexp"
	"strings"
)

var lsLongMode = regexp.MustCompile(`^[dl-][rwxstST-]{9}[+@.]?$`)
var dirLongMode = regexp.MustCompile(`^[d-][arwhs-]{4,6}$`)
var lsLongName = regexp.MustCompile(`^\S+(?:\s+\S+){7}\s+(.+)$`)
var dirLongName = regexp.MustCompile(`^\S+(?:\s+\S+){3}\s+(.+)$`)

// LsParser (Existing)
type LsParser struct{}

func (l *LsParser) Name() string { return "ls" }
func (l *LsParser) CanParse(cmd string, args []string) bool {
	return MatchCommand(cmd, "ls") || MatchCommand(cmd, "dir")
}
func (l *LsParser) Parse(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "total ") || strings.HasPrefix(trimmed, "Directory:") || strings.HasPrefix(trimmed, "Mode") || strings.HasPrefix(trimmed, "----") {
			continue
		}

		fields := strings.Fields(trimmed)
		// Only recognized long listings have column metadata. Ordinary ls -1
		// rows are whole filenames, even when they contain several words.
		if len(fields) >= 9 && lsLongMode.MatchString(fields[0]) {
			// mode links user group size month day time filename...
			result = append(result, fmt.Sprintf("%s %s", fields[4], lsLongName.FindStringSubmatch(trimmed)[1]))
		} else if len(fields) >= 5 && dirLongMode.MatchString(fields[0]) {
			// PowerShell Mode Date Time Size filename...
			result = append(result, fmt.Sprintf("%s %s", fields[3], dirLongName.FindStringSubmatch(trimmed)[1]))
		} else {
			result = append(result, trimmed)
		}
	}

	// If it's a long list, return as lines, otherwise join as space-separated
	if len(result) > 5 {
		return strings.Join(result, "\n")
	}
	return strings.Join(result, " ")
}

// FindParser (NEW)
type FindParser struct{}

func (f *FindParser) Name() string { return "find" }
func (f *FindParser) CanParse(cmd string, args []string) bool {
	return MatchCommand(cmd, "find") || MatchCommand(cmd, "where")
}
func (f *FindParser) Parse(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	count := 0
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		count++
		if count > 50 {
			continue
		} // Limit to 50 results
		result = append(result, trimmed)
	}
	res := strings.Join(result, "\n")
	if count > 50 {
		res += fmt.Sprintf("\n... (+ %d more results)", count-50)
	}
	return res
}

// TreeParser (NEW)
type TreeParser struct{}

func (t *TreeParser) Name() string { return "tree" }
func (t *TreeParser) CanParse(cmd string, args []string) bool {
	return MatchCommand(cmd, "tree")
}
func (t *TreeParser) Parse(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		// Replace ASCII tree characters with simple spaces
		r := strings.NewReplacer("│", " ", "├", " ", "└", " ", "─", " ", "──", " ", "   ", "  ")
		cleaned := r.Replace(line)
		result = append(result, cleaned)
	}
	return strings.Join(result, "\n")
}

// DuParser (NEW)
type DuParser struct{}

func (d *DuParser) Name() string { return "du" }
func (d *DuParser) CanParse(cmd string, args []string) bool {
	return MatchCommand(cmd, "du")
}
func (d *DuParser) Parse(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 2 {
			size := fields[0]
			path := fields[1]
			// Only show the size and the path
			result = append(result, fmt.Sprintf("%s\t%s", size, path))
		}
	}
	// If too many lines, show only top 20
	if len(result) > 20 {
		summary := result[:20]
		return strings.Join(summary, "\n") + fmt.Sprintf("\n... (+ %d more)", len(result)-20)
	}
	return strings.Join(result, "\n")
}
