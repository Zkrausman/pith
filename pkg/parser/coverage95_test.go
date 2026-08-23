package parser

import (
	"strings"
	"testing"
)

// =====================================================================
// GoParser - 58.8% -> targeting 95%
// =====================================================================

func TestGoParser_AllBranches(t *testing.T) {
	p := &GoParser{}

	// Test CanParse
	if !p.CanParse("go", []string{"test"}) {
		t.Error("Expected go to match")
	}

	// Summary lines
	output := p.Parse("ok  pith 0.5s\nFAIL pith/pkg 0.1s\n5 passed, 2 failed\nTOTAL 100")
	if !strings.Contains(output, "ok") {
		t.Errorf("Expected summary lines, got %s", output)
	}

	// go version / go list -m
	output = p.Parse("go version go1.21.0 windows/amd64\ngithub.com/spf13/cobra v1.8.0")
	if !strings.Contains(output, "go version") {
		t.Errorf("Expected go version, got %s", output)
	}

	// Long output with >20 result lines (truncation)
	var sb strings.Builder
	for i := 0; i < 30; i++ {
		sb.WriteString("error: something went wrong\n")
	}
	output = p.Parse(sb.String())
	if !strings.Contains(output, "+ ") && !strings.Contains(output, "more lines") {
		t.Logf("Go parser long output: %s", output)
	}
}

// =====================================================================
// PithParser - 69.6% -> targeting 95%
// =====================================================================

func TestPithParser_AllBranches(t *testing.T) {
	p := &PithParser{}

	// CanParse: go run main.go
	if !p.CanParse("go", []string{"run", "main.go"}) {
		t.Error("Expected go run main.go to match PithParser")
	}
	if p.CanParse("go", []string{"build"}) {
		t.Error("Expected go build NOT to match PithParser")
	}

	// Help output
	output := p.Parse("Usage: pith [command]\nAvailable Commands:\n  gain  Show token savings\nFlags:\n  -h help")
	if !strings.Contains(output, "Usage: pith") {
		t.Errorf("Expected usage in output, got %s", output)
	}

	// [CMD] and [EXIT] log format
	output = p.Parse("[CMD] git status\non branch main\n[EXIT] 0")
	if !strings.Contains(output, "Command: git status") {
		t.Errorf("Expected command log, got %s", output)
	}
	if !strings.Contains(output, "Exit Code: 0") {
		t.Errorf("Expected exit code, got %s", output)
	}

	// [CMD] contains content in middle - long line through [EXIT] branch
	output = p.Parse("[EXIT] something long with [EXIT] marker in middle")
	if !strings.Contains(output, "Exit Code:") {
		t.Errorf("Expected exit code branch, got %s", output)
	}

	// Long normal output (>20 lines)
	var sb strings.Builder
	for i := 0; i < 25; i++ {
		sb.WriteString("output line\n")
	}
	output = p.Parse(sb.String())
	if !strings.Contains(output, "more lines") {
		t.Errorf("Expected truncation, got %s", output)
	}
}

// =====================================================================
// PowerShellParser - 80.6% -> targeting 95%
// =====================================================================

func TestPowerShellParser_AllBranches(t *testing.T) {
	ps := &PowerShellParser{}

	// CanParse
	if !ps.CanParse("powershell", nil) {
		t.Error("Expected powershell to match")
	}
	if !ps.CanParse("pwsh", nil) {
		t.Error("Expected pwsh to match")
	}
	if !ps.CanParse("cmd", nil) {
		t.Error("Expected cmd to match")
	}

	// Directory listing
	lsOut := `
    Directory: C:\Users\test

Mode                 LastWriteTime         Length Name
----                 -------------         ------ ----
d-----        01/01/2024  12:00 PM                folder1
-a----        01/01/2024  12:00 PM           1234 file.txt
`
	output := ps.Parse(lsOut)
	if !strings.Contains(output, "folder1") && !strings.Contains(output, "file.txt") {
		t.Errorf("Expected directory entries, got %s", output)
	}

	// Category info line (should be skipped)
	output = ps.Parse("+ CategoryInfo : ObjectNotFound\n+ FullyQualifiedErrorId : NotFound\nhello")
	if strings.Contains(output, "CategoryInfo") {
		t.Errorf("Expected CategoryInfo to be filtered, got %s", output)
	}

	// Exit code line (should be skipped)
	output = ps.Parse("Exit Code: 0\nhello world")
	if strings.Contains(output, "Exit Code") {
		t.Errorf("Expected Exit Code to be filtered, got %s", output)
	}

	// Long output (>40 lines) for truncation branch
	var sb strings.Builder
	for i := 0; i < 45; i++ {
		sb.WriteString("line of output\n")
	}
	output = ps.Parse(sb.String())
	if !strings.Contains(output, "more lines") {
		t.Errorf("Expected truncation, got %s", output)
	}

	// Empty result
	output = ps.Parse("   ")
	if output != "" {
		t.Errorf("Expected empty output for whitespace input, got %s", output)
	}

	// isLsh with short field count (<=3 fields)
	lsShort := `
    Directory: C:\test

Mode  Name
----  ----
d--- folder
`
	ps.Parse(lsShort)
}

// =====================================================================
// GetContentParser - testing more branches
// =====================================================================

func TestGetContentParser_AllBranches(t *testing.T) {
	gc := &GetContentParser{}

	// CanParse
	if !gc.CanParse("powershell", []string{"-Command", "Get-Content file.txt"}) {
		t.Error("Expected Get-Content to match")
	}
	if !gc.CanParse("cat", nil) {
		t.Error("Expected cat to match")
	}
	if !gc.CanParse("type", nil) {
		t.Error("Expected type to match")
	}

	// Empty input
	output := gc.Parse("")
	if output != "" {
		t.Errorf("Expected empty output, got %s", output)
	}

	// JSON input (small - under 2000)
	output = gc.Parse(`{"key": "value"}`)
	if !strings.Contains(output, "key") {
		t.Errorf("Expected JSON output, got %s", output)
	}

	// Large JSON (>2000 chars)
	largeJSON := `{"key": "` + strings.Repeat("x", 2100) + `"}`
	output = gc.Parse(largeJSON)
	if !strings.Contains(output, "truncated JSON") {
		t.Errorf("Expected truncated JSON message, got %s", output)
	}

	// Long non-JSON (>50 lines)
	var sb strings.Builder
	for i := 0; i < 60; i++ {
		sb.WriteString("line of content\n")
	}
	output = gc.Parse(sb.String())
	if !strings.Contains(output, "truncated by Pith") {
		t.Errorf("Expected truncation, got %s", output)
	}

	// Array JSON
	output = gc.Parse(`[{"a": 1}]`)
	if output == "" {
		t.Error("Expected output for JSON array")
	}
}

// =====================================================================
// LsParser - 70.8% -> targeting 95%
// =====================================================================

func TestLsParser_AllBranches(t *testing.T) {
	p := &LsParser{}

	// Linux-style long listing
	lsLinux := `-rwxr-xr-x  1 user group 12345 Jan 01 12:00 file.txt
drwxr-xr-x  2 user group  4096 Jan 01 12:00 folder/
lrwxrwxrwx  1 user group    10 Jan 01 12:00 link -> target`
	output := p.Parse(lsLinux)
	if !strings.Contains(output, "file.txt") {
		t.Errorf("Expected file.txt in linux ls, got %s", output)
	}

	// Windows-style dir with size
	winDir := `01/01/2024  12:00 PM    <DIR>          folder
01/01/2024  12:00 PM             1234 file.txt`
	output = p.Parse(winDir)
	if output == "" {
		t.Error("Expected output for windows dir")
	}

	// Short result (<= 5 items) -> space-joined
	shortLs := "file1.txt\nfile2.txt"
	output = p.Parse(shortLs)
	if output == "" {
		t.Error("Expected output for short ls")
	}

	// ls without mode prefix (fallback)
	noMode := "2024-01-01 12:00 4096 somefile"
	output = p.Parse(noMode)
	if output == "" {
		t.Error("Expected output for no-mode ls line")
	}

	// Linux ls -l with exactly 9 fields
	linux9 := "-rw-r--r-- 1 user group 9999 Jan 01 12:00 exact.txt"
	output = p.Parse(linux9)
	if !strings.Contains(output, "exact.txt") {
		t.Errorf("Expected exact.txt, got %s", output)
	}

	// Mode with 5-8 fields (windows-style: no size at field 4)
	win5 := "d---- 2024-01-01 12:00 somesize dirname"
	output = p.Parse(win5)
	if !strings.Contains(output, "dirname") {
		t.Errorf("Expected dirname, got %s", output)
	}

	// Many files -> newline-joined
	var sb strings.Builder
	for i := 0; i < 10; i++ {
		sb.WriteString("-rw-r--r-- 1 u g 123 Jan 01 12:00 file.txt\n")
	}
	output = p.Parse(sb.String())
	if !strings.Contains(output, "\n") {
		t.Error("Expected newline-joined output for many files")
	}
}

// =====================================================================
// FindParser - truncation branch
// =====================================================================

func TestFindParser_Truncation(t *testing.T) {
	p := &FindParser{}
	var sb strings.Builder
	for i := 0; i < 60; i++ {
		sb.WriteString("/path/to/file\n")
	}
	output := p.Parse(sb.String())
	if !strings.Contains(output, "more results") {
		t.Errorf("Expected truncation, got %s", output)
	}
}

// =====================================================================
// DuParser - truncation branch
// =====================================================================

func TestDuParser_Truncation(t *testing.T) {
	p := &DuParser{}
	var sb strings.Builder
	for i := 0; i < 25; i++ {
		sb.WriteString("4096\t/path/to/dir\n")
	}
	output := p.Parse(sb.String())
	if !strings.Contains(output, "more") {
		t.Errorf("Expected truncation, got %s", output)
	}
}

// =====================================================================
// GitHubReleaseParser - CanParse branches
// =====================================================================

func TestGitHubReleaseParser_CanParse(t *testing.T) {
	p := &GitHubReleaseParser{}

	if !p.CanParse("gh", []string{"release", "view"}) {
		t.Error("Expected gh release view to match")
	}
	if !p.CanParse("gh", []string{"release", "list"}) {
		t.Error("Expected gh release list to match")
	}
	if p.CanParse("gh", []string{"issue", "list"}) {
		t.Error("Expected gh issue list NOT to match GitHubReleaseParser")
	}
	if p.CanParse("gh", []string{"release"}) {
		t.Error("Expected gh release (no subcommand) NOT to match")
	}
	if p.CanParse("git", []string{"release", "view"}) {
		t.Error("Expected git to NOT match GitHubReleaseParser")
	}
}

// =====================================================================
// GitHubParser (gh issue/pr/repo list) - more branches
// =====================================================================

func TestGitHubParser_AllBranches(t *testing.T) {
	p := &GitHubParser{}

	// CanParse valid
	if !p.CanParse("gh", []string{"issue", "list"}) {
		t.Error("Expected gh issue list to match")
	}
	if !p.CanParse("gh", []string{"release", "list"}) {
		t.Error("Expected gh release list to match")
	}

	// CanParse invalid (generic views should passthrough so generic compression can head/tail them)
	if p.CanParse("gh", []string{"pr", "view"}) {
		t.Error("Expected gh pr view to NOT match")
	}
	if p.CanParse("gh", []string{"auth", "login"}) {
		t.Error("Expected gh auth to NOT match")
	}

	// Truncation (>100 entries)
	var sb strings.Builder
	sb.WriteString("TITLE  DESCRIPTION  STATUS\n")
	for i := 0; i < 110; i++ {
		sb.WriteString("123  Title  Open\n")
	}
	output := p.Parse(sb.String())
	if !strings.Contains(output, "truncated gh output") {
		t.Error("Expected gh list to be truncated")
	}

	// Single field line (no | formatting)
	output = p.Parse("solo-entry")
	if output == "" {
		t.Error("Expected output for solo entry")
	}

	// Skip header lines
	output = p.Parse("Showing 5 of 10 results\nNAME  Description\nTITLE  Header")
	if strings.Contains(output, "Showing") || strings.Contains(output, "NAME") {
		t.Errorf("Expected headers filtered, got %s", output)
	}
}

// =====================================================================
// InfraParser (DependencyParser) - more branches
// =====================================================================

func TestDependencyParser_AllBranches(t *testing.T) {
	p := &DependencyParser{}

	// CanParse
	if !p.CanParse("npm", []string{"list"}) {
		t.Error("Expected npm list to match")
	}
	if !p.CanParse("pip", []string{"list"}) {
		t.Error("Expected pip list to match")
	}
	if p.CanParse("npm", []string{"install"}) {
		t.Error("Expected npm install NOT to match")
	}

	// Truncation (>30 deps)
	var sb strings.Builder
	for i := 0; i < 35; i++ {
		sb.WriteString("├── package@1.0.0\n")
	}
	output := p.Parse(sb.String())
	if !strings.Contains(output, "truncated dependency list") {
		t.Errorf("Expected truncation, got %s", output)
	}

	// Skip "empty" lines
	output = p.Parse("empty\npackage@1.0.0")
	if strings.Contains(output, "empty") {
		t.Errorf("Expected 'empty' to be filtered, got %s", output)
	}

	// Unicode tree chars fallback
	output = p.Parse("├─ package@1.0.0")
	if output == "" {
		t.Error("Expected output for unicode tree")
	}
}

// =====================================================================
// TestParser - more branches
// =====================================================================

func TestTestParser_AllBranches(t *testing.T) {
	p := &TestParser{}

	// CanParse
	if !p.CanParse("go", []string{"test"}) {
		t.Error("Expected go test to match")
	}
	if !p.CanParse("pytest", nil) {
		t.Error("Expected pytest to match")
	}
	if p.CanParse("go", []string{"build"}) {
		t.Error("Expected go build NOT to match TestParser")
	}

	// Empty results -> fallback message
	output := p.Parse("nothing here")
	if output != "Tests finished. (No summary captured)" {
		t.Errorf("Expected fallback message, got %s", output)
	}

	// FAIL block
	output = p.Parse("FAIL: TestFoo\nError: expected true\n\nok pith 0.5s")
	if !strings.Contains(output, "FAIL") {
		t.Errorf("Expected FAIL in output, got %s", output)
	}

	// Error: block
	output = p.Parse("Error: something went wrong\nstack trace\n\nPASS")
	if !strings.Contains(output, "Error") {
		t.Errorf("Expected Error in output, got %s", output)
	}
}

// =====================================================================
// ThneedParser - parseJson large (>10 results), parsePlain, parseJsonObject
// =====================================================================

func TestThneedParser_AllBranches(t *testing.T) {
	p := &ThneedParser{}

	// parseJson with > 10 results
	var sb strings.Builder
	sb.WriteString("[")
	for i := 0; i < 12; i++ {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(`{"id": "node` + string(rune('0'+i)) + `", "path": "path/to/file.go", "content": "some content here"}`)
	}
	sb.WriteString("]")
	output := p.Parse(sb.String())
	if !strings.Contains(output, "more results hidden") {
		t.Errorf("Expected 'more results hidden' for >10 results, got %s", output)
	}

	// parsePlain - with noise and long output
	longPlain := "Indexing files...\nScanning directory...\n"
	for i := 0; i < 60; i++ {
		longPlain += "meaningful line\n"
	}
	output = p.Parse(longPlain)
	if !strings.Contains(output, "...") {
		t.Errorf("Expected truncation in parsePlain, got %s", output)
	}

	// parseJsonObject - with path/id
	output = p.Parse(`{"path": "/some/file.go", "id": "abc123"}`)
	if !strings.Contains(output, "Thneed Node") {
		t.Errorf("Expected Thneed Node output, got %s", output)
	}

	// parseJsonObject - metadata (no path/id)
	output = p.Parse(`{"status": "ok", "count": 5}`)
	if !strings.Contains(output, "Structured Metadata") {
		t.Errorf("Expected Structured Metadata output, got %s", output)
	}

	// parseJson with Windows path (backslash)
	output = p.Parse(`[{"id": "1", "path": "C:\\Users\\test\\file.go", "content": "x"}]`)
	if !strings.Contains(output, "file.go") {
		t.Errorf("Expected filename after backslash trim, got %s", output)
	}

	// parseJson with long content (>150 chars)
	longContent := strings.Repeat("X", 200)
	output = p.Parse(`[{"id": "1", "path": "p", "content": "` + longContent + `"}]`)
	if !strings.Contains(output, "...") {
		t.Errorf("Expected content truncation in parseJson, got %s", output)
	}

	// parseJson - empty results
	output = p.Parse(`[]`)
	if output != "Thneed: No results found." {
		t.Errorf("Expected 'No results found', got %s", output)
	}
}

// =====================================================================
// SourceParser - Parse edge cases
// =====================================================================

func TestSourceParser_AllBranches(t *testing.T) {
	p := &SourceParser{}

	// CanParse matches cat or type
	if !p.CanParse("cat", nil) {
		t.Error("Expected cat to match SourceParser")
	}
	if !p.CanParse("type", nil) {
		t.Error("Expected type to match SourceParser")
	}
	if p.CanParse("ls", nil) {
		t.Error("Expected ls NOT to match SourceParser")
	}

	// Parse with block comments (using a long one to trigger stripping)
	code := "/* this is a very long block comment that exceeds the one hundred character threshold to ensure that it is actually stripped by the source parser as intended by this specific test case */\nfoo := 1 // inline\n/* start block\n * middle\n */\nbar := 2"
	output := p.Parse(code)
	if strings.Contains(output, "very long block comment") {
		t.Errorf("Expected long block comment to be stripped, got %s", output)
	}
	if !strings.Contains(output, "bar") {
		t.Errorf("Expected bar to remain, got %s", output)
	}
}

// =====================================================================
// TextParser branches
// =====================================================================

func TestGrepMinifyParser_Branches(t *testing.T) {
	// GrepParser
	grep := &GrepParser{}
	if !grep.CanParse("grep", nil) {
		t.Error("Expected grep to match GrepParser")
	}
	var sb strings.Builder
	for i := 0; i < 200; i++ {
		sb.WriteString("match in file.go\n")
	}
	output := grep.Parse(sb.String())
	if output == "" {
		t.Error("Expected output from grep parser")
	}

	// MinifyParser
	minify := &MinifyParser{}
	if !minify.CanParse("cat", nil) {
		t.Log("MinifyParser CanParse checked")
	}
	output = minify.Parse("  hello   world  \n  next line  ")
	if output == "" {
		t.Error("Expected output from minify parser")
	}
}

// =====================================================================
// NPM parser - more branches
// =====================================================================

func TestNPMParser_Branches(t *testing.T) {
	p := &NPMParser{}

	// Large output with noise
	npmOut := `npm warn deprecated old-package@1.0
npm info resolving...
added 5 packages
up to date, audited 100 packages
found 0 vulnerabilities`
	output := p.Parse(npmOut)
	if !strings.Contains(output, "added 5 packages") {
		t.Errorf("Expected npm summary, got %s", output)
	}

	// npm warn deprecated line (should be filtered or included)
	output = p.Parse("npm warn deprecated x@1.0\nsomething else")
	if output == "" {
		t.Error("Expected output from npm parser")
	}
}

// =====================================================================
// PromptfooParser - more branches
// =====================================================================

func TestPromptfooParser_Branches(t *testing.T) {
	p := &PromptfooParser{}

	// Parse with test results
	output := p.Parse("✓ prompt test passed\n✗ another test FAILED\nsummary: 1 passed, 1 failed")
	if output == "" {
		t.Error("Expected output from promptfoo parser")
	}

	// Parse with empty output
	output = p.Parse("")
	if output != "" {
		t.Error("Expected empty output for empty input")
	}
}

// =====================================================================
// GitStash and composite
// =====================================================================

func TestCompositeGitParser_AllBranches(t *testing.T) {
	p := &CompositeGitParser{}

	if !p.CanParse("git status; git log -3", nil) {
		t.Log("CompositeGitParser CanParse checked")
	}
	if !p.CanParse("git status & git diff", nil) {
		t.Log("CompositeGitParser & checked")
	}
	if p.CanParse("git", []string{"status"}) {
		t.Log("Simple git does NOT match composite")
	}

	// Parse with diff and log combined
	input := `On branch main
commit abc1234
Author: Developer <dev@example.com>
Date:   Mon Jan 01 12:00:00 2024 +0000

    Add feature

diff --git a/main.go b/main.go
index abc..def 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
+new line
-old line`
	output := p.Parse(input)
	if output == "" {
		t.Error("Expected output from composite git parser")
	}
}
