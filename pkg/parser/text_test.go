package parser

import (
	"strings"
	"testing"
)

func TestGrepParser(t *testing.T) {
	p := &GrepParser{}
	input := `pkg/parser/git.go:10:func TestSomething() {
pkg/parser/git.go:15:	fmt.Println("hi")
pkg/runner/runner.go:20:func NewRunner() {
`
	output := p.Parse(input)
	if !strings.Contains(output, "pkg/parser/git.go:") {
		t.Errorf("Output missing file header: %s", output)
	}
	if strings.Count(output, "pkg/parser/git.go:") > 1 {
		t.Error("File header should only appear once per file group")
	}
}

func TestMinifyParser(t *testing.T) {
	p := &MinifyParser{}
	input := `{
  "name": "pith",
  // This is a comment
  "version": "0.3.0",
  "enabled_parsers": {
    "git_status": true
  }
}
`
	// Test CanParse first
	if !p.CanParse("cat", []string{"config.json"}) {
		t.Error("MinifyParser should handle .json files via cat")
	}

	output := p.Parse(input)
	// Comments are not valid JSON; retain every captured byte rather than
	// inventing a repaired document or changing quoted whitespace.
	if output != input {
		t.Errorf("malformed JSON must pass through unchanged: got %q, want %q", output, input)
	}
}
