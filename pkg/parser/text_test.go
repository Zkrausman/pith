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

func TestGrepParserWindowsDrivePaths(t *testing.T) {
	if !(&GrepParser{}).CanParse("rg", []string{"needle", "C:\\repo"}) {
		t.Fatal("rg output is not routed to GrepParser")
	}
	input := "C:\\repo\\file.go:12:match:with:colons\nC:\\repo\\file.go:15:more\nD:/src/x.go:2:other\n"
	want := "C:\\repo\\file.go:\n  12: match:with:colons\n  15: more\n\nD:/src/x.go:\n  2: other"
	if got := (&GrepParser{}).Parse(input); got != want {
		t.Fatalf("Windows paths changed: got %q, want %q", got, want)
	}
}

func TestGrepParserPosixAndPlainLines(t *testing.T) {
	input := "status line\nsrc/x.go:4:found:here\nsrc/x.go:5:again\n"
	want := "status line\n\nsrc/x.go:\n  4: found:here\n  5: again"
	if got := (&GrepParser{}).Parse(input); got != want {
		t.Fatalf("POSIX/plain output changed: got %q, want %q", got, want)
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
	if strings.Contains(output, "//") {
		t.Error("Minified output should not contain comments")
	}
	if !strings.Contains(output, "\n") {
		t.Error("Balanced minification should preserve some newlines for readability")
	}
	if strings.Contains(output, "  ") {
		t.Error("Minified output should have collapsed indentation")
	}
}
