package parser

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var ansiRegex = regexp.MustCompile(`\x1b\[[0-9;]*[a-zA-Z]`)

func getGitSubcommand(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") {
			return arg
		}
		// These global flags consume the next argument
		if arg == "-C" || arg == "-c" || arg == "--git-dir" || arg == "--work-tree" || arg == "--namespace" || arg == "--super-prefix" || arg == "--config-env" || arg == "--attr-source" {
			i++
		}
	}
	return ""
}

// GitStatusParser (Existing)
type GitStatusParser struct{}

func (g *GitStatusParser) Name() string { return "git_status" }
func (g *GitStatusParser) CanParse(cmd string, args []string) bool {
	if cmd != "git" || len(args) == 0 {
		return false
	}
	sub := getGitSubcommand(args)
	return sub == "status" || sub == "add" || sub == "commit" || sub == "push"
}
func (g *GitStatusParser) Parse(output string) string {
	if hasStructuredLine(output) {
		return output
	}
	recognizedContext := false
	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "(use ") ||
			strings.HasPrefix(trimmed, "On branch") || strings.HasPrefix(trimmed, "Your branch") ||
			strings.HasPrefix(trimmed, "nothing to commit") || strings.HasPrefix(trimmed, "no changes added") ||
			strings.HasPrefix(trimmed, "Changes not staged") || strings.HasPrefix(trimmed, "Changes to be committed") ||
			strings.HasPrefix(trimmed, "Untracked files") ||
			strings.HasPrefix(trimmed, "Everything up-to-date") ||
			strings.HasPrefix(trimmed, "To ") {
			recognizedContext = true
			continue
		}
		if strings.Contains(trimmed, "->") { // push output only with human context
			continue
		}
		result = append(result, trimmed)
	}

	// Empty or unsupported capture cannot establish command success. Keep
	// machine-readable rows and their whitespace intact without Git context.
	if len(result) == 0 || !recognizedContext {
		return output
	}

	if len(result) > 20 {
		return strings.Join(result[:10], "\n") + "\n...\n" + strings.Join(result[len(result)-10:], "\n")
	}
	return strings.Join(result, "\n")
}

// GitLogParser compresses only complete, undecorated default-format records
// with a single message line. Any unrecognized content preserves the entire output.
type GitLogParser struct{}

var gitLogCommitRegex = regexp.MustCompile(`^commit ([0-9a-f]{40}|[0-9a-f]{64})$`)
var gitLogAuthorRegex = regexp.MustCompile(`^Author: ([^<>]+) <[^<>]+>$`)

func (g *GitLogParser) Name() string { return "git_log" }
func (g *GitLogParser) CanParse(cmd string, args []string) bool {
	return cmd == "git" && getGitSubcommand(args) == "log"
}
func (g *GitLogParser) Parse(output string) string {
	lines := strings.Split(strings.TrimSuffix(output, "\n"), "\n")
	var result []string
	for i := 0; i < len(lines); {
		if len(lines)-i < 5 {
			return output
		}
		commit := gitLogCommitRegex.FindStringSubmatch(lines[i])
		author := gitLogAuthorRegex.FindStringSubmatch(lines[i+1])
		if commit == nil || author == nil || !strings.HasPrefix(lines[i+2], "Date:   ") ||
			lines[i+3] != "" || !strings.HasPrefix(lines[i+4], "    ") {
			return output
		}
		date, err := time.Parse("Mon Jan _2 15:04:05 2006 -0700", strings.TrimPrefix(lines[i+2], "Date:   "))
		subject := strings.TrimPrefix(lines[i+4], "    ")
		if err != nil || subject == "" || strings.TrimSpace(subject) != subject {
			return output
		}
		result = append(result, formatCommit(commit[1][:7], author[1], date.Format("Jan 2 2006"), subject))
		i += 5
		if i < len(lines) {
			// Exactly one blank line separates records; no prefix/suffix or
			// extra message lines may be silently discarded.
			if lines[i] != "" || i+1 == len(lines) {
				return output
			}
			i++
		}
	}
	return strings.Join(result, "\n")
}

func formatCommit(h, a, d, s string) string { return h + " | " + a + " | " + d + " | " + s }

// GitDiffParser (NEW)
type GitDiffParser struct{}

func (g *GitDiffParser) Name() string { return "git_diff" }
func (g *GitDiffParser) CanParse(cmd string, args []string) bool {
	return cmd == "git" && getGitSubcommand(args) == "diff"
}
func (g *GitDiffParser) Parse(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		cleanLine := ansiRegex.ReplaceAllString(line, "")
		if strings.HasPrefix(cleanLine, "index ") || strings.HasPrefix(cleanLine, "diff --git") {
			continue
		}
		// Condense hunk headers: @@ -1,4 +1,4 @@ -> @@
		if strings.HasPrefix(cleanLine, "@@") {
			result = append(result, "@@")
			continue
		}
		// Simplify file markers
		if strings.HasPrefix(cleanLine, "--- a/") {
			result = append(result, "--- "+strings.TrimPrefix(cleanLine, "--- a/"))
			continue
		}
		if strings.HasPrefix(cleanLine, "+++ b/") {
			result = append(result, "+++ "+strings.TrimPrefix(cleanLine, "+++ b/"))
			continue
		}
		// Pass through everything else to preserve EOF markers, renames, and file modes
		result = append(result, cleanLine)
	}
	return strings.Join(result, "\n")
}

// GitBranchParser (NEW)
type GitBranchParser struct{}

func (g *GitBranchParser) Name() string { return "git_branch" }
func (g *GitBranchParser) CanParse(cmd string, args []string) bool {
	return cmd == "git" && getGitSubcommand(args) == "branch"
}
func (g *GitBranchParser) Parse(output string) string {
	lines := strings.Split(output, "\n")
	var current string
	var others []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "*") {
			current = trimmed
		} else {
			others = append(others, trimmed)
		}
	}
	res := current
	if len(others) > 0 {
		res += fmt.Sprintf("\n(+ %d other branches)", len(others))
	}
	return res
}

// CompositeGitParser (NEW) - Handles things like "git status; git diff"
type CompositeGitParser struct {
	statusParser *GitStatusParser
	logParser    *GitLogParser
	diffParser   *GitDiffParser
}

func (c *CompositeGitParser) Name() string { return "git_composite" }
func (c *CompositeGitParser) CanParse(cmd string, args []string) bool {
	// Check if it's a shell-joined command containing git
	return strings.Contains(cmd, "git ") && (strings.Contains(cmd, ";") || strings.Contains(cmd, "&"))
}
func (c *CompositeGitParser) Parse(output string) string {
	lines := strings.Split(output, "\n")
	var result []string

	// Validate an entire commit record before accepting any compression. A
	// fallback returns the original capture, including earlier sections and ANSI.
	for i := 0; i < len(lines); i++ {
		cleanLine := ansiRegex.ReplaceAllString(lines[i], "")
		trimmed := strings.TrimSpace(cleanLine)

		if strings.HasPrefix(cleanLine, "commit ") {
			if len(lines)-i < 5 {
				return output
			}
			record := ansiRegex.ReplaceAllString(strings.Join(lines[i:i+5], "\n"), "")
			summary := (&GitLogParser{}).Parse(record)
			if summary == record {
				return output
			}
			// Only a new record or recognized Git section may follow the
			// single subject. Bodies, signatures and unknown/truncated metadata
			// are unsupported, rather than silently discarded.
			next := i + 5
			for next < len(lines) && lines[next] == "" {
				next++
			}
			if next < len(lines) {
				boundary := ansiRegex.ReplaceAllString(lines[next], "")
				if !strings.HasPrefix(boundary, "commit ") &&
					!strings.HasPrefix(boundary, "diff --git ") &&
					!strings.HasPrefix(boundary, "On branch ") {
					return output
				}
			}
			result = append(result, summary)
			i = next - 1
			continue
		}

		if strings.HasPrefix(cleanLine, "index ") || strings.HasPrefix(cleanLine, "diff --git") {
			continue
		}
		if strings.HasPrefix(cleanLine, "@@") {
			result = append(result, "@@")
			continue
		}
		if strings.HasPrefix(cleanLine, "--- a/") {
			result = append(result, "--- "+strings.TrimPrefix(cleanLine, "--- a/"))
			continue
		}
		if strings.HasPrefix(cleanLine, "+++ b/") {
			result = append(result, "+++ "+strings.TrimPrefix(cleanLine, "+++ b/"))
			continue
		}

		// Noise filtering for git status elements
		if strings.HasPrefix(trimmed, "(use ") ||
			strings.HasPrefix(trimmed, "On branch") || strings.HasPrefix(trimmed, "Your branch") ||
			strings.Contains(trimmed, "nothing to commit") || strings.Contains(trimmed, "no changes added") ||
			strings.HasPrefix(trimmed, "Changes not staged") || strings.HasPrefix(trimmed, "Changes to be committed") ||
			strings.HasPrefix(trimmed, "Untracked files") {
			continue
		}

		// Pass through all remaining lines (including metadata, empty lines, and diff chunks)
		result = append(result, cleanLine)
	}

	return strings.Join(result, "\n")
}

// GitShowParser (NEW)
type GitShowParser struct {
	comp *CompositeGitParser
}

func (g *GitShowParser) Name() string { return "git_show" }
func (g *GitShowParser) CanParse(cmd string, args []string) bool {
	if cmd != "git" || getGitSubcommand(args) != "show" {
		return false
	}
	// Arbitrary object selectors can name blobs whose contents look exactly
	// like commits. Without an object lookup, support only the default HEAD.
	for i, arg := range args {
		if arg == "show" {
			return i == len(args)-1 || (i == len(args)-2 && args[i+1] == "HEAD")
		}
	}
	return false
}
func (g *GitShowParser) Parse(output string) string {
	// show can return arbitrary blob contents or annotated tags. Only a
	// leading default-format commit is eligible for composite compression.
	if !strings.HasPrefix(ansiRegex.ReplaceAllString(output, ""), "commit ") {
		return output
	}
	if g.comp == nil {
		g.comp = &CompositeGitParser{}
	}
	// Git show output looks just like a composite of git log and git diff!
	// CompositeGitParser validates the record before compressing it.
	return g.comp.Parse(output)
}
