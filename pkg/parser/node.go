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
	var final []string
	for _, line := range lines {
		final = append(final, line)
	}

	if len(final) > 100 {
		head := final[:30]
		tail := final[len(final)-70:]
		res := strings.Join(head, "\n")
		res += fmt.Sprintf("\n\n... (+ %d more lines truncated by Pith) ...\n\n", len(final)-100)
		res += strings.Join(tail, "\n")
		return res
	}

	return strings.Join(final, "\n")
}
