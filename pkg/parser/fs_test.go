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
	if !strings.Contains(output, "main.go") || !strings.Contains(output, "123") || strings.Contains(output, "-rw-r--r--") {
		t.Errorf("Expected filenames and sizes, and NO modes in output, got: %s", output)
	}
	if strings.Contains(output, "total") || strings.Contains(output, "user") {
		t.Errorf("Output contains redundant metadata: %s", output)
	}
}

func TestLsParserPreservesFilenamesWithSpaces(t *testing.T) {
	p := &LsParser{}
	for _, tc := range []struct{ name, input, want string }{
		{"plain", "project notes.md\n", "project notes.md"},
		{"plain multiple spaces", "project  notes.md\n", "project  notes.md"},
		{"plain four words", "daily project meeting notes.md\n", "daily project meeting notes.md"},
		{"plain without spaces", "main.go\n", "main.go"},
		{"long", "total 8\n-rw-r--r-- 1 user group 42 Jan 1 12:00 project notes.md\n", "42 project notes.md"},
		{"long multiple spaces", "-rw-r--r-- 1 user group 42 Jan 1 12:00 project  notes.md\n", "42 project  notes.md"},
		{"dir long", "-a--- 9/24/2026 10:00 42 project notes.md\n", "42 project notes.md"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := p.Parse(tc.input); got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
		})
	}
	if got := p.Parse("one name.md\ntwo name.md\n"); got != "one name.md two name.md" {
		t.Fatalf("short list layout changed: %q", got)
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
