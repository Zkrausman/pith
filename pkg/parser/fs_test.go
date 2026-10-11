package parser

import (
	"strings"
	"testing"
)

func TestLsParser(t *testing.T) {
	p := &LsParser{}

	// Test CanParse
	if !p.CanParse("ls", []string{"-l"}) {
		t.Error("LsParser should handle ls")
	}
	if !p.CanParse("dir", []string{}) {
		t.Error("LsParser should handle dir")
	}

	input := `total 8
-rw-r--r--  1 user  group  123 Mar 15 22:30 main.go
-rw-r--r--  1 user  group  456 Mar 15 22:31 README.md
`
	output := p.Parse(input)
	if output != input {
		t.Errorf("without a proven listing profile, LsParser must preserve output exactly: got %q, want %q", output, input)
	}
}

func TestFindParser(t *testing.T) {
	p := &FindParser{}

	// Test CanParse
	if !p.CanParse("find", []string{".", "-name", "*.go"}) {
		t.Error("FindParser should handle find")
	}
	if !p.CanParse("where", []string{"pith"}) {
		t.Error("FindParser should handle where")
	}

	input := ""
	for i := 0; i < 60; i++ {
		input += "file" + strings.Repeat("a", i) + ".go\n"
	}
	output := p.Parse(input)
	if !strings.Contains(output, "(+ 10 more results)") {
		t.Errorf("Expected truncation message, got output of length %d", len(strings.Split(output, "\n")))
	}
}

func TestTreeParser(t *testing.T) {
	p := &TreeParser{}

	// Test CanParse
	if !p.CanParse("tree", []string{}) {
		t.Error("TreeParser should handle tree")
	}

	input := `
.
├── pkg
│   └── parser
│       └── fs.go
└── main.go
`
	output := p.Parse(input)
	if strings.Contains(output, "├──") || strings.Contains(output, "│") {
		t.Errorf("Output still contains tree characters: %s", output)
	}
	if !strings.Contains(output, "pkg") || !strings.Contains(output, "fs.go") {
		t.Errorf("Output missing directory structure: %s", output)
	}
}

func TestDuParser(t *testing.T) {
	p := &DuParser{}

	// Test CanParse
	if !p.CanParse("du", []string{"-sh"}) {
		t.Error("DuParser should handle du")
	}

	input := `
123	./pkg/parser
456	./pkg/config
789	.
`
	output := p.Parse(input)
	if !strings.Contains(output, "123	./pkg/parser") {
		t.Errorf("Unexpected du output format: %s", output)
	}
}
