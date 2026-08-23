package parser

import (
	"fmt"
	"strings"
)

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
	lines := strings.Split(output, "\n")
	var result []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "(use ") ||
			strings.HasPrefix(trimmed, "On branch") || strings.HasPrefix(trimmed, "Your branch") ||
			strings.Contains(trimmed, "nothing to commit") || strings.Contains(trimmed, "no changes added") ||
			strings.HasPrefix(trimmed, "Changes not staged") || strings.HasPrefix(trimmed, "Changes to be committed") ||
			strings.HasPrefix(trimmed, "Untracked files") ||
			strings.HasPrefix(trimmed, "Everything up-to-date") ||
			strings.HasPrefix(trimmed, "To ") ||
			strings.Contains(trimmed, "->") { // push output
			continue
		}
		result = append(result, trimmed)
	}

	if len(result) == 0 {
		return "Git: Success (No verbose output)"
	}

	if len(result) > 20 {
		return strings.Join(result[:10], "\n") + "\n...\n" + strings.Join(result[len(result)-10:], "\n")
	}
	return strings.Join(result, "\n")
}

// GitLogParser (Existing)
type GitLogParser struct{}

func (g *GitLogParser) Name() string { return "git_log" }
func (g *GitLogParser) CanParse(cmd string, args []string) bool {
	return cmd == "git" && getGitSubcommand(args) == "log"
}
func (g *GitLogParser) Parse(output string) string {
	lines := strings.Split(output, "\n")
	var result []string
	var currentCommit, currentAuthor, currentDate, currentSubject string
	for _, line := range lines {
		if strings.HasPrefix(line, "commit ") {
			if currentCommit != "" {
				result = append(result, formatCommit(currentCommit, currentAuthor, currentDate, currentSubject))
			}
			currentCommit = strings.TrimPrefix(line, "commit ")
			if len(currentCommit) > 7 {
				currentCommit = currentCommit[:7]
			}
			currentAuthor, currentDate, currentSubject = "", "", ""
		} else if strings.HasPrefix(line, "Author: ") {
			currentAuthor = strings.TrimPrefix(line, "Author: ")
			if idx := strings.Index(currentAuthor, " <"); idx != -1 {
				currentAuthor = currentAuthor[:idx]
			}
		} else if strings.HasPrefix(line, "Date: ") {
			currentDate = strings.TrimPrefix(line, "Date: ")
			fields := strings.Fields(currentDate)
			if len(fields) >= 3 {
				currentDate = fields[1] + " " + fields[2] + " " + fields[4]
			}
		} else if strings.HasPrefix(line, "    ") && currentSubject == "" {
			currentSubject = strings.TrimSpace(line)
		}
	}
	if currentCommit != "" {
		result = append(result, formatCommit(currentCommit, currentAuthor, currentDate, currentSubject))
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
		if strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "diff --git") {
			continue
		}
		// Condense hunk headers: @@ -1,4 +1,4 @@ -> @@
		if strings.HasPrefix(line, "@@") {
			result = append(result, "@@")
			continue
		}
		// Simplify file markers
		if strings.HasPrefix(line, "--- a/") {
			result = append(result, "--- "+strings.TrimPrefix(line, "--- a/"))
			continue
		}
		if strings.HasPrefix(line, "+++ b/") {
			result = append(result, "+++ "+strings.TrimPrefix(line, "+++ b/"))
			continue
		}
		// Only keep changes and minimal context if needed, but here we keep all changed lines
		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") {
			result = append(result, line)
		}
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
	lines := strings.Split(output, "
")
	var result []string

	var currentCommit, currentAuthor, currentDate, currentSubject string

	flushCommit := func() {
		if currentCommit != "" {
			result = append(result, formatCommit(currentCommit, currentAuthor, currentDate, currentSubject))
			currentCommit = ""
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(line, "commit ") {
			flushCommit()
			currentCommit = strings.TrimPrefix(line, "commit ")
			if len(currentCommit) > 7 {
				currentCommit = currentCommit[:7]
			}
			currentAuthor, currentDate, currentSubject = "", "", ""
			continue
		} else if strings.HasPrefix(line, "Author: ") {
			currentAuthor = strings.TrimPrefix(line, "Author: ")
			if idx := strings.Index(currentAuthor, " <"); idx != -1 {
				currentAuthor = currentAuthor[:idx]
			}
			continue
		} else if strings.HasPrefix(line, "Date: ") {
			currentDate = strings.TrimPrefix(line, "Date: ")
			fields := strings.Fields(currentDate)
			if len(fields) >= 3 {
				currentDate = fields[1] + " " + fields[2] + " " + fields[4]
			}
			continue
		} else if strings.HasPrefix(line, "    ") && currentSubject == "" && currentCommit != "" {
			currentSubject = trimmed
			continue
		}

		if currentCommit != "" && !strings.HasPrefix(line, "    ") {
			flushCommit()
		}

		if strings.HasPrefix(line, "index ") || strings.HasPrefix(line, "diff --git") {
			continue
		}
		if strings.HasPrefix(line, "@@") {
			result = append(result, "@@")
			continue
		}
		if strings.HasPrefix(line, "--- a/") {
			result = append(result, "--- "+strings.TrimPrefix(line, "--- a/"))
			continue
		}
		if strings.HasPrefix(line, "+++ b/") {
			result = append(result, "+++ "+strings.TrimPrefix(line, "+++ b/"))
			continue
		}

		if strings.HasPrefix(line, "+") || strings.HasPrefix(line, "-") || strings.HasPrefix(line, " ") {
			result = append(result, line)
			continue
		}
		
		if line == "" {
			result = append(result, line)
			continue
		}

		if strings.HasPrefix(trimmed, "(use ") ||
			strings.HasPrefix(trimmed, "On branch") || strings.HasPrefix(trimmed, "Your branch") ||
			strings.Contains(trimmed, "nothing to commit") || strings.Contains(trimmed, "no changes added") ||
			strings.HasPrefix(trimmed, "Changes not staged") || strings.HasPrefix(trimmed, "Changes to be committed") ||
			strings.HasPrefix(trimmed, "Untracked files") {
			continue
		}

		result = append(result, line)
	}

	flushCommit()
	return strings.Join(result, "
")
}

// GitShowParser (NEW)
type GitShowParser struct{
	comp *CompositeGitParser
}

func (g *GitShowParser) Name() string { return "git_show" }
func (g *GitShowParser) CanParse(cmd string, args []string) bool {
	return cmd == "git" && getGitSubcommand(args) == "show"
}
func (g *GitShowParser) Parse(output string) string {
	if g.comp == nil {
		g.comp = &CompositeGitParser{}
	}
	// Git show output looks just like a composite of git log and git diff!
	// We can safely pass it through CompositeGitParser.
	return g.comp.Parse(output)
}
