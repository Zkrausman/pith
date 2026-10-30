package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"pith/pkg/telemetry"
	"runtime"
	"strings"
	"testing"
)

func TestRootCmd(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"--version"})

	b := new(bytes.Buffer)
	cmd.SetOut(b)
	cmd.SetErr(b)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("rootCmd.Execute() failed: %v", err)
	}

	out := b.String()
	if !strings.Contains(out, version) {
		t.Errorf("Expected version %s, got %s", version, out)
	}
}

func TestVersionCmd(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"version"})

	b := new(bytes.Buffer)
	cmd.SetOut(b)
	cmd.SetErr(b)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	out := b.String()
	if !strings.Contains(out, version) {
		t.Errorf("Expected version %s, got %s", version, out)
	}
}

func TestGainCmd_Help(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"gain", "--help"})
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	if err := cmd.Execute(); err != nil {
		t.Errorf("gain --help failed: %v", err)
	}
}

func TestDiscoverCmd_Help(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"discover", "--help"})
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	if err := cmd.Execute(); err != nil {
		t.Errorf("discover --help failed: %v", err)
	}
}

func TestConfigCmd_Help(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"config", "--help"})
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	if err := cmd.Execute(); err != nil {
		t.Errorf("config --help failed: %v", err)
	}
}

func TestHookInputSchema(t *testing.T) {
	// Test newer schema
	data := []byte(`{"tool_input": {"command": "git status"}, "tool_response": {"llmContent": "hi"}}`)
	var input HookInput
	if err := json.Unmarshal(data, &input); err != nil {
		t.Fatal(err)
	}
	if input.ToolInput["command"] != "git status" {
		t.Errorf("Expected git status, got %v", input.ToolInput["command"])
	}

	// Test older schema
	dataOld := []byte(`{"tool_call_request": {"arguments": "{\"command\": \"ls\"}"}, "tool_response": {"llmContent": "hi"}}`)
	var inputOld HookInput
	if err := json.Unmarshal(dataOld, &inputOld); err != nil {
		t.Fatal(err)
	}
	if inputOld.ToolCallRequest.Arguments == "" {
		t.Error("Expected arguments to be populated")
	}
}

func TestPiTransformRejectsTrailingJSONValues(t *testing.T) {
	request := `{"command":"git status","output":"result","exitCode":0}`
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"pi", "transform"})
	cmd.SetIn(strings.NewReader(request + request))
	cmd.SetOut(new(bytes.Buffer))

	if err := cmd.Execute(); err == nil {
		t.Fatal("pi transform accepted multiple JSON values")
	}
}

func TestPiTransformHonorsPersistedEnabledParsers(t *testing.T) {
	storage := t.TempDir()
	t.Setenv("PITH_STORAGE", storage)
	if err := os.WriteFile(filepath.Join(storage, "config.json"), []byte(`{"enabled_parsers":{"git_status":false}}`), 0644); err != nil {
		t.Fatal(err)
	}
	request, err := json.Marshal(map[string]interface{}{
		"command":  "git status",
		"output":   strings.Repeat("On branch main\n\nChanges not staged for commit:\n\tmodified: pkg/pi/hook.go\n", 200),
		"exitCode": 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	out := new(bytes.Buffer)
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"pi", "transform"})
	cmd.SetIn(bytes.NewReader(request))
	cmd.SetOut(out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Parser      string `json:"parser"`
		Passthrough bool   `json:"passthrough"`
	}
	if err := json.Unmarshal(out.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Parser != "" || !response.Passthrough {
		t.Fatalf("disabled parser was used: %#v", response)
	}
}

func TestHookOutputSchema(t *testing.T) {
	output := HookOutput{
		Decision: "deny",
		Reason:   "compressed",
	}
	data, err := json.Marshal(output)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"decision":"deny","reason":"compressed","systemMessage":""}` {
		t.Errorf("Unexpected JSON output: %s", string(data))
	}
}

func TestGainCmd(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)

	// Add some mock telemetry data
	tel, err := telemetry.NewTelemetry(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	tel.Record(telemetry.ExecutionRecord{
		Command:          "git status",
		OriginalTokens:   100,
		CompressedTokens: 50,
		ParserUsed:       "git",
	})
	tel.Close()

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"gain"})
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	if err := cmd.Execute(); err != nil {
		t.Errorf("gain failed: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "Pith: Overall Token Savings") {
		t.Errorf("Unexpected output: %s", out)
	}
}

func TestGainCmd_PricesByRecordedModel(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)
	tel, err := telemetry.NewTelemetry(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	rate := 20.0
	if err := tel.Record(telemetry.ExecutionRecord{Command: "git status", OriginalTokens: 1000, CompressedTokens: 500, Model: "openai/gpt-5", InputCostPerMillion: &rate}); err != nil {
		t.Fatal(err)
	}
	if err := tel.Record(telemetry.ExecutionRecord{Command: "legacy", OriginalTokens: 200, CompressedTokens: 100}); err != nil {
		t.Fatal(err)
	}
	tel.Close()

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"stats"})
	out := new(bytes.Buffer)
	cmd.SetOut(out)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	result := out.String()
	if !strings.Contains(result, "openai/gpt-5") || !strings.Contains(result, "$0.01 recorded") || !strings.Contains(result, "fallback estimate") {
		t.Fatalf("model-aware gain output missing: %s", result)
	}
}

func TestDiscoverCmd(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)

	// Add some mock telemetry data
	tel, err := telemetry.NewTelemetry(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	tel.Record(telemetry.ExecutionRecord{
		Command:          "unknown-cmd",
		OriginalTokens:   100,
		CompressedTokens: 100,
		IsPassthrough:    true,
	})
	tel.Close()

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"discover"})
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	if err := cmd.Execute(); err != nil {
		t.Errorf("discover failed: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "Opportunity Discovery") {
		t.Errorf("Unexpected output: %s", out)
	}
}

func TestResetCmd(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"reset", "--all"})
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	if err := cmd.Execute(); err != nil {
		t.Errorf("reset failed: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "All telemetry data has been reset") {
		t.Errorf("Unexpected output: %s", out)
	}
}

func TestHookCmd(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)

	input := HookInput{}
	input.ToolInput = make(map[string]interface{})
	input.ToolInput["command"] = "git status"
	input.ToolResponse.LlmContent = "Output: On branch main\nnothing to commit"

	data, _ := json.Marshal(input)

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()

	r, w, _ := os.Pipe()
	os.Stdin = r
	go func() {
		w.Write(data)
		w.Close()
	}()

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"_hook"})
	b := new(bytes.Buffer)
	cmd.SetOut(b)

	if err := cmd.Execute(); err != nil {
		t.Errorf("_hook failed: %v", err)
	}

	var output HookOutput
	if err := json.Unmarshal(b.Bytes(), &output); err != nil {
		t.Fatalf("Failed to parse hook output: %v", err)
	}

	// Since git status should be parsed, decision should be deny
	if output.Decision != "deny" {
		t.Errorf("Expected decision deny, got %s", output.Decision)
	}
	if !strings.Contains(output.SystemMessage, "tokens saved:") {
		t.Errorf("Expected SystemMessage to contain 'tokens saved:', got %s", output.SystemMessage)
	}
}

func TestGainCmd_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"gain"})
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	if err := cmd.Execute(); err != nil {
		t.Errorf("gain failed: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "No telemetry data recorded yet") {
		t.Errorf("Expected 'No telemetry data' message, got %s", out)
	}
}

func TestResetCmd_Discover(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"reset", "--discover"})
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	if err := cmd.Execute(); err != nil {
		t.Errorf("reset discover failed: %v", err)
	}
	out := b.String()
	if !strings.Contains(out, "Discovery data (passthrough commands) has been reset") {
		t.Errorf("Unexpected output: %s", out)
	}
}

func TestHookCmd_NoCmd(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)

	input := HookInput{}
	input.ToolResponse.LlmContent = "some content"
	data, _ := json.Marshal(input)

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()
	r, w, _ := os.Pipe()
	os.Stdin = r
	go func() {
		w.Write(data)
		w.Close()
	}()

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"_hook"})
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	if err := cmd.Execute(); err != nil {
		t.Errorf("_hook failed: %v", err)
	}

	var output HookOutput
	json.Unmarshal(b.Bytes(), &output)
	if output.Decision != "allow" {
		t.Errorf("Expected decision allow for no command, got %s", output.Decision)
	}
}

func TestHookCmd_Error(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)

	input := HookInput{}
	input.ToolInput = make(map[string]interface{})
	input.ToolInput["command"] = "git status"
	input.ToolResponse.LlmContent = "Error: fatal error"
	data, _ := json.Marshal(input)

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()
	r, w, _ := os.Pipe()
	os.Stdin = r
	go func() {
		w.Write(data)
		w.Close()
	}()

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"_hook"})
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	if err := cmd.Execute(); err != nil {
		t.Errorf("_hook failed: %v", err)
	}

	var output HookOutput
	json.Unmarshal(b.Bytes(), &output)
	if !strings.Contains(output.Reason, "fatal error") {
		t.Errorf("Expected error message in reason, got %s", output.Reason)
	}
}

func TestHookCmd_OutputPrefix(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)

	input := HookInput{}
	input.ToolInput = make(map[string]interface{})
	input.ToolInput["command"] = "git status"
	input.ToolResponse.LlmContent = "Output: On branch main"
	data, _ := json.Marshal(input)

	oldStdin := os.Stdin
	defer func() { os.Stdin = oldStdin }()
	r, w, _ := os.Pipe()
	os.Stdin = r
	go func() {
		w.Write(data)
		w.Close()
	}()

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"_hook"})
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	_ = cmd.Execute()
}

func TestInstallCmd_Individual(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"install", "--claude"})
	_ = cmd.Execute()

	cmd = NewRootCmd()
	cmd.SetArgs([]string{"install", "--codex"})
	_ = cmd.Execute()
}

func TestInstallCmd_NoFlags(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"install"})
	_ = cmd.Execute()
}

func TestInstallCmd_All(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"install", "--all", "--global"})
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	cmd.SetErr(b)
	_ = cmd.Execute()
}

func TestInstallCmd(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)
	t.Setenv("HOME", tmpDir)
	t.Setenv("USERPROFILE", tmpDir)

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"install", "--gemini", "--global"})
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	cmd.SetErr(b)

	_ = cmd.Execute()
}

func TestUpdateCmd(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"update"})
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	_ = cmd.Execute()
}

func TestRawCmd(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)

	var args []string
	if runtime.GOOS == "windows" {
		args = []string{"raw", "cmd", "/c", "echo", "hello"}
	} else {
		args = []string{"raw", "echo", "hello"}
	}

	cmd := NewRootCmd()
	cmd.SetArgs(args)
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	if err := cmd.Execute(); err != nil {
		t.Errorf("raw failed: %v", err)
	}
}

func TestResetCmd_Errors(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)

	cmd := NewRootCmd()
	cmd.SetArgs([]string{"reset"})
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	if err := cmd.Execute(); err == nil {
		t.Error("Expected error for reset without flags")
	}
}

func TestRootCmd_Run(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)

	var args []string
	if runtime.GOOS == "windows" {
		args = []string{"cmd", "/c", "echo", "hello"}
	} else {
		args = []string{"echo", "hello"}
	}

	cmd := NewRootCmd()
	cmd.SetArgs(args)
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	if err := cmd.Execute(); err != nil {
		t.Errorf("root run failed: %v", err)
	}
}

func TestDashboardCmd_Help(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"dashboard", "--help"})
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	if err := cmd.Execute(); err != nil {
		t.Errorf("dashboard --help failed: %v", err)
	}
}

func TestRawCmd_Help(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"raw", "--help"})
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	if err := cmd.Execute(); err != nil {
		t.Errorf("raw --help failed: %v", err)
	}
}

func TestRootCmd_Empty(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("PITH_STORAGE", tmpDir)

	cmd := NewRootCmd()
	cmd.SetArgs([]string{}) // No args
	b := new(bytes.Buffer)
	cmd.SetOut(b)
	cmd.SetErr(b)
	_ = cmd.Execute()
}

func TestHookPreTool_StandardCommand(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"hook-pretool", "--source", "antigravity"})

	payload := fmt.Sprintf(`{"toolCall":{"name":"run_command","args":{"CommandLine":"git status","Cwd":%q,"IsDaemon":false}}}`, t.TempDir())
	inBuf := bytes.NewBufferString(payload)
	outBuf := new(bytes.Buffer)
	cmd.SetIn(inBuf)
	cmd.SetOut(outBuf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("hook-pretool execution failed: %v", err)
	}

	var resp PreToolHookOutput
	if err := json.Unmarshal(outBuf.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal response failed: %v", err)
	}

	if resp.Decision != "allow" {
		t.Errorf("Expected decision allow, got %s", resp.Decision)
	}
	if len(resp.Overwrite) != 0 {
		t.Fatalf("non-transformable git command must pass through unchanged: %#v", resp.Overwrite)
	}
}

func TestHookPreTool_WithModelName(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"hook-pretool", "--source", "antigravity"})

	payload := fmt.Sprintf(`{"toolCall":{"name":"run_command","args":{"CommandLine":"git diff","Cwd":%q}},"modelName":"gemini-3.7-flash; $(touch /tmp/nope)"}`, t.TempDir())
	inBuf := bytes.NewBufferString(payload)
	outBuf := new(bytes.Buffer)
	cmd.SetIn(inBuf)
	cmd.SetOut(outBuf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("hook-pretool execution failed: %v", err)
	}

	var resp PreToolHookOutput
	if err := json.Unmarshal(outBuf.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal response failed: %v", err)
	}

	if resp.Decision != "allow" {
		t.Errorf("Expected decision allow, got %s", resp.Decision)
	}
	if len(resp.Overwrite) != 0 {
		t.Fatalf("non-transformable git command was rewritten: %#v", resp.Overwrite)
	}
}

func TestHookPreTool_DaemonCommand(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"hook-pretool"})

	payload := `{"toolCall":{"name":"run_command","args":{"CommandLine":"npm start","IsDaemon":true}}}`
	inBuf := bytes.NewBufferString(payload)
	outBuf := new(bytes.Buffer)
	cmd.SetIn(inBuf)
	cmd.SetOut(outBuf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("hook-pretool execution failed: %v", err)
	}

	var resp PreToolHookOutput
	if err := json.Unmarshal(outBuf.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal response failed: %v", err)
	}

	if resp.Decision != "allow" {
		t.Errorf("Expected decision allow, got %s", resp.Decision)
	}
	if resp.Overwrite != nil && len(resp.Overwrite) > 0 {
		t.Errorf("Daemon process should not have overwrite, got %v", resp.Overwrite)
	}
}

func TestHookPreTool_RecursionGuard(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"hook-pretool"})

	payload := `{"toolCall":{"name":"run_command","args":{"CommandLine":"pith.exe git status"}}}`
	inBuf := bytes.NewBufferString(payload)
	outBuf := new(bytes.Buffer)
	cmd.SetIn(inBuf)
	cmd.SetOut(outBuf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("hook-pretool execution failed: %v", err)
	}

	var resp PreToolHookOutput
	if err := json.Unmarshal(outBuf.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal response failed: %v", err)
	}

	if resp.Overwrite != nil && len(resp.Overwrite) > 0 {
		t.Errorf("Pith command should not be re-wrapped, got %v", resp.Overwrite)
	}
}

func TestHookPreTool_NonRunCommand(t *testing.T) {
	cmd := NewRootCmd()
	cmd.SetArgs([]string{"hook-pretool"})

	payload := `{"toolCall":{"name":"view_file","args":{"AbsolutePath":"/test/file"}}}`
	inBuf := bytes.NewBufferString(payload)
	outBuf := new(bytes.Buffer)
	cmd.SetIn(inBuf)
	cmd.SetOut(outBuf)

	if err := cmd.Execute(); err != nil {
		t.Fatalf("hook-pretool execution failed: %v", err)
	}

	var resp PreToolHookOutput
	if err := json.Unmarshal(outBuf.Bytes(), &resp); err != nil {
		t.Fatalf("Unmarshal response failed: %v", err)
	}

	if resp.Overwrite != nil && len(resp.Overwrite) > 0 {
		t.Errorf("Non run_command should not have overwrite, got %v", resp.Overwrite)
	}
}
