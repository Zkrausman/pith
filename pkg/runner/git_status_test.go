package runner

import (
	"pith/pkg/config"
	"strings"
	"testing"
)

func TestGitStatusRenameNormalParserRoute(t *testing.T) {
	r := NewRunner(&config.Config{EnabledParsers: map[string]bool{"git_status": true}}, nil)
	p := r.selectParser("git status")
	if p == nil || p.Name() != "git_status" {
		t.Fatalf("git status parser not selected: %v", p)
	}
	got := p.Parse("On branch main\nChanges to be committed:\n    renamed:    old name.go -> new name.go\n")
	if !strings.Contains(got, "old name.go -> new name.go") {
		t.Fatalf("normal parser route lost rename: %q", got)
	}
}
