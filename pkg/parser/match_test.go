package parser

import (
	"testing"
)

func TestMatchCommand(t *testing.T) {
	tests := []struct {
		name     string
		cmd      string
		target   string
		expected bool
	}{
		{"Exact match", "npx", "npx", true},
		{"Case insensitive match", "NPX", "npx", true},
		{"Windows exe extension", "npx.exe", "npx", true},
		{"Windows cmd extension", "npx.cmd", "npx", true},
		{"Windows bat extension", "npx.bat", "npx", true},
		{"Windows ps1 extension", "npx.ps1", "npx", true},
		{"Full path Linux", "/usr/bin/npx", "npx", true},
		{"Full path Windows", `C:\Program Files\nodejs\npx.cmd`, "npx", true},
		{"Full path Windows mix", `C:/Program Files/nodejs/npx.exe`, "npx", true},
		{"Local path Windows", `.\other-tool.exe`, "other-tool", true},
		{"Local path Unix", `./snag`, "snag", true},
		{"Mismatch", "ls", "npx", false},
		{"Partial match prefix", "npx-cli", "npx", false},
		{"Partial match suffix", "mynpx", "npx", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := MatchCommand(tt.cmd, tt.target); got != tt.expected {
				t.Errorf("MatchCommand(%q, %q) = %v, want %v", tt.cmd, tt.target, got, tt.expected)
			}
		})
	}
}
