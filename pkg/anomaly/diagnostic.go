package anomaly

import (
	"bytes"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

// Diagnose performs root-cause analysis using the details captured in an anomaly.
func Diagnose(a Anomaly) (string, error) {
	fmt.Printf("\n    [Pith Diagnostics] Preparing analysis for '%s'...\n", a.Project)

	prompt := diagnosticPrompt(a)
	fmt.Println("    [Pith Diagnostics] Consulting the Oracle for Root Cause Analysis...")

	// Ask the LLM using the configured Gemini CLI route.
	geminiCmd := exec.Command("gemini", "chat", "--prompt", prompt)

	// We want to force the oracle so Overseer handles it
	geminiCmd.Args = append(geminiCmd.Args, "--oracle")

	var out bytes.Buffer
	var stderr bytes.Buffer
	geminiCmd.Stdout = &out
	geminiCmd.Stderr = &stderr

	err := geminiCmd.Run()
	if err != nil {
		return "", fmt.Errorf("diagnostic failed: %v\nstderr: %s", err, stderr.String())
	}

	return strings.TrimSpace(out.String()), nil
}

func diagnosticPrompt(a Anomaly) string {
	return fmt.Sprintf(`You are an expert AI Forensics Engineer.
An anomaly was detected in our LLM telemetry stream. Your task is to diagnose the root cause and provide actionable advice.

## Anomaly Details
- Project: %s
- Severity: %s
- Flag Reason: %s
- Model Used: %s

## Offending Interaction
**Prompt:** %s
**Response:** %s

Please provide a concise, structured Root Cause Analysis and a proposed fix.`,
		a.Project, a.Severity, a.Reason, a.Model,
		diagnosticSnippet(a.Prompt), diagnosticSnippet(a.Response))
}

var diagnosticSecretPattern = regexp.MustCompile(`(?i)(api[_-]?key|token|password|secret)\s*[:=]\s*[^\s]+`)

// diagnosticSnippet keeps external diagnostic payloads small and removes
// common credential assignments. It is deliberately conservative: callers
// must still explicitly opt in before anything leaves the machine.
func diagnosticSnippet(value string) string {
	value = diagnosticSecretPattern.ReplaceAllString(value, "$1=[REDACTED]")
	const maxBytes = 512
	if len(value) > maxBytes {
		return value[:maxBytes] + "…[truncated]"
	}
	return value
}
