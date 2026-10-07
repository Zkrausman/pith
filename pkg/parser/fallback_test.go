package parser

import (
	"strings"
	"testing"
)

// Select the first match, as the runner and Pi hook do. Instantiating the
// intended parser directly would miss a change in registry precedence.
func fallbackParserFor(t *testing.T, command, wantName string) Parser {
	t.Helper()
	parts := strings.Fields(command)
	for _, candidate := range GetAllParsers() {
		if candidate.CanParse(parts[0], parts[1:]) {
			if candidate.Name() != wantName {
				t.Fatalf("%q selected %q, want %q", command, candidate.Name(), wantName)
			}
			return candidate
		}
	}
	t.Fatalf("no parser selected for %q", command)
	return nil
}

func TestParserFallbackNoInventedSuccess(t *testing.T) {
	commands := []struct {
		command string
		parser  string
	}{
		{"git status", "git_status"},
		{"git status --porcelain=v1", "git_status"},
		{"git add fixture.go", "git_status"},
		{"go tool cover -func missing.cov", "go_cover"},
		{"npm test", "tests"},
		{"npm test --json", "tests"},
		{"go test ./...", "tests"},
		{"go test -json ./...", "tests"},
		{"pytest", "tests"},
	}
	for _, command := range commands {
		t.Run(command.command, func(t *testing.T) {
			p := fallbackParserFor(t, command.command, command.parser)
			for name, output := range map[string]string{
				"empty":          "",
				"whitespace":     " \t\r\n\n\t ",
				"unknown":        "capture unavailable",
				"unknown-bytes":  "  collecting 25 cases\t\r\nconnection closed\n\n",
				"malformed-json": `{"stats":{"tests":25,"passes":23,`,
			} {
				t.Run(name, func(t *testing.T) {
					if got := p.Parse(output); got != output {
						t.Fatalf("unsupported output changed: got %q, want exact %q", got, output)
					}
				})
			}
		})
	}
}

func TestGitStatusFallbackPreservesMachineReadableBytes(t *testing.T) {
	for _, command := range []string{"git status --short", "git status --porcelain=v1", "git status --porcelain=v1 -z"} {
		t.Run(command, func(t *testing.T) {
			p := fallbackParserFor(t, command, "git_status")
			for name, output := range map[string]string{
				"filename-substrings":   "?? nothing to commit.txt\n?? no changes added.txt\n M other.go\n",
				"rename-with-other-row": "R  old.go -> new.go\n M other.go\n",
				"two-column-status":     " M fixture.go\n?? new-file.go\n",
				"nul-separated":         " M fixture.go\x00?? new-file.go\x00",
			} {
				t.Run(name, func(t *testing.T) {
					if got := p.Parse(output); got != output {
						t.Fatalf("machine-readable status changed: got %q, want %q", got, output)
					}
				})
			}
		})
	}
}

func TestGitStatusFallbackPreservesFilteredOnlyOutput(t *testing.T) {
	p := fallbackParserFor(t, "git status", "git_status")
	for name, output := range map[string]string{
		"clean-human-status": "On branch main\nYour branch is up to date with 'origin/main'.\n\nnothing to commit, working tree clean\n",
		"header-only":        "On branch main\r\n\r\n",
		"advice-only":        "  (use \"git add\" to stage files)\n",
	} {
		t.Run(name, func(t *testing.T) {
			if got := p.Parse(output); got != output {
				t.Fatalf("filtered-only output must not become a success claim: got %q, want %q", got, output)
			}
		})
	}
}

func TestTestAndCoverageFallbackPreservesUnrecognizedBytes(t *testing.T) {
	for _, command := range []struct {
		command string
		parser  string
	}{
		{"npm test", "tests"},
		{"go test ./...", "tests"},
		{"pytest", "tests"},
		{"go tool cover -func missing.cov", "go_cover"},
	} {
		t.Run(command.command, func(t *testing.T) {
			p := fallbackParserFor(t, command.command, command.parser)
			for name, output := range map[string]string{
				"unknown-multiline": "collecting 25 cases\nconnection closed\n",
				"spacing-and-crlf":  "  capture unavailable\t\r\n\r\nretained detail\t\r\n",
				"utf8":              "collecting café cases\n未完了\n",
			} {
				t.Run(name, func(t *testing.T) {
					if got := p.Parse(output); got != output {
						t.Fatalf("fallback must preserve every byte: got %q, want %q", got, output)
					}
				})
			}
		})
	}
}

func TestParserFallbackPreservesStructuredLookingOutput(t *testing.T) {
	for _, command := range []struct {
		command string
		parser  string
	}{
		{"git status", "git_status"},
		{"git add fixture.go", "git_status"},
		{"npm test --json", "tests"},
		{"go test -json ./...", "tests"},
		{"pytest", "tests"},
		{"go tool cover -func missing.cov", "go_cover"},
	} {
		t.Run(command.command, func(t *testing.T) {
			p := fallbackParserFor(t, command.command, command.parser)
			for name, output := range map[string]string{
				"valid-string":              "\"TOTAL: 25 passed\"\n",
				"malformed-string":          "\"TOTAL: 25 passed\n",
				"banner-before-json":        "\n> synthetic-fixture test\n> fixture-runner --json\n\n{\n  \"summary\": \"TOTAL: 25 passed\",\n  \"detail\":",
				"malformed-object":          " \t{\n  \"stats\": {\"tests\":25,\"passes\":23,\n",
				"malformed-array":           "\r\n[\n  {\"tests\":25,\"passes\":23},\n",
				"malformed-passing-summary": "{\n  \"summary\": \"TOTAL: 25 passed\",\n  \"detail\":",
				"malformed-failing-summary": "[\n  \"FAIL: checkout\",\n  \"detail\":",
				"malformed-coverage":        "  {\n  \"coverage\": \"50.0%\",\n  \"detail\":\n",
				"valid-summary-object":      "{\n  \"summary\": \"TOTAL: 25 passed\",\n  \"coverage\": \"50.0%\"\n}\n",
				"valid-summary-array":       "[\n  \"FAIL: checkout\",\n  \"coverage: 50.0%\"\n]\n",
			} {
				t.Run(name, func(t *testing.T) {
					if got := p.Parse(output); got != output {
						t.Fatalf("structured-looking bytes must not be interpreted as human evidence: got %q, want %q", got, output)
					}
				})
			}
		})
	}
}

func TestGoToolCoverFallbackWithoutTotal(t *testing.T) {
	p := fallbackParserFor(t, "go tool cover -func fixture.cov", "go_cover")
	// Complete coverage of these two observed rows cannot establish coverage
	// across the entire command's scope when no aggregate was captured.
	output := "example.invalid/fixture/a.go:10:\tFirst\t100.0%\nexample.invalid/fixture/b.go:20:\tSecond\t100.0%\n"
	if got := p.Parse(output); got != output {
		t.Fatalf("all-100%% rows without a total must pass through: got %q, want %q", got, output)
	}
}

func TestParserFallbackRecognizedEvidenceControls(t *testing.T) {
	cases := []struct {
		name, command, parser, output, want string
	}{
		{
			"git-changes", "git status", "git_status",
			"On branch main\nChanges not staged for commit:\n\tmodified: fixture.go\n",
			"modified: fixture.go",
		},
		{
			"jest-counts", "npm test", "tests",
			"Running fixture\nTest Suites: 2 passed, 2 total\nTests:       2 skipped, 23 passed, 25 total\n",
			"Test Suites: 2 passed, 2 total\nTests:       2 skipped, 23 passed, 25 total",
		},
		{
			"go-summary", "go test ./...", "tests",
			"=== RUN TestFixture\n--- PASS: TestFixture\nok  example.invalid/fixture 0.001s\n",
			"ok  example.invalid/fixture 0.001s",
		},
		{
			"pytest-summary", "pytest", "tests",
			"collecting fixture\n23 passed in 0.01s\n",
			"23 passed in 0.01s",
		},
		{
			"coverage-total", "go tool cover -func fixture.cov", "go_cover",
			"fixture.go:10:\tFirst\t100.0%\nfixture.go:20:\tSecond\t50.0%\ntotal:\t(statements)\t75.0%\n",
			"fixture.go:20:\tSecond\t50.0%\ntotal:\t(statements)\t75.0%",
		},
		{
			"complete-coverage-total", "go tool cover -func fixture.cov", "go_cover",
			"fixture.go:10:\tFirst\t100.0%\ntotal:\t(statements)\t100.0%\n",
			"total:\t(statements)\t100.0%",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := fallbackParserFor(t, tc.command, tc.parser)
			if got := p.Parse(tc.output); got != tc.want {
				t.Fatalf("recognized evidence changed: got %q, want %q", got, tc.want)
			}
		})
	}
}
