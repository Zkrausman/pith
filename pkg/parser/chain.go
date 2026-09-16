package parser

import (
	"strings"
)

type ChainParser struct {
}

func (c *ChainParser) Name() string {
	return "chain"
}

func (c *ChainParser) CanParse(cmd string, args []string) bool {
	fullCmd := strings.Join(append([]string{cmd}, args...), " ")
	return strings.ContainsAny(fullCmd, ";|&")
}

func (c *ChainParser) Parse(output string) string {
	return output
}

func (c *ChainParser) SplitSubCommands(fullCmd string) []string {
	// Simple split by shell operators
	delimiters := []string{";", "&&", "||", "|"}
	subcmds := []string{fullCmd}

	for _, delim := range delimiters {
		var newSubcmds []string
		for _, s := range subcmds {
			parts := strings.Split(s, delim)
			for _, p := range parts {
				trimmed := strings.TrimSpace(p)
				if trimmed != "" {
					newSubcmds = append(newSubcmds, trimmed)
				}
			}
		}
		subcmds = newSubcmds
	}
	return subcmds
}

// MayContainShellSyntax is a conservative dispatch veto, not a shell lexer.
// Quoted literals also veto parsing: aggregate output has no trustworthy
// per-command boundaries. Never split or reconstruct commands to bypass it.
func MayContainShellSyntax(command string) bool {
	return strings.ContainsAny(command, ";|&\r\n`$()<>{}\"'")
}
