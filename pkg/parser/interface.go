package parser

import (
	"path/filepath"
	"strings"
)

type Parser interface {
	Name() string
	CanParse(cmd string, args []string) bool
	Parse(output string) string
}

// MatchCommand checks if the given command matches the target,
// handling Windows extensions and full paths.
func MatchCommand(cmd string, target string) bool {
	// Base case
	if cmd == target {
		return true
	}

	// Normalize path separators to forward slashes for cross-platform matching
	normalized := strings.ReplaceAll(cmd, "\\", "/")
	base := strings.ToLower(filepath.Base(normalized))
	targetLow := strings.ToLower(target)

	if base == targetLow {
		return true
	}

	// Windows extensions
	for _, ext := range []string{".exe", ".cmd", ".bat", ".ps1"} {
		if base == targetLow+ext {
			return true
		}
	}

	return false
}

func GetAllParsers() []Parser {
	parsers := []Parser{
		// Git
		&CompositeGitParser{},
		&GitStatusParser{},
		&GitLogParser{},
		&GitShowParser{},
		&GitDiffParser{},
		&GitBranchParser{},
		// FS
		&LsParser{},
		&FindParser{},
		&TreeParser{},
		&DuParser{},
		// Text
		&GrepParser{},
		&MinifyParser{},
		&SourceParser{},
		// Infra
		&EnvParser{},
		&DockerPsParser{},
		&GitHubReleaseParser{},
		&GitHubParser{},
		&DependencyParser{},
		&TestParser{},
		&GoToolCoverParser{},
		// New Tools
		&WebParser{},
		&PithParser{},
		&PowerShellParser{},
		&GetContentParser{},
		&GoParser{},
		&VitestParser{},
		&BDParser{},
		&PromptfooParser{},
		&ThneedParser{},
		&SnagParser{},
		&NodeParser{},
		&NPMParser{},
	}
	return append(parsers, &ChainParser{})
}
