package parser

import (
	"fmt"
	"strings"
	"testing"
)

func TestParsersCanParse(t *testing.T) {
	parsers := GetAllParsers()
	for _, p := range parsers {
		// Just ensure we hit CanParse for each
		p.CanParse("ls", []string{"-la"})
		p.CanParse("git", []string{"status"})
		p.CanParse("go", []string{"test"})
		p.CanParse("npm", []string{"install"})
		p.CanParse("docker", []string{"ps"})
		p.CanParse("bd", []string{"ready"})
	}
}

func TestWebParser_Edge(t *testing.T) {
	p := &WebParser{}

	// Large JSON retains all keys and values, not just a key summary.
	largeJSON := `{"a": 1, "b": 2, "c": 3, "d": 4, "e": 5, "f": 6, "g": 7, "h": 8, "i": 9, "j": 10, "k": 11, "l": 12, "m": 13, "n": 14, "o": 15, "p": 16, "q": 17, "r": 18, "s": 19, "t": 20, "u": 21, "v": 22, "w": 23, "x": 24, "y": 25, "z": 26, "long": "this is a very long string to make the json large enough to trigger the key summary branch if it was over 500 chars total minified........................................................................................................................................................................................................................................................................................................................................................................................................................................"}`
	output := p.Parse(largeJSON)
	wantJSON := `{"a":1,"b":2,"c":3,"d":4,"e":5,"f":6,"g":7,"h":8,"i":9,"j":10,"k":11,"l":12,"m":13,"n":14,"o":15,"p":16,"q":17,"r":18,"s":19,"t":20,"u":21,"v":22,"w":23,"x":24,"y":25,"z":26,"long":"this is a very long string to make the json large enough to trigger the key summary branch if it was over 500 chars total minified........................................................................................................................................................................................................................................................................................................................................................................................................................................"}`
	if output != wantJSON {
		t.Errorf("large JSON lost content: got %q, want %q", output, wantJSON)
	}

	// HTML with title
	html := `<html><head><title>Test Page</title></head><body>Hello</body></html>`
	output = p.Parse(html)
	if !strings.Contains(output, "HTML Content: [Test Page]") {
		t.Errorf("Expected HTML Content with title, got %s", output)
	}

	// HTML without title
	htmlNoTitle := `<html><body>Hello</body></html>`
	output = p.Parse(htmlNoTitle)
	if !strings.Contains(output, "HTML Content (") {
		t.Errorf("Expected HTML Content summary, got %s", output)
	}

	// Long text
	longText := strings.Repeat("A", 1200)
	output = p.Parse(longText)
	if !strings.Contains(output, "... (Total: 1200 chars)") {
		t.Errorf("Expected truncation summary, got %s", output)
	}
}

func TestBDParser_Edge(t *testing.T) {
	p := &BDParser{}

	// Help output
	help := `Usage: bd [command]
Available Commands:
  ready      Show issues ready for work
Flags:
  -h, --help  help for bd`
	output := p.Parse(help)
	if !strings.Contains(output, "Usage: bd") {
		t.Errorf("Expected usage in output, got %s", output)
	}

	// Many issues
	issues := "Total: 50 issues\n"
	for i := 0; i < 50; i++ {
		issues += fmt.Sprintf("○ issue-%d P1 summary\n", i)
	}
	output = p.Parse(issues)
	if !strings.Contains(output, "more lines") {
		t.Errorf("Expected truncation message, got %s", output)
	}

	// Long line
	longLine := "○ " + strings.Repeat("A", 200)
	output = p.Parse(longLine)
	if !strings.Contains(output, "...") {
		t.Errorf("Expected truncation of long line, got %s", output)
	}
}

func TestGithubReleaseParser(t *testing.T) {
	p := &GitHubReleaseParser{}
	input := `Tag: v1.0.0
Title: Release 1.0.0
Assets:
pith sha256:1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef 10 MB
View on GitHub: http://example.com`
	output := p.Parse(input)
	if output == "" {
		t.Error("Expected output for github release")
	}
}
