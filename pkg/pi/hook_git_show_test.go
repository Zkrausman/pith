package pi

import (
	"strings"
	"testing"
)

func TestOptimizeHookGitShowFallback(t *testing.T) {
	for _, output := range []string{
		"commit abc1234\nAuthor: Example <example@example.invalid>\nDate:   Sun Mar 15\n\n    token=synthetic\n",
		strings.Replace(hookDefaultGitLog, "    A subject\n", "", 1),
		hookDefaultGitLog + "\n    token=synthetic\n",
		"On branch main\nindex token=synthetic\n@@ literal\n",
	} {
		for _, command := range []string{"git show", "git show HEAD:fixture", "git status; git show"} {
			assertGitLogPassthrough(t, HookRequest{Command: command, Output: output})
		}
	}
	got := OptimizeHook(HookRequest{Command: "git show", Output: strings.Replace(hookDefaultGitLog, "A subject", "token=synthetic", 1), StoragePath: t.TempDir()})
	if got.Parser != "git_show" || got.Passthrough || !strings.HasSuffix(got.Output, "token=[REDACTED]") {
		t.Fatalf("supported show must compress and redact: %#v", got)
	}
}
