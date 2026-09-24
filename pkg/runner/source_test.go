package runner

import (
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"

	"pith/pkg/config"
	"pith/pkg/telemetry"
)

func TestDetectSourceGemini(t *testing.T) {
	t.Setenv("GEMINI_CLI", "true")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("CLAUDE_CODE", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	if DetectSource() != "gemini" {
		t.Errorf("Expected gemini, got %s", DetectSource())
	}
}

func TestDetectSourceClaude(t *testing.T) {
	t.Setenv("GEMINI_CLI", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("CLAUDE_CODE", "true")
	t.Setenv("ANTHROPIC_API_KEY", "")
	if DetectSource() != "claude" {
		t.Errorf("Expected claude, got %s", DetectSource())
	}
}

func TestDetectSourceUnknown(t *testing.T) {
	t.Setenv("GEMINI_CLI", "")
	t.Setenv("GOOGLE_API_KEY", "")
	t.Setenv("CLAUDE_CODE", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	if DetectSource() != "unknown" {
		t.Errorf("Expected unknown, got %s", DetectSource())
	}
}

func TestRunnerCatShortSourceOutput(t *testing.T) {
	fixtureDir := t.TempDir()
	t.Chdir(fixtureDir)
	input := strings.Join([]string{
		"package main",
		"// generic comment",
		"// TODO: keep this",
		`var endpoint = "https://example.invalid/api" // generic inline`,
		`var fallback = "http://example.invalid/a//b"`,
		`var note = "https://example.invalid/warn" // FIXME: keep this`,
	}, "\n") + "\n"
	if err := os.WriteFile("example.go", []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{
		"package main",
		"// TODO: keep this",
		`var endpoint = "https://example.invalid/api"`,
		`var fallback = "http://example.invalid/a//b"`,
		`var note = "https://example.invalid/warn" // FIXME: keep this`,
	}, "\n")

	tel, err := telemetry.NewTelemetry(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer tel.Close()
	r := NewRunner(&config.Config{MaxLines: 100, HeadLines: 5, TailLines: 5}, tel)
	if p := r.selectParser("cat example.go"); p == nil || p.Name() != "source" {
		t.Fatalf("cat example.go selected %v, want source", p)
	}

	command := "cat"
	if _, err := exec.LookPath(command); err != nil {
		if runtime.GOOS != "windows" {
			t.Skip("cat is unavailable")
		}
		// On Windows without cat.exe, exercise the same SourceParser via cmd's type.
		command = "type"
	}
	if p := r.selectParser(command + " example.go"); p == nil || p.Name() != "source" {
		t.Fatalf("%s example.go selected %v, want source", command, p)
	}

	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	defer stdout.Close()
	originalStdout := os.Stdout
	os.Stdout = stdout
	defer func() { os.Stdout = originalStdout }()
	err = r.RunWithOptions([]string{command, "example.go"}, false)
	os.Stdout = originalStdout
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(stdout.Name())
	if err != nil {
		t.Fatal(err)
	}
	if output := strings.ReplaceAll(string(got), "\r\n", "\n"); output != want {
		t.Errorf("short runner output = %q, want %q", output, want)
	}
	records, err := tel.GetRecentExecutions(10, "")
	if err != nil || len(records) != 1 {
		t.Fatalf("runner records = %v, error = %v", records, err)
	}
	if records[0].ParserUsed != "source" || records[0].IsPassthrough || records[0].DecisionReason != telemetry.DecisionTransformed {
		t.Errorf("runner did not apply SourceParser: %+v", records[0])
	}
}
