package pi

import (
	"strings"
	"testing"
)

const hookDefaultGitLog = "commit 0123456789abcdef0123456789abcdef01234567\nAuthor: Example Author <author@example.invalid>\nDate:   Sun Mar 15 22:49:38 2026 -0400\n\n    A subject\n"

func assertGitLogPassthrough(t *testing.T, req HookRequest) {
	t.Helper()
	req.StoragePath = t.TempDir()
	got := OptimizeHook(req)
	strategy := "passthrough"
	upstream := upstreamTruncationRegex.MatchString(req.Output)
	if upstream {
		strategy = "upstream-host-truncation"
	}
	want := responseMetadata(req.Output, PiRedact(req.Output), strategy, upstream)
	want.Passthrough = true
	if got != want {
		t.Fatalf("got %#v, want redacted passthrough %#v", got, want)
	}
}

func TestOptimizeHookGitLogUnsupportedPreserved(t *testing.T) {
	for name, output := range map[string]string{
		"oneline":    "abc1234 subject\n123abcd token=synthetic\n",
		"custom":     "abc1234|Author|subject\n\n",
		"graph":      "* abc1234 subject\n| * 123abcd another\n",
		"prefix":     "prefix\n" + hookDefaultGitLog,
		"suffix":     hookDefaultGitLog + "suffix token=synthetic\n",
		"stat":       hookDefaultGitLog + "\n file.go | 1 +\n",
		"patch":      hookDefaultGitLog + "\ndiff --git a/x b/x\n-a\n+b\n",
		"multiline":  hookDefaultGitLog + "    body\n",
		"short-date": strings.Replace(hookDefaultGitLog, "Sun Mar 15 22:49:38 2026 -0400", "Sun Mar 15", 1),
		"crlf":       strings.ReplaceAll(hookDefaultGitLog, "\n", "\r\n"),
	} {
		t.Run(name, func(t *testing.T) {
			assertGitLogPassthrough(t, HookRequest{Command: "git log", Output: output})
		})
	}
}

func TestOptimizeHookMixedCommandPreservation(t *testing.T) {
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
		t.Run(command, func(t *testing.T) {
			// A fully recognizable log still must not be parsed when command
			// provenance is ambiguous, even if the other command was silent.
			assertGitLogPassthrough(t, HookRequest{Command: command, Output: hookDefaultGitLog})
			assertGitLogPassthrough(t, HookRequest{Command: command, Output: hookDefaultGitLog + "\nsecond token=synthetic\r\n\n"})
		})
	}
}

func TestOptimizeHookGitLogSupportedAndGuards(t *testing.T) {
	got := OptimizeHook(HookRequest{Command: "git log -2", Output: hookDefaultGitLog, StoragePath: t.TempDir()})
	if got.Output != "0123456 | Example Author | Mar 15 2026 | A subject" || got.Parser != "git_log" || got.Passthrough {
		t.Fatalf("normal log must still compress: %#v", got)
	}
	for _, req := range []HookRequest{
		{Command: "git log", Output: hookDefaultGitLog, ExitCode: 1},
		{Command: "git log", Output: hookDefaultGitLog, RawBypass: true},
		{Command: "git log", Output: hookDefaultGitLog, EnabledParsers: map[string]bool{"git_log": false}},
		{Command: "git log", Output: strings.Replace(hookDefaultGitLog, "A subject", "ERROR token=synthetic", 1)},
		{Command: "git log", Output: "warning: token=synthetic\n"},
		{Command: "git log", Output: "Tests 2 passed\n"},
		{Command: "git log", Output: "... output truncated by host ...\n"},
		{Command: "git log", Output: "{\"token\":\"synthetic\"}"},
		{Command: "git log", Output: "true"},
		{Command: "git rev-parse HEAD", Output: hookDefaultGitLog},
	} {
		assertGitLogPassthrough(t, req)
	}
	redacted := OptimizeHook(HookRequest{Command: "git log", Output: strings.Replace(hookDefaultGitLog, "A subject", "token=synthetic", 1), StoragePath: t.TempDir()})
	if redacted.Parser != "git_log" || !strings.HasSuffix(redacted.Output, "token=[REDACTED]") {
		t.Fatalf("supported log must still redact: %#v", redacted)
	}
}

func TestPiOptimizeMixedCommandPreservation(t *testing.T) {
	input := strings.Repeat("ordinary aggregate output token=synthetic\n", 500)
	for _, command := range []string{"git log; echo second", "git log\necho second", "git log\n", "git log --author='ordinary author'"} {
		got, err := PiOptimizeWithConfig(command, input, 0, PiConfig{ThresholdBytes: 200, Redact: true, StoragePath: t.TempDir()})
		if err != nil || got != PiRedact(input) {
			t.Fatalf("shared preservation guard failed for %q: %v", command, err)
		}
	}
}
