package runner

import (
	"strings"
	"testing"

	"pith/pkg/config"
)

func TestRunnerSelectParserMixedCommandPreservation(t *testing.T) {
	r := NewRunner(&config.Config{MaxLines: 10, HeadLines: 2, TailLines: 2}, nil)
	for _, command := range []string{
		"git log; echo second", "git log&&echo second", "git log || echo second",
		"git log | cat", "git log & echo second", "git log\necho second",
		"git log\r\necho second", "git log\recho second", "git log\n",
		"git log > fixture", "git log < fixture", "git log $(echo main)",
		"git log `echo main`", "(git log)", "{ git log; }",
		"git log --author='ordinary author'", "git log --author=\"a|b\"",
		"git status; echo second", "rg pattern fixture; echo second",
		"cd fixture && git log", "pwsh -Command \"git log; echo second\"",
	} {
		if got := r.selectParser(command); got != nil {
			t.Errorf("%q selected %s for aggregate output", command, got.Name())
		}
	}
	if got := r.selectParser("git log -2"); got == nil || got.Name() != "git_log" {
		t.Fatalf("normal single-command parser selection changed: %v", got)
	}
	r.cfg.EnabledParsers = map[string]bool{"git_log": false}
	if got := r.selectParser("git log"); got != nil {
		t.Fatalf("disabled parser selected: %v", got)
	}
	// This test isolates dispatch without executing commands or opening storage.
	// Independent runner truncation remains active, not a lossless runner promise.
	if got := r.ApplyMiddleOutTruncation(strings.Repeat("ordinary output\n", 30)); !strings.Contains(got, "removed by Pith") {
		t.Fatal("independent truncation policy unexpectedly changed")
	}
}
