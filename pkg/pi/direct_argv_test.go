package pi

import (
	"fmt"
	"strings"
	"testing"

	"pith/pkg/parser"
	"pith/pkg/telemetry"
)

func TestOptimizeHookDirectArgvRoutesExistingParserByExecutableBasename(t *testing.T) {
	input := strings.Repeat("synthetic/long/path/file.go:10:match; literal\n", 30)
	got := OptimizeHook(HookRequest{
		Command: "/opt/tools/rg -n 'match; literal' src",
		Argv:    []string{"/opt/tools/rg", "-n", "match; literal", "src"},
		Output:  input,
	})
	if got.Parser != "grep" || got.Passthrough || got.Output == input {
		t.Fatalf("direct rg argv did not reach the existing grep parser: %#v", got)
	}
	if got.OriginalByteCount != len(input) || got.RetainedByteCount != len(got.Output) ||
		got.RetainedByteCount >= got.OriginalByteCount {
		t.Fatalf("direct argv response must report a strict byte reduction: %#v", got)
	}
}

func TestOptimizeHookDirectArgvKnownLossyOverlapsStayRaw(t *testing.T) {
	jsonOutput := "{\n  \"name\": \"fixture\",\n  \"items\": [1, 2, 3]\n}\n"
	for _, tc := range []struct {
		name, command string
		argv          []string
		output        string
	}{
		{name: "ls whitespace filenames", command: "/bin/ls -l", argv: []string{"/bin/ls", "-l"}, output: "-rw-r--r-- 1 user group 10 Jan 01 00:00 name with spaces.txt\n"},
		{name: "source comments", command: "/bin/cat source.go", argv: []string{"/bin/cat", "source.go"}, output: "package fixture\n// comment with quoted text\nconst text = \"http://example.invalid/a\"\n"},
		{name: "git status rename", command: "git -C repo status --short", argv: []string{"git", "-C", "repo", "status", "--short"}, output: "R  destination.txt -> source.txt\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := OptimizeHook(HookRequest{Command: tc.command, Argv: tc.argv, Output: tc.output})
			if got.Output != tc.output || got.Parser != "" || !got.Passthrough {
				t.Fatalf("known lossy overlap changed output or fell through: %#v", got)
			}
		})
	}

	got := OptimizeHook(HookRequest{
		Command: "cat data.json", Argv: []string{"cat", "data.json"}, Output: jsonOutput,
	})
	if got.Parser != "minify" || got.Passthrough || got.Output == jsonOutput {
		t.Fatalf("single JSON read should use only the lossless minify parser: %#v", got)
	}
	if strings.Contains(got.Output, "GetContent") || !strings.Contains(got.Output, `"items":[1,2,3]`) {
		t.Fatalf("JSON read did not preserve its values while compacting whitespace: %q", got.Output)
	}
}

func TestOptimizeHookDirectArgvExcludesSubstringGetContentFallback(t *testing.T) {
	input := strings.Repeat("catalog fixture line\n", 60)
	if (&parser.GetContentParser{}).CanParse("catalog", nil) {
		t.Fatal("GetContentParser must reject incidental command substrings")
	}
	got, record := optimizeHookWithTelemetry(t, HookRequest{Command: "catalog", Argv: []string{"catalog"}, Output: input})
	if got.Output != input || got.Parser != "" || !got.Passthrough ||
		record.DecisionReason != telemetry.DecisionUnsupportedParser {
		t.Fatalf("unrelated direct executable must remain unsupported raw: response=%#v record=%#v", got, record)
	}
}

func TestOptimizeHookDirectGoSummaryUsesActualArgvAndHonorsRawPolicy(t *testing.T) {
	output := "=== RUN   TestStandaloneError\n--- PASS: TestStandaloneError (0.03s)\nPASS\nok  pith/pkg/parser 0.001s\n"
	misleadingCommand := OptimizeHook(HookRequest{
		Command: "/usr/bin/go test -v ./pkg/parser",
		Argv:    []string{"node", "app.js"},
		Output:  output,
	})
	if misleadingCommand.Output != output || misleadingCommand.Parser != "" || !misleadingCommand.Passthrough {
		t.Fatalf("direct interpreter argv must override contradictory command text: %#v", misleadingCommand)
	}

	actualGo := OptimizeHook(HookRequest{
		Command: "node app.js",
		Argv:    []string{"/usr/bin/go", "test", "-v", "./pkg/parser"},
		Output:  output,
	})
	if actualGo.Parser != "tests" || actualGo.Passthrough || actualGo.Output == output {
		t.Fatalf("validated Go summary should follow actual direct argv: %#v", actualGo)
	}
}

func TestOptimizeHookDirectArgvInterpretersAndRecursiveWrappersStayRaw(t *testing.T) {
	cases := []struct {
		name, command string
		argv          []string
		output        string
	}{
		{name: "environment wrapper", command: "env", argv: []string{"env"}, output: strings.Repeat("VISIBLE=value\n", 12) + "malformed fixture\n"},
		{name: "node interpreter", command: "node", argv: []string{"node", "app.js"}, output: strings.Repeat("node fixture line\n", 110)},
		{name: "PowerShell interpreter", command: "powershell", argv: []string{"powershell", "-Command", "Get-ChildItem"}, output: "Directory: fixture\n" + strings.Repeat("Mode                 LastWriteTime         Length Name\n", 50)},
		{name: "recursive Pith command", command: "pith", argv: []string{"pith", "pi", "transform"}, output: strings.Repeat("fixture line\n", 30)},
		{name: "Go run Pith command", command: "go", argv: []string{"go", "run", "main.go"}, output: strings.Repeat("fixture line\n", 30)},
		{name: "find exec Pith command", command: "find", argv: []string{"find", ".", "-exec", "pith", "pi", "transform", "{}", ";"}, output: strings.Repeat("./fixture/path\n", 30)},
		{name: "package launcher Pith command", command: "npm", argv: []string{"npm", "run", "pith"}, output: strings.Repeat("fixture line\n", 30)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, record := optimizeHookWithTelemetry(t, HookRequest{Command: tc.command, Argv: tc.argv, Output: tc.output})
			if got.Output != tc.output || got.Parser != "" || !got.Passthrough ||
				record.DecisionReason != telemetry.DecisionProtectedPassthrough {
				t.Fatalf("interpreter or recursive command was not protected raw: %#v", got)
			}
		})
	}
}

func TestOptimizeHookDirectArgvRetainsLegacyAndOutputGuards(t *testing.T) {
	input := strings.Repeat("synthetic/long/path/file.go:10:match\n", 30)
	cases := []struct {
		name string
		req  HookRequest
	}{
		{name: "legacy quoted shell string", req: HookRequest{Command: "rg -n 'match; literal' src", Output: input}},
		{name: "invalid direct argv", req: HookRequest{Command: "rg -n match src", Argv: []string{"rg", "\x00"}, Output: input}},
		{name: "disabled parser", req: HookRequest{Command: "rg -n match src", Argv: []string{"rg", "-n", "match", "src"}, Output: input, EnabledParsers: map[string]bool{"grep": false}}},
		{name: "nonzero status", req: HookRequest{Command: "rg -n match src", Argv: []string{"rg", "-n", "match", "src"}, Output: input, ExitCode: 2}},
		{name: "raw bypass", req: HookRequest{Command: "rg -n match src", Argv: []string{"rg", "-n", "match", "src"}, Output: input, RawBypass: true}},
		{name: "warning marker", req: HookRequest{Command: "rg -n match src", Argv: []string{"rg", "-n", "match", "src"}, Output: "warning: fixture\n" + input}},
		{name: "upstream truncation", req: HookRequest{Command: "rg -n match src", Argv: []string{"rg", "-n", "match", "src"}, Output: input + "... output truncated by host ...\n"}},
		{name: "structured output", req: HookRequest{Command: "rg -n match src", Argv: []string{"rg", "-n", "match", "src"}, Output: `{"match":true}`}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := OptimizeHook(tc.req)
			if !got.Passthrough || got.Parser != "" {
				t.Fatalf("guard was bypassed: %#v", got)
			}
		})
	}
}

func TestOptimizeHookDirectArgvKeepsMandatoryRedaction(t *testing.T) {
	secret := "token=fixture-secret-value"
	input := strings.Repeat("synthetic/long/path/file.go:10:"+secret+"\n", 30)
	got := OptimizeHook(HookRequest{
		Command: "rg -n token src",
		Argv:    []string{"rg", "-n", "token", "src"},
		Output:  input,
	})
	if got.Parser != "grep" || got.Passthrough || strings.Contains(got.Output, secret) {
		t.Fatalf("direct argv bypassed mandatory redaction: %#v", got)
	}
}

func TestOptimizeHookDirectRawPolicyRecordsProtectedPassthrough(t *testing.T) {
	dir := t.TempDir()
	output := "package fixture\n// comment\n" + strings.Repeat("const line = 1\n", 40)
	got := OptimizeHook(HookRequest{
		Command: "cat source.go", Argv: []string{"cat", "source.go"}, Output: output,
		TelemetryEnabled: true, StoragePath: dir,
	})
	if got.Output != output || !got.Passthrough {
		t.Fatalf("raw source policy changed output: %#v", got)
	}
	tel, err := telemetry.NewTelemetry(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Close()
	records, err := tel.GetRecentExecutions(1, "")
	if err != nil || len(records) != 1 {
		t.Fatalf("execution records: %#v, %v", records, err)
	}
	if records[0].DecisionReason != telemetry.DecisionProtectedPassthrough || !records[0].IsPassthrough || records[0].ParserUsed != "" {
		t.Fatalf("known raw overlap should account as protected passthrough: %#v", records[0])
	}
}

func TestOptimizeHookLegacyIdentityParsersRemainPassthrough(t *testing.T) {
	cases := []struct {
		name, command, output string
		wantReason            telemetry.DecisionReason
	}{
		{name: "source identity with mandatory redaction", command: "cat source.go", output: "package fixture\nconst value = 1\ntoken=fixture-secret-value", wantReason: telemetry.DecisionProtectedPassthrough},
		{name: "ls identity", command: "ls", output: "one-name", wantReason: telemetry.DecisionProtectedPassthrough},
		{name: "Get-Content identity", command: "Get-Content -Path file.txt", output: "one line", wantReason: telemetry.DecisionUnsupportedParser},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, record := optimizeHookWithTelemetry(t, HookRequest{Command: tc.command, Output: tc.output})
			if got.Parser != "" || !got.Passthrough || record.DecisionReason != tc.wantReason {
				t.Fatalf("identity parser must retain raw passthrough provenance: response=%#v record=%#v", got, record)
			}
			if tc.name == "source identity with mandatory redaction" {
				if got.Output == tc.output || strings.Contains(got.Output, "fixture-secret-value") {
					t.Fatalf("identity fallback must retain mandatory redaction: %q", got.Output)
				}
			} else if got.Output != tc.output {
				t.Fatalf("identity parser changed bytes: got %q want %q", got.Output, tc.output)
			}
		})
	}
}

func TestOptimizeHookDirectProtectedAndUnsupportedMatrix(t *testing.T) {
	for _, tc := range []struct {
		name, command string
		argv          []string
		output        string
		protected     bool
	}{
		{name: "git diff", command: "git diff", argv: []string{"git", "diff"}, output: "diff --git a/a b/a\n--- a/a\n+++ b/a\n@@ -1 +1 @@\n-a\n+b\n", protected: true},
		{name: "git status rename arrow", command: "git status --short", argv: []string{"git", "status", "--short"}, output: "R  destination.txt -> source.txt\n", protected: true},
		{name: "git global option rename arrow", command: "git -C repo status --short", argv: []string{"git", "-C", "repo", "status", "--short"}, output: "R  destination.txt -> source.txt\n", protected: true},
		{name: "git status porcelain zero rename", command: "git status -z", argv: []string{"git", "status", "-z"}, output: "R  new\x00old\x00", protected: true},
		{name: "git status porcelain v2 rename", command: "git status --porcelain=v2", argv: []string{"git", "status", "--porcelain=v2"}, output: "# branch.oid abc\n2 R. N... 100644 100644 100644 abc def renamed path\told path\n", protected: true},
		{name: "ls filename whitespace", command: "ls -l", argv: []string{"ls", "-l"}, output: "-rw-r--r-- 1 user group 10 Jan 01 00:00 name with spaces.txt\n", protected: true},
		{name: "source comments", command: "cat source.go", argv: []string{"cat", "source.go"}, output: "package fixture\n// comment with quoted text\nconst text = \"http://example.invalid/a\"\n", protected: true},
		{name: "unsupported git show content", command: "git show HEAD", argv: []string{"git", "show", "HEAD"}, output: "opaque object content\n"},
		{name: "unsupported go version summary", command: "go version", argv: []string{"go", "version"}, output: "go version go1.26.1 linux/amd64"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, record := optimizeHookWithTelemetry(t, HookRequest{Command: tc.command, Argv: tc.argv, Output: tc.output})
			if got.Output != tc.output || got.Parser != "" || !got.Passthrough {
				t.Fatalf("raw disposition changed output: %#v", got)
			}
			if record.DecisionReason != telemetry.DecisionProtectedPassthrough && tc.protected {
				t.Fatalf("expected protected raw, got decision=%q: %#v", record.DecisionReason, got)
			}
			if record.DecisionReason == telemetry.DecisionProtectedPassthrough && !tc.protected {
				t.Fatalf("expected unsupported parser, got protected raw: %#v", got)
			}
		})
	}
}

func optimizeHookWithTelemetry(t *testing.T, request HookRequest) (HookResponse, telemetry.ExecutionRecord) {
	t.Helper()
	dir := t.TempDir()
	request.StoragePath = dir
	request.TelemetryEnabled = true
	response := OptimizeHook(request)
	tel, err := telemetry.NewTelemetry(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Close()
	records, err := tel.GetRecentExecutions(1, "")
	if err != nil || len(records) != 1 {
		t.Fatalf("expected one telemetry record, got %#v, %v", records, err)
	}
	return response, records[0]
}

func TestDirectArgvParserCoverageMatrixMatchesRegistry(t *testing.T) {
	type coverage struct {
		name, command, disposition, policy string
		argv                               []string
		output                             string
		wantReason                         telemetry.DecisionReason
	}
	matrix := []coverage{
		{name: "git_composite", command: "git status; git diff", disposition: "protected_raw", policy: "shell compound stays with Codex", output: "On branch fixture\n M file.go\n", wantReason: telemetry.DecisionProtectedPassthrough},
		{name: "git_status", command: "git status --short", argv: []string{"git", "status", "--short"}, disposition: "protected_raw", policy: "rename output stays raw", output: "R  source.txt -> destination.txt\n", wantReason: telemetry.DecisionProtectedPassthrough},
		{name: "git_log", command: "git log", argv: []string{"git", "log"}, disposition: "routed", policy: "complete default-format records", output: fixtureGitLog()},
		{name: "git_show", command: "git show HEAD", argv: []string{"git", "show", "HEAD"}, disposition: "unsupported", policy: "opaque object content has no safe reduction", output: "opaque object content\n", wantReason: telemetry.DecisionRejectedNonReduction},
		{name: "git_diff", command: "git diff", argv: []string{"git", "diff"}, disposition: "protected_raw", policy: "diff markers bypass minimization", output: "diff --git a/a.go b/a.go\n--- a/a.go\n+++ b/a.go\n@@ -1 +1 @@\n-a\n+b\n", wantReason: telemetry.DecisionProtectedPassthrough},
		{name: "git_branch", command: "git branch", argv: []string{"git", "branch"}, disposition: "routed", policy: "branch inventory summary", output: fixtureBranches()},
		{name: "ls", command: "ls -l", argv: []string{"ls", "-l"}, disposition: "protected_raw", policy: "filename whitespace is ambiguous", output: "-rw-r--r-- 1 user group 10 Jan 01 00:00 name with spaces.txt\n", wantReason: telemetry.DecisionProtectedPassthrough},
		{name: "find", command: "find .", argv: []string{"find", "."}, disposition: "routed", policy: "bounded result summary", output: fixtureLines("./fixture/long/result/path/file", 70)},
		{name: "tree", command: "tree", argv: []string{"tree"}, disposition: "routed", policy: "tree glyph compaction", output: fixtureTree()},
		{name: "du", command: "du -h", argv: []string{"du", "-h"}, disposition: "routed", policy: "well-formed records above the row limit", output: fixtureDU()},
		{name: "grep", command: "rg -n needle src", argv: []string{"rg", "-n", "needle", "src"}, disposition: "routed", policy: "only strict reductions are selected", output: strings.Repeat("synthetic/long/path/file.go:10:needle; literal\n", 30)},
		{name: "minify", command: "cat data.json", argv: []string{"cat", "data.json"}, disposition: "routed", policy: "one JSON file only", output: "{\n  \"name\": \"fixture\",\n  \"items\": [1, 2, 3, 4]\n}\n"},
		{name: "source", command: "cat source.go", argv: []string{"cat", "source.go"}, disposition: "protected_raw", policy: "avoid quoted-comment stripping", output: fixtureLines("const fixture = 1 // ordinary source comment", 30), wantReason: telemetry.DecisionProtectedPassthrough},
		{name: "env", command: "env", argv: []string{"env"}, disposition: "protected_raw", policy: "recursive environment wrapper", output: fixtureEnv(), wantReason: telemetry.DecisionProtectedPassthrough},
		{name: "docker_ps", command: "docker ps", argv: []string{"docker", "ps"}, disposition: "routed", policy: "container inventory", output: fixtureDocker()},
		{name: "gh_release", command: "gh release list", argv: []string{"gh", "release", "list"}, disposition: "routed", policy: "release table", output: fixtureRelease()},
		{name: "github", command: "gh status", argv: []string{"gh", "status"}, disposition: "routed", policy: "status result", output: fixtureLongStatus()},
		{name: "dependencies", command: "npm list", argv: []string{"npm", "list"}, disposition: "routed", policy: "dependency tree", output: fixtureDependencies()},
		{name: "tests", command: "/usr/bin/go test -v ./pkg/parser", argv: []string{"/usr/bin/go", "test", "-v", "./pkg/parser"}, disposition: "routed", policy: "validated Go success summary; final generic summaries remain raw", output: fixtureGoTest()},
		{name: "go_cover", command: "go tool cover -func coverage.out", argv: []string{"go", "tool", "cover", "-func", "coverage.out"}, disposition: "routed", policy: "recognized coverage profile", output: fixtureCoverage()},
		{name: "web-content", command: "curl -s https://example.invalid/data", argv: []string{"curl", "-s", "https://example.invalid/data"}, disposition: "routed", policy: "plain text or HTML summary; JSON stays structured", output: strings.Repeat("Fixture page content with enough words to provide a bounded synthetic response. ", 30)},
		{name: "pith-internal", command: "pith raw", argv: []string{"pith", "raw"}, disposition: "protected_raw", policy: "recursive Pith execution", output: fixtureLines("synthetic Pith output row", 30), wantReason: telemetry.DecisionProtectedPassthrough},
		{name: "windows-shell", command: "powershell -Command Get-ChildItem", argv: []string{"powershell", "-Command", "Get-ChildItem"}, disposition: "protected_raw", policy: "interpreter stays with Codex", output: "Directory: fixture\nMode LastWriteTime Length Name\n" + fixtureLines("-a---- 2026-01-01 12:00 1 fixture.txt", 45), wantReason: telemetry.DecisionProtectedPassthrough},
		{name: "get-content", command: "cat source.go", argv: []string{"cat", "source.go"}, disposition: "protected_raw", policy: "only direct JSON minification is eligible", output: fixtureLines("package fixture; const value = 1", 40), wantReason: telemetry.DecisionProtectedPassthrough},
		{name: "go", command: "go version", argv: []string{"go", "version"}, disposition: "unsupported", policy: "short version output has no reduction", output: "go version go1.26.1 linux/amd64", wantReason: telemetry.DecisionRejectedNonReduction},
		{name: "vitest", command: "vitest run", argv: []string{"vitest", "run"}, disposition: "routed", policy: "bounded success summary", output: fixtureLines("synthetic vitest progress row", 60)},
		{name: "bd", command: "bd list", argv: []string{"bd", "list"}, disposition: "routed", policy: "recognized issue list", output: fixtureIssues()},
		{name: "promptfoo", command: "promptfoo eval", argv: []string{"promptfoo", "eval"}, disposition: "routed", policy: "recognized evaluation summary", output: fixtureLines("synthetic evaluation record", 50)},
		{name: "snag", command: "snag find", argv: []string{"snag", "find"}, disposition: "routed", policy: "bounded search results", output: fixtureLines("synthetic snag search result with a long path and description", 60)},
		{name: "node", command: "node app.js", argv: []string{"node", "app.js"}, disposition: "protected_raw", policy: "interpreter stays with Codex", output: fixtureLines("synthetic node output row", 60), wantReason: telemetry.DecisionProtectedPassthrough},
		{name: "npm", command: "npm run build", argv: []string{"npm", "run", "build"}, disposition: "routed", policy: "recognized package manager output", output: fixtureLines("synthetic package build step", 55)},
		{name: "chain", command: "git status; git diff", disposition: "protected_raw", policy: "shell compound stays with Codex", output: "On branch fixture\n M file.go\n", wantReason: telemetry.DecisionProtectedPassthrough},
	}
	registered := parser.GetAllParsers()
	if len(matrix) != len(registered) {
		t.Fatalf("coverage matrix has %d rows for %d registered parsers", len(matrix), len(registered))
	}
	byName := make(map[string]parser.Parser, len(registered))
	for _, candidate := range registered {
		byName[candidate.Name()] = candidate
	}
	seen := make(map[string]bool, len(matrix))
	for _, row := range matrix {
		if seen[row.name] {
			t.Errorf("coverage matrix repeats parser %q", row.name)
		}
		seen[row.name] = true
		t.Run(row.name, func(t *testing.T) {
			candidate, ok := byName[row.name]
			if !ok {
				t.Fatalf("coverage matrix parser %q is missing from registry", row.name)
			}
			commandName, args := row.command, []string(nil)
			if len(row.argv) > 0 {
				commandName, args = row.argv[0], row.argv[1:]
			}
			if !candidate.CanParse(commandName, args) {
				t.Fatalf("matrix invocation does not match %q: %q %#v (%s)", row.name, commandName, args, row.policy)
			}
			if row.policy == "" {
				t.Fatalf("parser %q lacks a routing or raw-policy note", row.name)
			}
			if row.disposition != "routed" && row.disposition != "protected_raw" && row.disposition != "unsupported" {
				t.Fatalf("parser %q has invalid matrix disposition %q", row.name, row.disposition)
			}
			response, record := optimizeHookWithTelemetry(t, HookRequest{
				Command: row.command, Argv: row.argv, Output: row.output,
			})
			strictReduction := response.RetainedByteCount < response.OriginalByteCount
			t.Logf("expected=%s actual-parser=%q decision=%q passthrough=%t strict-byte-reduction=%t", row.disposition, response.Parser, record.DecisionReason, response.Passthrough, strictReduction)
			switch row.disposition {
			case "routed":
				if response.Parser != row.name || record.ParserUsed != row.name || response.Passthrough ||
					response.Output == row.output || !strictReduction ||
					record.DecisionReason != telemetry.DecisionTransformed || record.IsPassthrough {
					t.Errorf("did not route and reduce through selected parser (%s): response=%#v record=%#v", row.policy, response, record)
				}
			case "protected_raw", "unsupported":
				if response.Output != row.output || response.Parser != "" || !response.Passthrough ||
					record.ParserUsed != "" || !record.IsPassthrough || record.DecisionReason != row.wantReason {
					t.Errorf("did not preserve expected raw disposition (%s): response=%#v record=%#v", row.policy, response, record)
				}
			}
		})
	}
	for _, candidate := range registered {
		if !seen[candidate.Name()] {
			t.Errorf("registered parser %q is absent from coverage matrix", candidate.Name())
		}
	}
}

func TestOptimizeHookDirectArgvParserPriorityDoesNotFallThrough(t *testing.T) {
	jsonOutput := "{\n  \"name\": \"fixture\",\n  \"items\": [1, 2, 3]\n}\n"
	minified := OptimizeHook(HookRequest{
		Command: "cat data.json", Argv: []string{"cat", "data.json"}, Output: jsonOutput,
	})
	if minified.Parser != "minify" || minified.Passthrough || minified.Output == jsonOutput {
		t.Fatalf("single-file JSON overlap should select only minify: %#v", minified)
	}

	npmOutput := fixtureLines("synthetic package runner line", 55)
	npmTest, npmRecord := optimizeHookWithTelemetry(t, HookRequest{
		Command: "npm test", Argv: []string{"npm", "test"}, Output: npmOutput,
	})
	if npmTest.Output != npmOutput || npmTest.Parser != "" || !npmTest.Passthrough ||
		npmRecord.DecisionReason != telemetry.DecisionRejectedNonReduction {
		t.Fatalf("npm test's first matching tests parser should not fall through to npm: response=%#v record=%#v", npmTest, npmRecord)
	}

	catalogOutput := fixtureLines("catalog fixture content", 60)
	catalog, catalogRecord := optimizeHookWithTelemetry(t, HookRequest{
		Command: "catalog", Argv: []string{"catalog"}, Output: catalogOutput,
	})
	if catalog.Output != catalogOutput || catalog.Parser != "" || !catalog.Passthrough ||
		catalogRecord.DecisionReason != telemetry.DecisionUnsupportedParser {
		t.Fatalf("direct catalog must not fall through to GetContentParser: response=%#v record=%#v", catalog, catalogRecord)
	}
}

func fixtureLines(prefix string, count int) string {
	var output strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&output, "%s %03d\n", prefix, i)
	}
	return output.String()
}

func fixtureGitLog() string {
	var output strings.Builder
	for i := 0; i < 4; i++ {
		if i > 0 {
			output.WriteByte('\n')
		}
		fmt.Fprintf(&output,
			"commit %040x\nAuthor: Fixture %d <fixture%d@example.invalid>\nDate:   Mon Jan  2 15:04:05 2006 +0000\n\n    Synthetic subject %d\n",
			i+1, i, i, i,
		)
	}
	return output.String()
}

func fixtureBranches() string {
	return "* main\n" + fixtureLines("  fixture-branch", 45)
}

func fixtureTree() string {
	return fixtureLines("├── very-long-tree-entry-name/child-file.txt", 45)
}

func fixtureDU() string {
	var output strings.Builder
	for i := 0; i < 35; i++ {
		fmt.Fprintf(&output, "1.0M\t/fixture/very/long/path/file-%03d.dat\n", i)
	}
	return output.String()
}

func fixtureEnv() string {
	return fixtureLines("FIXTURE_VALUE=value", 35)
}

func fixtureDocker() string {
	return "CONTAINER ID IMAGE COMMAND CREATED STATUS PORTS NAMES\n" +
		fixtureLines("abcdef012345 image:fixture command 2-hours-ago Up-2-hours port-map container-name", 30)
}

func fixtureRelease() string {
	var output strings.Builder
	output.WriteString("Tag: v1.2.3\nTitle: Synthetic release\nAssets\nNAME DIGEST SIZE\n")
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&output, "fixture-asset-%02d sha256:%064x %d MiB\n", i, i+1, i+1)
	}
	return output.String()
}

func fixtureLongStatus() string {
	return fixtureLines("Synthetic status column with additional fixed-width details to exercise row trimming", 115)
}

func fixtureDependencies() string {
	return "fixture-root@1.0.0\n" + fixtureLines("├── fixture-dependency-with-long-name@1.2.3", 45)
}

func fixtureGoTest() string {
	return "=== RUN   TestAlpha\n--- PASS: TestAlpha (0.00s)\n" +
		"=== RUN   TestBeta\n--- PASS: TestBeta (0.01s)\nPASS\nok  pith/pkg/parser 0.001s\n"
}

func fixtureCoverage() string {
	var output strings.Builder
	for i := 0; i < 40; i++ {
		fmt.Fprintf(&output, "fixture.go:%d: Function%03d\t%d.0%%\n", i+1, i, (i%9)+10)
	}
	output.WriteString("total: (statements) 42.0%\n")
	return output.String()
}

func fixtureIssues() string {
	var output strings.Builder
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&output, "○ fixture-%03d ● P1 Synthetic issue title\n", i)
	}
	return output.String()
}
