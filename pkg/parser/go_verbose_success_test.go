package parser

import (
	"strings"
	"testing"
)

const goVerboseFixture = "=== RUN   TestParent\n" +
	"=== RUN   TestParent/ErrorWord\n" +
	"    --- PASS: TestParent/ErrorWord (0.00s)\n" +
	"=== RUN   TestParent/FailWord\n" +
	"    --- PASS: TestParent/FailWord (17µs)\n" +
	"--- PASS: TestParent (1m2.5s)\n" +
	"=== RUN   TestStandaloneError\n" +
	"--- PASS: TestStandaloneError (0.03s)\n" +
	"PASS\n" +
	"ok  \tpith/pkg/telemetry\t1.206s\n"

const goVerboseFixtureCommand = "/toolchain/go/bin/go test -v ./pkg/telemetry"

func TestValidatedGoVerboseSuccess(t *testing.T) {
	got, ok := ValidatedGoVerboseSuccess(goVerboseFixtureCommand, goVerboseFixture, 0)
	if !ok || got != "PASS\nok  \tpith/pkg/telemetry\t1.206s" {
		t.Fatalf("summary=%q valid=%v", got, ok)
	}
}

func TestValidatedGoVerboseSuccessRejectsAmbiguousInputs(t *testing.T) {
	badOutput := map[string]string{
		"diagnostic":               "=== RUN   TestA\nwarning: fixture\n--- PASS: TestA (0s)\nPASS\nok  pith/pkg 1s\n",
		"diff":                     "=== RUN   TestA\n--- PASS: TestA (0s)\n--- change\nPASS\nok  pith/pkg 1s\n",
		"truncation":               "=== RUN   TestA\n--- PASS: TestA (0s)\n... output truncated by host ...\nPASS\nok  pith/pkg 1s\n",
		"duplicate-run":            "=== RUN   TestA\n=== RUN   TestA\n--- PASS: TestA (0s)\nPASS\nok  pith/pkg 1s\n",
		"duplicate-pass":           "=== RUN   TestA\n--- PASS: TestA (0s)\n--- PASS: TestA (0s)\nPASS\nok  pith/pkg 1s\n",
		"unmatched-pass":           "--- PASS: TestA (0s)\nPASS\nok  pith/pkg 1s\n",
		"skip":                     "=== RUN   TestA\n--- SKIP: TestA (0s)\nPASS\nok  pith/pkg 1s\n",
		"failure":                  "=== RUN   TestA\n--- FAIL: TestA (0s)\nFAIL\nFAIL pith/pkg 0s\n",
		"bad-duration":             "=== RUN   TestA\n--- PASS: TestA (NaNs)\nPASS\nok  pith/pkg 1s\n",
		"overflow-duration":        "=== RUN   TestA\n--- PASS: TestA (999999999999999999999999h)\nPASS\nok  pith/pkg 1s\n",
		"negative-duration":        "=== RUN   TestA\n--- PASS: TestA (-1s)\nPASS\nok  pith/pkg 1s\n",
		"malformed-summary":        "=== RUN   TestA\n--- PASS: TestA (0s)\nPASS\nok pith/pkg 1s\n",
		"malformed-summary-prefix": "=== RUN   TestA\n--- PASS: TestA (0s)\nPASS\nxxok  pith/pkg 1s\n",
		"cached-summary":           "=== RUN   TestA\n--- PASS: TestA (0s)\nPASS\nok  pith/pkg (cached)\n",
		"extra-summary":            "=== RUN   TestA\n--- PASS: TestA (0s)\nPASS\nok  pith/pkg 1s\nok  pith/pkg/other 2s\n",
		"unmatched-run":            "=== RUN   TestA\n=== RUN   TestB\n--- PASS: TestA (0s)\nPASS\nok  pith/pkg 1s\n",
		"trailing-diagnostic":      "=== RUN   TestA\n--- PASS: TestA (0s)\nPASS\nok  pith/pkg 1s\ndiagnostic\n",
		"missing-final-newline":    "=== RUN   TestA\n--- PASS: TestA (0s)\nPASS\nok  pith/pkg 1s",
		"blank-line":               "=== RUN   TestA\n\n--- PASS: TestA (0s)\nPASS\nok  pith/pkg 1s\n",
		"pause":                    "=== RUN   TestA\n=== PAUSE TestA\n--- PASS: TestA (0s)\nPASS\nok  pith/pkg 1s\n",
		"indented-diagnostic":      "=== RUN   TestA\n    warning: fixture\n--- PASS: TestA (0s)\nPASS\nok  pith/pkg 1s\n",
		"malformed-pass-indent":    "=== RUN   TestA\n  --- PASS: TestA (0s)\nPASS\nok  pith/pkg 1s\n",
		"overlong-pass-indent":     "=== RUN   TestA\n" + strings.Repeat(" ", 68) + "--- PASS: TestA (0s)\nPASS\nok  pith/pkg 1s\n",
	}
	for name, output := range badOutput {
		t.Run(name, func(t *testing.T) {
			if got, ok := ValidatedGoVerboseSuccess(goVerboseFixtureCommand, output, 0); ok {
				t.Fatalf("accepted ambiguous output as %q", got)
			}
		})
	}
	for name, command := range map[string]string{
		"nonzero":                  goVerboseFixtureCommand,
		"shell":                    "/toolchain/go/bin/go test -v ./pkg/telemetry && echo done",
		"quoted":                   `"/toolchain with spaces/go" test -v ./pkg/telemetry`,
		"extra-flag":               "/toolchain/go/bin/go test -v -count=1 ./pkg/telemetry",
		"multiple-package":         "/toolchain/go/bin/go test -v ./pkg/...",
		"malformed-package-prefix": "/toolchain/go/bin/go test -v @./pkg/telemetry",
		"relative":                 "go test -v ./pkg/telemetry",
	} {
		t.Run(name, func(t *testing.T) {
			exit := 0
			if name == "nonzero" {
				exit = 1
			}
			if got, ok := ValidatedGoVerboseSuccess(command, goVerboseFixture, exit); ok {
				t.Fatalf("accepted unsupported command as %q", got)
			}
		})
	}
}
