package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCodexHooksFlagDispatchesInstallerWithoutAutoTrust(t *testing.T) {
	original := installCodexHooksCommand
	defer func() { installCodexHooksCommand = original }()
	called := 0
	installCodexHooksCommand = func(cmd *cobra.Command) error {
		called++
		cmd.Println("fake Codex installer")
		return nil
	}

	cmd := NewRootCmd()
	var output bytes.Buffer
	cmd.SetOut(&output)
	cmd.SetErr(&output)
	cmd.SetArgs([]string{"--hooks"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Fatalf("installer calls = %d, want exactly one", called)
	}
	if !strings.Contains(output.String(), "fake Codex installer") {
		t.Fatalf("unexpected output: %q", output.String())
	}
}

func TestCodexHooksFlagRejectsNativeCommandArguments(t *testing.T) {
	original := installCodexHooksCommand
	defer func() { installCodexHooksCommand = original }()
	called := false
	installCodexHooksCommand = func(*cobra.Command) error {
		called = true
		return nil
	}

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"--hooks", "echo", "must-not-run"})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "does not accept a command") {
		t.Fatalf("--hooks with command args error = %v", err)
	}
	if called {
		t.Fatal("installer ran with native command arguments")
	}
}
