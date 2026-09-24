package runner

import (
	"pith/pkg/config"
	"strings"
	"testing"
)

func TestLsSpacesNormalParserRoute(t *testing.T) {
	r := NewRunner(&config.Config{EnabledParsers: map[string]bool{"ls": true}}, nil)
	p := r.selectParser("ls -l")
	if p == nil || p.Name() != "ls" {
		t.Fatalf("ls parser not selected: %v", p)
	}
	got := p.Parse("-rw-r--r-- 1 user group 42 Jan 1 12:00 project notes.md\n")
	if !strings.Contains(got, "project notes.md") {
		t.Fatalf("normal parser route lost filename: %q", got)
	}
}
