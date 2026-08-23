package parser

import (
	"fmt"
	"strings"
)

type NodeParser struct{}

func (n *NodeParser) Name() string { return "node" }

func (n *NodeParser) CanParse(cmd string, args []string) bool {
	return MatchCommand(cmd, "node")
}

func (n *NodeParser) Parse(output string) string {
	lines := strings.Split(output, "\n")
	// Node output often contains stack traces or JSON where indentation is important.
	// We preserve empty lines and indentation but bound the size.
	// Stack traces often have the most critical info at the top (error message) and bottom (where it failed).

	if len(lines) > 100 {
		head := lines[:30]
		tail := lines[len(lines)-70:]
		res := strings.Join(head, "\n")
		res += fmt.Sprintf("\n\n... (+ %d more lines truncated by Pith) ...\n\n", len(lines)-100)
		res += strings.Join(tail, "\n")
		return res
	}

	return strings.Join(lines, "\n")
}
