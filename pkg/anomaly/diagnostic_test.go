package anomaly

import (
	"strings"
	"testing"
)

func TestDiagnosticSnippetRedactsAndBoundsPayload(t *testing.T) {
	value := "password=super-secret api_key: abc123 " + strings.Repeat("x", 600)
	got := diagnosticSnippet(value)
	if strings.Contains(got, "super-secret") || strings.Contains(got, "abc123") {
		t.Fatalf("secret was not redacted: %q", got)
	}
	if len(got) > 540 || !strings.Contains(got, "[truncated]") {
		t.Fatalf("diagnostic payload was not bounded: length %d", len(got))
	}
}

func TestDiagnosticPromptIncludesAnomalyDetails(t *testing.T) {
	anomaly := Anomaly{
		Project:  "demo-project",
		Severity: "critical",
		Reason:   "synthetic anomaly reason",
		Model:    "demo-model",
		Prompt:   "synthetic prompt",
		Response: "synthetic response",
	}

	prompt := diagnosticPrompt(anomaly)
	for _, want := range []string{
		"Project: demo-project",
		"Severity: critical",
		"Flag Reason: synthetic anomaly reason",
		"Model Used: demo-model",
		"**Prompt:** synthetic prompt",
		"**Response:** synthetic response",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("diagnostic prompt is missing %q", want)
		}
	}
}
