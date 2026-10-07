package pi

import (
	"fmt"
	"strings"
	"testing"

	"pith/pkg/parser"
)

func assertFallbackPassthrough(t *testing.T, req HookRequest, wantOutput string) {
	t.Helper()
	countLines := func(s string) int {
		if s == "" {
			return 0
		}
		return strings.Count(s, "\n") + 1
	}
	want := HookResponse{
		Output:               wantOutput,
		Passthrough:          true,
		OriginalLineCount:    countLines(req.Output),
		RetainedLineCount:    countLines(wantOutput),
		OriginalByteCount:    len(req.Output),
		RetainedByteCount:    len(wantOutput),
		MinimizationStrategy: "passthrough",
	}
	if got := OptimizeHook(req); got != want {
		t.Fatalf("got %#v, want exact passthrough provenance %#v", got, want)
	}
}

func TestOptimizeHookFallbackNoInventedSuccess(t *testing.T) {
	for _, command := range []string{
		"git status", "git status --porcelain=v1", "git add fixture.go",
		"go tool cover -func missing.cov", "npm test", "npm test --json",
		"go test ./...", "go test -json ./...", "pytest",
	} {
		t.Run(command, func(t *testing.T) {
			for name, output := range map[string]string{
				"empty":          "",
				"whitespace":     " \t\r\n\n\t ",
				"unknown":        "  collecting 25 cases\t\r\nconnection closed\n\n",
				"malformed-json": " \t{\n  \"stats\": {\"tests\":25,\"passes\":23,\n",
			} {
				for _, exitCode := range []int{0, 1, 2} {
					t.Run(fmt.Sprintf("%s/exit-%d", name, exitCode), func(t *testing.T) {
						assertFallbackPassthrough(t, HookRequest{Command: command, Output: output, ExitCode: exitCode}, output)
					})
				}
			}
		})
	}
}

func TestOptimizeHookFallbackPreservesStructuredLookingEvidence(t *testing.T) {
	for _, command := range []string{"git status", "npm test --json", "go test -json ./...", "pytest", "go tool cover -func missing.cov"} {
		t.Run(command, func(t *testing.T) {
			for name, output := range map[string]string{
				"valid-string":              "\"TOTAL: 25 passed\"\n",
				"malformed-string":          "\"TOTAL: 25 passed\n",
				"banner-before-json":        "\n> synthetic-fixture test\n> fixture-runner --json\n\n{\n  \"summary\": \"TOTAL: 25 passed\",\n  \"detail\":",
				"malformed-passing-summary": "{\n  \"summary\": \"TOTAL: 25 passed\",\n  \"detail\":",
				"malformed-failing-summary": "[\n  \"FAIL: checkout\",\n  \"detail\":",
				"malformed-coverage":        "  {\n  \"coverage\": \"50.0%\",\n  \"detail\":\n",
				"valid-summary-object":      "{\n  \"summary\": \"TOTAL: 25 passed\",\n  \"coverage\": \"50.0%\"\n}\n",
				"valid-summary-array":       "[\n  \"FAIL: checkout\",\n  \"coverage: 50.0%\"\n]\n",
			} {
				t.Run(name, func(t *testing.T) {
					assertFallbackPassthrough(t, HookRequest{Command: command, Output: output}, output)
				})
			}
		})
	}
}

func TestOptimizeHookFallbackRedactsWithoutClaimingTransformation(t *testing.T) {
	for _, command := range []string{"git status", "git add fixture.go", "npm test", "go test ./...", "pytest", "go tool cover -func missing.cov"} {
		t.Run(command, func(t *testing.T) {
			for _, tc := range []struct {
				name, output, want string
			}{
				{"unknown", "  token=synthetic-fixture-only\r\nconnection closed\n", "  token=[REDACTED]\r\nconnection closed\n"},
				{"malformed-json", "{\n  \"token\": \"synthetic-fixture-only\",\n  \"stats\":", "{\n  \"token\": \"[REDACTED]\",\n  \"stats\":"},
			} {
				t.Run(tc.name, func(t *testing.T) {
					assertFallbackPassthrough(t, HookRequest{Command: command, Output: tc.output}, tc.want)
				})
			}
		})
	}
}

func TestOptimizeHookFallbackDoesNotInventCoverageTotal(t *testing.T) {
	output := "fixture.go:10:\tFirst\t100.0%\nfixture.go:20:\tSecond\t100.0%\n"
	assertFallbackPassthrough(t, HookRequest{Command: "go tool cover -func fixture.cov", Output: output}, output)
}

func TestOptimizeHookFallbackRecognizedEvidenceControls(t *testing.T) {
	for _, tc := range []struct {
		name, command, parser, output, want string
	}{
		{"git", "git status", "git_status", "On branch main\n\tmodified: fixture.go\n", "modified: fixture.go"},
		{"tests", "npm test", "tests", "running fixture\nTOTAL: 25\n", "TOTAL: 25"},
		{"pytest", "pytest", "tests", "collecting fixture\n23 passed in 0.01s\n", "23 passed in 0.01s"},
		{"coverage", "go tool cover -func fixture.cov", "go_cover", "fixture.go:10:\tFirst\t100.0%\nfixture.go:20:\tSecond\t50.0%\ntotal:\t(statements)\t75.0%\n", "fixture.go:20:\tSecond\t50.0%\ntotal:\t(statements)\t75.0%"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := HookRequest{Command: tc.command, Output: tc.output}
			got := OptimizeHook(req)
			if got.Output != tc.want || got.Parser != tc.parser || got.Passthrough || got.MinimizationStrategy != "parser:"+tc.parser {
				t.Fatalf("recognized human evidence must still use its parser: %#v", got)
			}
			// Recognition cannot bypass the existing failure, raw, or parser
			// configuration guards, even when the text appears successful.
			t.Run("nonzero", func(t *testing.T) {
				req := req
				req.ExitCode = 2
				assertFallbackPassthrough(t, req, tc.output)
			})
			t.Run("raw", func(t *testing.T) {
				req := req
				req.RawBypass = true
				assertFallbackPassthrough(t, req, tc.output)
			})
			t.Run("disabled", func(t *testing.T) {
				req := req
				req.EnabledParsers = map[string]bool{}
				for _, candidate := range parser.GetAllParsers() {
					req.EnabledParsers[candidate.Name()] = false
				}
				assertFallbackPassthrough(t, req, tc.output)
			})
		})
	}
	for name, output := range map[string]string{
		"go-final-counts":   "=== RUN TestFixture\n--- PASS: TestFixture\nok  example.invalid/fixture 0.001s\n",
		"final-test-counts": "Test Suites: 2 passed, 2 total\nTests:       2 skipped, 23 passed, 25 total\n",
		"failure":           "FAIL: checkout\nError: expected value\n\nretained detail\n",
		"warning":           "warning: capture incomplete\nTOTAL: 25\n",
	} {
		t.Run(name, func(t *testing.T) {
			assertFallbackPassthrough(t, HookRequest{Command: "npm test", Output: output}, output)
		})
	}
}
