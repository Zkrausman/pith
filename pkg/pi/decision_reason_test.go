package pi

import (
	"encoding/json"
	"strings"
	"testing"

	"pith/pkg/telemetry"
)

func TestHookDecisionReasons(t *testing.T) {
	for _, tc := range []struct {
		name   string
		req    HookRequest
		reason telemetry.DecisionReason
		parser string
	}{
		{"empty", HookRequest{}, telemetry.DecisionUnknown, ""},
		{"no-command", HookRequest{Output: "ordinary output"}, telemetry.DecisionUnknown, ""},
		{"raw", HookRequest{Command: "git log", Output: hookDefaultGitLog, RawBypass: true}, telemetry.DecisionProtectedPassthrough, ""},
		{"shell", HookRequest{Command: "git log; echo second", Output: hookDefaultGitLog}, telemetry.DecisionProtectedPassthrough, ""},
		{"exit", HookRequest{Command: "git log", Output: hookDefaultGitLog, ExitCode: 1}, telemetry.DecisionProtectedPassthrough, ""},
		{"error", HookRequest{Command: "node", Output: "ERROR token=synthetic"}, telemetry.DecisionProtectedPassthrough, ""},
		{"warning", HookRequest{Command: "node", Output: "warning: synthetic"}, telemetry.DecisionProtectedPassthrough, ""},
		{"summary", HookRequest{Command: "node", Output: "Tests 2 passed"}, telemetry.DecisionProtectedPassthrough, ""},
		{"upstream", HookRequest{Command: "node", Output: "... output truncated by host ..."}, telemetry.DecisionProtectedPassthrough, ""},
		{"inspection", HookRequest{Command: "git status --porcelain", Output: " M fixture.go\n"}, telemetry.DecisionProtectedPassthrough, ""},
		{"json", HookRequest{Command: "node", Output: `{"token":"synthetic"}`}, telemetry.DecisionProtectedPassthrough, ""},
		{"diff", HookRequest{Command: "node", Output: "diff --git a/fixture b/fixture\n-a\n+b"}, telemetry.DecisionProtectedPassthrough, ""},
		{"no-parser", HookRequest{Command: "unknown", Output: "token=synthetic"}, telemetry.DecisionUnsupportedParser, ""},
		{"disabled", HookRequest{Command: "git log", Output: hookDefaultGitLog, EnabledParsers: map[string]bool{"git_log": false}}, telemetry.DecisionUnsupportedParser, ""},
		{"unsupported-log", HookRequest{Command: "git log --oneline", Output: "abc1234 subject\n"}, telemetry.DecisionUnsupportedParser, ""},
		{"grep-expansion", HookRequest{Command: "rg pattern fixture", Output: "a:1:x\nb:2:y\nc:3:z\n"}, telemetry.DecisionRejectedNonReduction, ""},
		{"grep-equal-estimate", HookRequest{Command: "rg pattern fixture", Output: "abcd\n"}, telemetry.DecisionRejectedNonReduction, ""},
		{"grep-unicode", HookRequest{Command: "rg pattern fixture", Output: "界界:1:x\n界界:2:y\n"}, telemetry.DecisionRejectedNonReduction, ""},
		{"grep-accepted", HookRequest{Command: "rg pattern fixture", Output: strings.Repeat("synthetic/long/path/file.go:10:match\n", 20)}, telemetry.DecisionTransformed, "grep"},
		{"log-accepted", HookRequest{Command: "git log", Output: hookDefaultGitLog}, telemetry.DecisionTransformed, "git_log"},
		{"parser-marker", HookRequest{Command: "node", Output: strings.Repeat("ordinary output\n", 140)}, telemetry.DecisionTransformed, "node"},
		// Accepted parsing need not reduce bytes: do not add a new reduction guard.
		{"parser-no-op", HookRequest{Command: "node", Output: "ordinary output"}, telemetry.DecisionTransformed, "node"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, storage := isolateHookTelemetry(t)
			req := tc.req
			req.StoragePath = storage
			disabled := OptimizeHook(req)
			req.TelemetryEnabled = true
			got := OptimizeHook(req)
			if got != disabled {
				t.Fatalf("accounting changed response: %#v != %#v", got, disabled)
			}
			if got.Parser != tc.parser || got.Passthrough != (tc.parser == "") {
				t.Fatalf("provenance: %#v", got)
			}
			if tc.parser == "" && got.Output != PiRedact(req.Output) {
				t.Fatalf("passthrough changed: %#v", got)
			}
			if tc.name == "parser-marker" && !got.ParserNetReductionKnown {
				t.Fatal("lost parser marker provenance")
			}
			if tc.name == "log-accepted" && got.Output != "0123456 | Example Author | Mar 15 2026 | A subject" {
				t.Fatalf("log changed: %q", got.Output)
			}
			data, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var fields map[string]any
			if err := json.Unmarshal(data, &fields); err != nil {
				t.Fatal(err)
			}
			if len(fields) != 14 {
				t.Fatalf("response fields changed: %s", data)
			}
			if _, exists := fields["decision_reason"]; exists {
				t.Fatal("reason leaked into hook response")
			}
			tel, err := telemetry.NewTelemetry(storage)
			if err != nil {
				t.Fatal(err)
			}
			defer tel.Close()
			rows, err := tel.GetRecentExecutions(10, "")
			if err != nil || len(rows) != 1 {
				t.Fatalf("records: %#v %v", rows, err)
			}
			rec := rows[0]
			if rec.DecisionReason != tc.reason || rec.ParserUsed != got.Parser || rec.IsPassthrough != got.Passthrough {
				t.Fatalf("accounting: %#v, want %q", rec, tc.reason)
			}
		})
	}
}
