package parser

import (
	"strings"
	"testing"
)

func TestGitStatusParser(t *testing.T) {
	p := &GitStatusParser{}
	input := `On branch main
Your branch is up to date with 'origin/main'.

Changes to be committed:
  (use "git restore --staged <file>..." to unstage)
	new file:   main.go

Changes not staged for commit:
  (use "git add <file>..." to update what will be committed)
  (use "git restore <file>..." to discard changes in working directory)
	modified:   pkg/parser/git.go

Untracked files:
  (use "git add <file>..." to include in what will be committed)
	pkg/parser/git_test.go

nothing added to commit but untracked files present (use "git add" to track)
`
	output := p.Parse(input)
	expected := []string{"new file:   main.go", "modified:   pkg/parser/git.go", "pkg/parser/git_test.go"}
	for _, exp := range expected {
		if !strings.Contains(output, exp) {
			t.Errorf("Expected output to contain %q, but got:\n%s", exp, output)
		}
	}
	if strings.Contains(output, "On branch") {
		t.Error("Output should not contain boilerplate 'On branch'")
	}
}

func TestGitLogParser(t *testing.T) {
	p := &GitLogParser{}
	input := `commit 3f7d7f7b12345678000000000000000000000000
Author: Zachary Krausman <zkrausman@gmail.com>
Date:   Sun Mar 15 22:49:38 2026 -0400

    Update: Display changelog during pith update

commit 1cd63311f8330490000000000000000000000000
Author: Zachary Krausman <zkrausman@gmail.com>
Date:   Sun Mar 15 22:30:00 2026 -0400

    Initial commit
`
	output := p.Parse(input)
	if !strings.Contains(output, "3f7d7f7 | Zachary Krausman | Mar 15 2026 | Update: Display changelog") {
		t.Errorf("Unexpected log output:\n%s", output)
	}
}

func TestGitDiffParser(t *testing.T) {
	p := &GitDiffParser{}
	input := `diff --git a/main.go b/main.go
index 1234567..89abcdef 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+import "fmt"
 func main() {
-	fmt.Println("hi")
+	fmt.Println("hello")
 }
`
	output := p.Parse(input)
	if strings.Contains(output, "index 1234567") {
		t.Error("Output should not contain index hashes")
	}
	if !strings.Contains(output, "+import \"fmt\"") {
		t.Error("Output should contain added lines")
	}
	if !strings.Contains(output, "@@") {
		t.Error("Output should contain condensed hunk headers")
	}
}

func TestGitBranchParser(t *testing.T) {
	p := &GitBranchParser{}
	input := `* main
  feature/test
  bugfix/fix-1
`
	output := p.Parse(input)
	if !strings.HasPrefix(output, "* main") {
		t.Errorf("Expected current branch to be first, got: %s", output)
	}
	if !strings.Contains(output, "(+ 2 other branches)") {
		t.Errorf("Expected branch count summary, got: %s", output)
	}
}

func TestCompositeGitParser(t *testing.T) {
	p := &CompositeGitParser{}
	input := `On branch main
modified:   main.go

commit 58fa1e900190f04d75832d77acaa0eaccf823b0b
Author: Zachary Krausman <zkrausman@gmail.com>
Date:   Mon Mar 16 01:51:29 2026 -0400

    chore: update changelog for v0.4.3
`
	// Test CanParse
	if !p.CanParse("git status & git log", []string{}) {
		t.Error("CompositeGitParser should handle shell-joined git commands")
	}

	output := p.Parse(input)
	if !strings.Contains(output, "modified:   main.go") {
		t.Error("Output should contain status info")
	}
	if !strings.Contains(output, "58fa1e9 | Zachary Krausman | Mar 16 2026 | chore: update changelog for v0.4.3") {
		t.Errorf("Output should contain condensed log info, got:\n%s", output)
	}
}
