package main

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

type panicOnRead struct{}

func (panicOnRead) Read([]byte) (int, error) { panic("disabled hook read stdin") }

func invokeDisabledPreTool(t *testing.T, input io.Reader) PreToolHookOutput {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.SetIn(input)
	var output bytes.Buffer
	cmd.SetOut(&output)
	if err := runHookPreTool(cmd, nil); err != nil {
		t.Fatalf("constant allow response failed: %v", err)
	}
	var decoded PreToolHookOutput
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("decode response %q: %v", output.String(), err)
	}
	return decoded
}

func TestHookPreToolDisabledAlwaysAllowsWithoutReadingInput(t *testing.T) {
	inputs := []io.Reader{
		strings.NewReader(""),
		strings.NewReader("not json"),
		strings.NewReader(`{"toolCall":{"name":"run_command","args":{"CommandLine":"touch should-not-run"}}}`),
		panicOnRead{},
	}
	for _, input := range inputs {
		got := invokeDisabledPreTool(t, input)
		if got.Decision != "allow" || got.Reason != "" || len(got.Overwrite) != 0 {
			t.Fatalf("disabled hook response = %#v", got)
		}
	}
}

func TestDisabledAGYCallbackDoesNotCreateStorage(t *testing.T) {
	storage := filepath.Join(t.TempDir(), "unused-pith-storage")
	t.Setenv("PITH_STORAGE", storage)
	got := invokeDisabledPreTool(t, panicOnRead{})
	if got.Decision != "allow" || len(got.Overwrite) != 0 {
		t.Fatalf("disabled hook response = %#v", got)
	}
	if _, err := os.Stat(storage); !os.IsNotExist(err) {
		t.Fatalf("disabled callback created storage: %v", err)
	}
}

func TestRetiredAGYExecutionCommandsAreNotRegistered(t *testing.T) {
	for _, command := range NewRootCmd().Commands() {
		if command.Name() == "hook-run" || command.Name() == "agy-exec" {
			t.Fatalf("retired AGY execution command remains registered: %s", command.Name())
		}
	}
}

func TestAntigravityInstallRejectedBeforeBinaryInstall(t *testing.T) {
	previous := installPithBinaryCommand
	called := false
	installPithBinaryCommand = func() error {
		called = true
		return nil
	}
	t.Cleanup(func() { installPithBinaryCommand = previous })

	root := NewRootCmd()
	var installCommand *cobra.Command
	for _, command := range root.Commands() {
		if command.Name() == "install" {
			installCommand = command
			break
		}
	}
	if installCommand == nil {
		t.Fatal("install command missing")
	}
	if err := installCommand.Flags().Set("antigravity", "true"); err != nil {
		t.Fatal(err)
	}
	err := runInstall(installCommand, nil)
	if err == nil || !strings.Contains(err.Error(), "disabled pending a supported, reviewed implementation") {
		t.Fatalf("unexpected Antigravity install result: %v", err)
	}
	if called {
		t.Fatal("binary installation ran before the Antigravity rejection")
	}
}
