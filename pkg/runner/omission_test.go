package runner

import (
	"strings"
	"testing"

	"pith/pkg/config"
)

func TestMiddleOutOmissionAccounting(t *testing.T) {
	const input = "head1\nhead2\nhidden3\ncontext4\nERROR failure5\ncontext6\ntail7\ntail8"
	const want = "head1\nhead2\n\n... [1 lines of non-critical output removed by Pith] ...\n\ncontext4\nERROR failure5\ncontext6\ntail7\ntail8"
	r := &Runner{cfg: &config.Config{MaxLines: 5, HeadLines: 2, TailLines: 2}}
	for _, suffix := range []string{"", "\n"} {
		t.Run("final-newline="+strings.ReplaceAll(suffix, "\n", "yes"), func(t *testing.T) {
			if got := r.ApplyMiddleOutTruncation(input + suffix); got != want+suffix {
				t.Fatalf("truncated output = %q, want %q", got, want+suffix)
			}
		})
	}
}
