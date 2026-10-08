package parser

import (
	"fmt"
	"strings"
	"testing"
)

func vitestLateFailureFixture() string {
	lines := make([]string, 0, 70)
	for i := 1; i <= 60; i++ {
		lines = append(lines, fmt.Sprintf("routine %03d", i))
	}
	lines = append(lines,
		"FAIL checkout.test.ts > rejects expired session",
		"AssertionError: values differ",
		`Expected: "expired"`,
		`Received: "valid"`,
		"",
		"  at checkout.test.ts:42:7",
		"Test Files  1 failed (1)",
		"Tests  1 failed | 24 passed (25)",
	)
	return strings.Join(lines, "\n") + "\n"
}

func TestVitestParserPreservesLateFailureDiagnostics(t *testing.T) {
	input := vitestLateFailureFixture()
	if got := (&VitestParser{}).Parse(input); got != input {
		t.Fatalf("Vitest failure diagnostics changed; got %q", got)
	}
}

func TestVitestSuccessfulSummaryUnchanged(t *testing.T) {
	input := "✓ checkout.test.ts (25 tests) 118ms\nTest Files  1 passed (1)\nTests  25 passed (25)\nStart at 10:00:00\nDuration 1.2s\n"
	want := "✓ checkout.test.ts (25 tests) 118ms\nTest Files  1 passed (1)\nTests  25 passed (25)\nStart at 10:00:00\nDuration 1.2s"
	if got := (&VitestParser{}).Parse(input); got != want {
		t.Fatalf("successful summary changed: got %q, want %q", got, want)
	}
}
