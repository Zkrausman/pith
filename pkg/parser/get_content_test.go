package parser

import (
	"strings"
	"testing"
)

func TestGetContentParser(t *testing.T) {
	p := &GetContentParser{}

	// Test CanParse
	if !p.CanParse("powershell", []string{"-Command", "Get-Content file.json"}) {
		t.Errorf("Expected CanParse to handle powershell Get-Content")
	}
	if !p.CanParse("Get-Content", []string{"file.json"}) {
		t.Errorf("Expected CanParse to handle direct Get-Content")
	}
	if !p.CanParse("cat", []string{"file.json"}) {
		t.Errorf("Expected CanParse to handle cat alias")
	}

	// Test JSON Minification
	jsonInput := `{
		"id": "123",
		"name": "test",
		"items": [
			1,
			2,
			3
		]
	}`
	jsonOutput := p.Parse(jsonInput)
	if strings.Contains(jsonOutput, "  ") || strings.Contains(jsonOutput, "\n") {
		t.Errorf("Expected JSON to be minified (no spaces/newlines outside strings)")
	}

	// Test Large JSON Truncation
	largeJsonInput := `[` + strings.Repeat(`{"key":"`+strings.Repeat("v", 100)+`"},`, 20) + `{"key":"final"}]`
	largeJsonOutput := p.Parse(largeJsonInput)
	if len(largeJsonOutput) > 2100 {
		t.Errorf("Expected large JSON to be truncated, got length %d", len(largeJsonOutput))
	}
	if !strings.Contains(largeJsonOutput, "minimized by Pith") {
		t.Errorf("Expected truncation suffix for large JSON")
	}

	// Test Plain Text Truncation
	textInput := strings.Repeat("line\n", 100)
	textOutput := p.Parse(textInput)
	lines := strings.Split(strings.TrimSpace(textOutput), "\n")
	// Note: p.Parse adds a line for truncation message
	if len(lines) > 52 {
		t.Errorf("Expected plain text to be truncated to ~50 lines, got %d", len(lines))
	}
	if !strings.Contains(textOutput, "minimized by Pith") {
		t.Errorf("Expected truncation message for long text")
	}
}
