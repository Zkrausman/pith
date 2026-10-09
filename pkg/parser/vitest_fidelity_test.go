package parser

import (
	"crypto/sha256"
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
	t.Logf("late failure fixture SHA-256: %x", sha256.Sum256([]byte(input)))
	if got := (&VitestParser{}).Parse(input); got != input {
		t.Fatalf("Vitest failure diagnostics changed; got %q", got)
	}
}

func TestVitestParserRecognizesANSIFailOnly(t *testing.T) {
	input := "\x1b[31mFAIL checkout.test.ts > rejects expired session\x1b[0m\nexpected this exact output to survive\n  at checkout.test.ts:42:7\n"
	if got := (&VitestParser{}).Parse(input); got != input {
		t.Fatalf("ANSI-only FAIL diagnostic changed; got %q", got)
	}
}

func TestVitestParserRemovesNumberedFailureDecoration(t *testing.T) {
	heading := "FAIL checkout.test.ts > rejects expired session\n"
	details := "AssertionError: values differ\n"
	for _, tc := range []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "existing one-character tail",
			input: heading + strings.Repeat("⎯", 40) + "[1/3]⎯\n" + details,
			want:  heading + details,
		},
		{
			name:  "spaced long tail",
			input: heading + "⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯ [1/3] ⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯\n" + details,
			want:  heading + details,
		},
		{
			name:  "embedded diagnostic rule text",
			input: heading + `Expected: "⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯ [1/3]⎯"` + "\n" + details,
			want:  heading + `Expected: "⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯ [1/3]⎯"` + "\n" + details,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := (&VitestParser{}).Parse(tc.input); got != tc.want {
				t.Fatalf("numbered decoration handling changed output: got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestVitestParserPreservesRuleCharactersInDiagnostics(t *testing.T) {
	input := vitestLateFailureFixture()
	input = strings.Replace(input, `Expected: "expired"`, `Expected: "expired ⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯ source-value"`, 1)
	input = strings.Replace(input, `Received: "valid"`, `Received: "valid"
  const divider = "⎯⎯⎯⎯⎯⎯⎯⎯⎯⎯";`, 1)
	coloredCRLF := strings.ReplaceAll(input, "\n", "\r\n")
	coloredCRLF = strings.Replace(coloredCRLF, "FAIL checkout.test.ts", "\x1b[31mFAIL checkout.test.ts", 1)
	coloredCRLF = strings.Replace(coloredCRLF, "rejects expired session\r\n", "rejects expired session\x1b[0m\r\n", 1)
	for _, tc := range []struct {
		name  string
		input string
	}{
		{name: "rule characters in values and source excerpt", input: input},
		{name: "ANSI marker and CRLF", input: coloredCRLF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Logf("fixture SHA-256: %x", sha256.Sum256([]byte(tc.input)))
			if got := (&VitestParser{}).Parse(tc.input); got != tc.input {
				t.Fatalf("Vitest diagnostics changed; got %q", got)
			}
		})
	}
}

func TestVitestParserPreservesUnhandledFailureMarkers(t *testing.T) {
	for _, marker := range []string{
		"Unhandled Errors",
		"⎯⎯⎯ Unhandled Errors ⎯⎯⎯",
		"Unhandled Rejection: \"primitive rejection\"",
	} {
		t.Run(marker, func(t *testing.T) {
			lines := make([]string, 0, 62)
			for i := 1; i <= 60; i++ {
				lines = append(lines, fmt.Sprintf("routine %03d", i))
			}
			lines = append(lines,
				marker,
				`"primitive rejection details"`,
				"  at vitest-runner.js:42:7",
				"Test Files  1 passed (1)",
				"Tests  25 passed (25)",
			)
			input := strings.Join(lines, "\n") + "\n"
			t.Logf("fixture SHA-256: %x", sha256.Sum256([]byte(input)))
			if got := (&VitestParser{}).Parse(input); got != input {
				t.Fatalf("Vitest failure marker output changed; got %q", got)
			}
		})
	}
}

func TestVitestFailureMarkerClassification(t *testing.T) {
	for _, tc := range []struct {
		name string
		line string
		want bool
	}{
		{name: "positive singular count", line: "Errors 1 error", want: true},
		{name: "positive plural count", line: "Errors 2 errors", want: true},
		{name: "zero error count", line: "Errors 0 errors", want: false},
		{name: "zero singular count", line: "Errors 0 error", want: false},
		{name: "count with trailing prose", line: "Errors 2 errors were reported", want: false},
		{name: "benign mention", line: "The report documents unhandled errors from an older run", want: false},
		{name: "passed summary", line: "Tests 25 passed (25)", want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := vitestOutputHasFailure([]string{tc.line}); got != tc.want {
				t.Fatalf("vitestOutputHasFailure(%q) = %t, want %t", tc.line, got, tc.want)
			}
		})
	}
}

func TestVitestSuccessfulSummaryUnchanged(t *testing.T) {
	input := "✓ checkout.test.ts (25 tests) 118ms\nTest Files  1 passed (1)\nTests  25 passed (25)\nStart at 10:00:00\nDuration 1.2s\n"
	want := "✓ checkout.test.ts (25 tests) 118ms\nTest Files  1 passed (1)\nTests  25 passed (25)\nStart at 10:00:00\nDuration 1.2s"
	if got := (&VitestParser{}).Parse(input); got != want {
		t.Fatalf("successful summary changed: got %q, want %q", got, want)
	}
}
