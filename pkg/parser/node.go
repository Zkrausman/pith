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
	var result []string

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		
		// Typically, Node stack traces or large logs can be massive.
		// We just bound the output size here to be safe.
		result = append(result, trimmed)

		// Hard cutoff for node outputs to prevent massive json or array dumps
		if len(result) > 100 {
			break
		}
	}

	res := strings.Join(result, "\n")
	if len(lines) > 100 {
		res += fmt.Sprintf("\n... (+ %d more lines truncated by Pith)", len(lines)-100)
	}
	return res
}
