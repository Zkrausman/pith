package parser

import (
	"strings"
	"testing"
)

func TestSourceParser(t *testing.T) {
	p := &SourceParser{}
	input := `package main
	
	import "fmt"
	
	// This is a generic comment
	// BUG: This is a high-signal comment
	func main() {
		/* Multi-line
		   TODO: Fix this
		   comment */
		fmt.Println("Hello") // Inline HACK
		fmt.Println("World") // Standard inline
	}`

	output := p.Parse(input)
	if output != input {
		t.Errorf("Source without language context must be preserved exactly; got %q, want %q", output, input)
	}
}

func TestTestParserFidelity(t *testing.T) {
	p := &TestParser{}

	input := `PASS: TestOne
FAIL: TestTwo
    Error: Assertion failed
    Expected: 1
    Actual: 2

    Stack Trace:
    at main.go:10
    at testing.go:100

ok  	pith/pkg/parser	0.123s
PASS: TestThree`

	output := p.Parse(input)

	if !strings.Contains(output, "FAIL: TestTwo") {
		t.Errorf("Expected failure line to be preserved")
	}
	if !strings.Contains(output, "Error: Assertion failed") {
		t.Errorf("Expected error details to be preserved")
	}
	if !strings.Contains(output, "Stack Trace:") {
		t.Errorf("Expected stack trace to be preserved (even after empty line)")
	}
	if !strings.Contains(output, "at main.go:10") {
		t.Errorf("Expected multi-line failure details to be preserved")
	}
	if !strings.Contains(output, "ok  	pith/pkg/parser") {
		t.Errorf("Expected summary line to be preserved")
	}
	hasOne := strings.Contains(output, "PASS: TestOne")
	hasThree := strings.Contains(output, "PASS: TestThree")
	if hasOne || hasThree {
		t.Errorf("Expected individual passing tests to be stripped. hasOne=%v, hasThree=%v. Actual output:\n%s", hasOne, hasThree, output)
	}
}

func TestGitHubReleaseParser(t *testing.T) {
	p := &GitHubReleaseParser{}
	input := `Tag: v0.5.4
Title: Release v0.5.4
Latest: true

Assets
NAME                    DIGEST                                                                   SIZE     
pith-linux-amd64        sha256:9f6385346102d93580eba389bf24c7ea6c95e9e60b58b0c1ebe2f989863a897a  19.31 MiB
pith-windows-amd64.exe  sha256:5edcbe01c20005a3e24f088bdc7f34535684b29d07bd7ad35d0ed2b78bf11e49  19.71 MiB

View on GitHub: https://github.com/Zkrausman/Pith/releases/tag/v0.5.4`

	output := p.Parse(input)
	if strings.Contains(output, "sha256:") {
		t.Errorf("Expected SHA digests to be removed")
	}
	if !strings.Contains(output, "- pith-linux-amd64 (19.31 MiB)") {
		t.Errorf("Expected asset name and size to be preserved")
	}
}

func TestChainParser(t *testing.T) {
	p := &ChainParser{}
	if !p.CanParse("git status; git log", []string{}) {
		t.Errorf("Expected ChainParser to handle semicolon")
	}
	if !p.CanParse("go test && go build", []string{}) {
		t.Errorf("Expected ChainParser to handle &&")
	}

	subcmds := p.SplitSubCommands("git status; git log && ls")
	if len(subcmds) != 3 {
		t.Errorf("Expected 3 sub-commands, got %d", len(subcmds))
	}
}

func TestWebParser(t *testing.T) {
	p := &WebParser{}

	// Test CanParse
	if !p.CanParse("curl", []string{"http://example.com"}) {
		t.Error("WebParser should handle curl")
	}
	if !p.CanParse("iwr", []string{"http://example.com"}) {
		t.Error("WebParser should handle iwr")
	}

	// Test HTML extraction
	htmlInput := `<!DOCTYPE html><html><head><title>Test Page</title></head><body>Hello</body></html>`
	htmlOutput := p.Parse(htmlInput)
	if !strings.Contains(htmlOutput, "HTML Content: [Test Page]") {
		t.Errorf("WebParser failed to extract HTML title: %s", htmlOutput)
	}

	// Test JSON minification
	jsonInput := `{"foo": "bar", "baz": 123}`
	jsonOutput := p.Parse(jsonInput)
	if strings.Contains(jsonOutput, " ") {
		t.Errorf("WebParser failed to minify JSON: %s", jsonOutput)
	}
}

func TestPithParser(t *testing.T) {
	p := &PithParser{}

	// Test CanParse
	if !p.CanParse("pith", []string{"gain"}) {
		t.Error("PithParser should handle pith")
	}
	if !p.CanParse("go", []string{"run", "main.go", "discover"}) {
		t.Error("PithParser should handle go run main.go")
	}

	// Test Log summary
	logInput := `[CMD] git status
[EXIT] 0
On branch main
nothing to commit`
	logOutput := p.Parse(logInput)
	if !strings.Contains(logOutput, "Command: git status") || !strings.Contains(logOutput, "Exit Code: 0") {
		t.Errorf("PithParser failed to summarize logs: %s", logOutput)
	}
}

func TestGoParser(t *testing.T) {
	p := &GoParser{}

	// Test CanParse
	if !p.CanParse("go", []string{"version"}) {
		t.Error("GoParser should handle go")
	}

	// Test Summary preservation
	input := `ok  	pith/pkg/parser	0.359s
FAIL	pith/pkg/runner	0.123s
TOTAL: 10 passed, 1 failed`
	output := p.Parse(input)
	if !strings.Contains(output, "ok  	pith/pkg/parser") || !strings.Contains(output, "FAIL") || !strings.Contains(output, "TOTAL") {
		t.Errorf("GoParser missing summary lines: %s", output)
	}
}
