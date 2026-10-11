package main

import (
	"bytes"
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"pith/pkg/config"
	"pith/pkg/telemetry"
)

func TestLegacyHookMixedCommandPreservation(t *testing.T) {
	for _, command := range []string{
		"git status; echo second", "git status&&echo second", "git status || echo second",
		"git status | cat", "git status & echo second", "git status\necho second",
		"git status\r\necho second", "git status\recho second", "git status\n",
		"git status > fixture", "git status $(echo main)", "git status `echo main`",
		"& git status", "\"git\" status", "git status --ignored='literal'",
		"git log --oneline; echo second", "git status",
	} {
		t.Run(command, func(t *testing.T) {
			home, storage := t.TempDir(), t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("PITH_STORAGE", storage)
			cfg := &config.Config{StoragePath: storage, LastUpdateCheck: time.Now().Unix(), MaxLines: 500}
			if err := cfg.Save(); err != nil {
				t.Fatal(err)
			}
			input := HookInput{ToolInput: map[string]interface{}{"command": command}}
			input.ToolResponse.LlmContent = "On branch main\n M file.go\nsecond\n"
			data, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			stdin, err := os.CreateTemp(t.TempDir(), "stdin")
			if err != nil {
				t.Fatal(err)
			}
			defer stdin.Close()
			if _, err := stdin.Write(data); err != nil {
				t.Fatal(err)
			}
			if _, err := stdin.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			oldStdin := os.Stdin
			os.Stdin = stdin
			defer func() { os.Stdin = oldStdin }()
			cmd := NewRootCmd()
			cmd.SetArgs([]string{"_hook"})
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			var got HookOutput
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("decode %q: %v", out.String(), err)
			}
			if command == "git status" {
				if got.Decision != "deny" || !strings.Contains(got.SystemMessage, "git_status") {
					t.Fatalf("ordinary single command should still parse: %#v", got)
				}
			} else if got.Decision != "allow" || got.Reason != "" {
				t.Fatalf("mixed/ambiguous command should keep original host output: %#v", got)
			}
		})
	}
}

func TestLegacyHookIdentityParserKeepsPassthroughProvenance(t *testing.T) {
	cases := []struct {
		name, command, output string
		maxLines              int
		truncated             bool
	}{
		{name: "source identity", command: "cat source.go", output: "package fixture\nconst value = 1"},
		{name: "ls identity", command: "ls", output: "one-name"},
		{name: "Get-Content identity", command: "Get-Content -Path file.txt", output: "one line"},
		{name: "later middle-out truncation", command: "cat source.go", output: "package fixture\nconst one = 1\nconst two = 2\nconst three = 3", maxLines: 2, truncated: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home, storage := t.TempDir(), t.TempDir()
			t.Setenv("HOME", home)
			t.Setenv("USERPROFILE", home)
			t.Setenv("PITH_STORAGE", storage)
			maxLines := tc.maxLines
			if maxLines == 0 {
				maxLines = 500
			}
			cfg := &config.Config{
				StoragePath: storage, LastUpdateCheck: time.Now().Unix(),
				MaxLines: maxLines, HeadLines: 1, TailLines: 1,
			}
			if err := cfg.Save(); err != nil {
				t.Fatal(err)
			}
			input := HookInput{ToolInput: map[string]interface{}{"command": tc.command}}
			input.ToolResponse.LlmContent = tc.output
			data, err := json.Marshal(input)
			if err != nil {
				t.Fatal(err)
			}
			stdin, err := os.CreateTemp(t.TempDir(), "stdin")
			if err != nil {
				t.Fatal(err)
			}
			defer stdin.Close()
			if _, err := stdin.Write(data); err != nil {
				t.Fatal(err)
			}
			if _, err := stdin.Seek(0, 0); err != nil {
				t.Fatal(err)
			}
			oldStdin := os.Stdin
			os.Stdin = stdin
			defer func() { os.Stdin = oldStdin }()
			cmd := NewRootCmd()
			cmd.SetArgs([]string{"_hook"})
			var out bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&out)
			if err := cmd.Execute(); err != nil {
				t.Fatal(err)
			}
			var got HookOutput
			if err := json.Unmarshal(out.Bytes(), &got); err != nil {
				t.Fatalf("decode %q: %v", out.String(), err)
			}
			if tc.truncated {
				if got.Decision != "deny" || !strings.Contains(got.Reason, "removed by Pith") || !strings.Contains(got.SystemMessage, "parser: none") {
					t.Fatalf("later truncation should not acquire parser provenance: %#v", got)
				}
			} else if got.Decision != "allow" || got.Reason != "" {
				t.Fatalf("identity parser should leave host output untouched: %#v", got)
			}
			tel, err := telemetry.NewTelemetry(storage)
			if err != nil {
				t.Fatal(err)
			}
			records, err := tel.GetRecentExecutions(1, "")
			_ = tel.Close()
			if err != nil || len(records) != 1 {
				t.Fatalf("hook telemetry records=%#v, err=%v", records, err)
			}
			if records[0].ParserUsed != "none" || !records[0].IsPassthrough {
				t.Fatalf("identity parser was falsely counted as transformed: %#v", records[0])
			}
			wantReason := telemetry.DecisionUnsupportedParser
			if tc.truncated {
				wantReason = telemetry.DecisionTransformed
			}
			if records[0].DecisionReason != wantReason {
				t.Fatalf("identity and later truncation provenance mismatch: %#v", records[0])
			}
		})
	}
}
