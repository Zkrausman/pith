package install

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func fakeExecutable(t *testing.T, path string, contents []byte) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, contents, 0755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestCodexHookInstallPreservesConfigAndIsIdempotent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Codex hook command uses a POSIX shell on supported installer platforms")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home with 'quotes")
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0700); err != nil {
		t.Fatal(err)
	}
	sourceBinary := fakeExecutable(t, filepath.Join(root, "build tree", "pith"), []byte("fake-pith-binary"))
	adapter := []byte("#!/usr/bin/env python3\n# exact embedded test fixture\n")
	oldBinary := []byte("previous installed pith")
	oldAdapter := []byte("previous adapter")
	if err := os.MkdirAll(filepath.Join(home, ".pith", "bin"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(home, ".pith", "hooks", "codex"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".pith", "bin", "pith"), oldBinary, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, ".pith", "hooks", "codex", "pre_tool_use_pith.py"), oldAdapter, 0600); err != nil {
		t.Fatal(err)
	}

	original := []byte(`{
  "description": "user-owned Codex hooks",
  "custom": {"number": 9007199254740993, "nested": [true, {"keep": "yes"}]},
  "hooks": {
    "SubagentStart": [{"matcher": "tldraw", "hooks": [{"type": "command", "command": "keep-this", "custom": {"nested": [1, 2, 3]}}]}],
    "PreToolUse": [{"matcher": "^Bash$", "hooks": [{"type": "command", "command": "user-hook", "timeout": 29}, {"type": "command", "command": "user-owned-marker", "statusMessage": "Pith: checking Bash command (managed by pith --hooks)"}]}],
    "SessionEnd": [{"hooks": [{"type": "command", "command": "other-event"}]}]
  }
}`)
	settingsPath := filepath.Join(home, ".codex", "hooks.json")
	if err := os.WriteFile(settingsPath, original, 0600); err != nil {
		t.Fatal(err)
	}

	result, err := setupCodexPreToolUseHook(home, sourceBinary, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if result.SettingsPath != settingsPath {
		t.Fatalf("settings path = %q, want %q", result.SettingsPath, settingsPath)
	}
	if got, err := os.ReadFile(result.BinaryPath); err != nil || !bytes.Equal(got, []byte("fake-pith-binary")) {
		t.Fatalf("installed binary = %q, err=%v", got, err)
	}
	if got, err := os.ReadFile(result.AdapterPath); err != nil || !bytes.Equal(got, adapter) {
		t.Fatalf("installed adapter = %q, err=%v", got, err)
	}
	if info, err := os.Stat(result.BinaryPath); err != nil || info.Mode().Perm() != 0755 {
		t.Fatalf("installed Pith mode = %v, err=%v", info, err)
	}
	if info, err := os.Stat(result.AdapterPath); err != nil || info.Mode().Perm() != 0644 {
		t.Fatalf("installed adapter mode = %v, err=%v", info, err)
	}
	if strings.Contains(string(mustRead(t, settingsPath)), sourceBinary) {
		t.Fatal("hook command leaked the private source executable path")
	}
	if len(result.Backups) != 3 {
		t.Fatalf("backup paths = %v, want backups for existing binary, adapter, and hooks.json", result.Backups)
	}
	wantBackups := map[string][]byte{
		result.BinaryPath:  oldBinary,
		result.AdapterPath: oldAdapter,
		settingsPath:      original,
	}
	for _, backup := range result.Backups {
		var originalPath string
		for path := range wantBackups {
			if strings.HasPrefix(backup, path+".pith-backup-") {
				originalPath = path
				break
			}
		}
		if originalPath == "" {
			t.Fatalf("backup path is not tied to an expected installed file: %s", backup)
		}
		if got := mustRead(t, backup); !bytes.Equal(got, wantBackups[originalPath]) {
			t.Fatalf("backup for %s changed original bytes: %s", originalPath, got)
		}
	}

	var beforeRoot, afterRoot map[string]any
	if err := json.Unmarshal(original, &beforeRoot); err != nil {
		t.Fatal(err)
	}
	installedSettings := mustRead(t, settingsPath)
	if !bytes.Contains(installedSettings, []byte("9007199254740993")) {
		t.Fatal("large unrelated JSON number was changed")
	}
	if err := json.Unmarshal(installedSettings, &afterRoot); err != nil {
		t.Fatal(err)
	}
	if beforeRoot["description"] != afterRoot["description"] || !equalJSONValue(t, beforeRoot["custom"], afterRoot["custom"]) {
		t.Fatalf("unrelated top-level data changed: %#v", afterRoot)
	}
	beforeHooks := beforeRoot["hooks"].(map[string]any)
	afterHooks := afterRoot["hooks"].(map[string]any)
	if !equalJSONValue(t, beforeHooks["SubagentStart"], afterHooks["SubagentStart"]) {
		t.Fatalf("SubagentStart (including nested tldraw hook) changed: %#v", afterHooks["SubagentStart"])
	}
	if !equalJSONValue(t, beforeHooks["SessionEnd"], afterHooks["SessionEnd"]) {
		t.Fatalf("unrelated event changed: %#v", afterHooks["SessionEnd"])
	}
	groups := afterHooks["PreToolUse"].([]any)
	if len(groups) != 2 {
		t.Fatalf("PreToolUse groups = %d, want existing group plus one Pith group", len(groups))
	}
	userHandlers := groups[0].(map[string]any)["hooks"].([]any)
	if len(userHandlers) != 2 || userHandlers[0].(map[string]any)["command"] != "user-hook" || userHandlers[1].(map[string]any)["command"] != "user-owned-marker" {
		t.Fatal("existing PreToolUse handler was changed")
	}
	pithGroup := groups[1].(map[string]any)
	if pithGroup["matcher"] != "^Bash$" {
		t.Fatalf("Pith matcher = %v", pithGroup["matcher"])
	}
	pithHandlers := pithGroup["hooks"].([]any)
	if len(pithHandlers) != 1 {
		t.Fatalf("Pith handlers = %d, want one", len(pithHandlers))
	}
	pithHandler := pithHandlers[0].(map[string]any)
	wantCommand := quotePOSIX("python3") + " -I -S " + quotePOSIX(result.AdapterPath) + " --pith-binary " + quotePOSIX(result.BinaryPath)
	if pithHandler["command"] != wantCommand {
		t.Fatalf("hook command = %q, want %q", pithHandler["command"], wantCommand)
	}
	if pithHandler["statusMessage"] != codexHookStatusMessage || pithHandler["type"] != "command" {
		t.Fatalf("unexpected Pith hook object: %#v", pithHandler)
	}
	if _, exists := pithHandler["trusted"]; exists {
		t.Fatal("installer must not set trust metadata")
	}
	if !strings.Contains(wantCommand, " --pith-binary '") || strings.Contains(wantCommand, sourceBinary) {
		t.Fatalf("unexpected adapter CLI command: %q", wantCommand)
	}

	firstSettings := append([]byte(nil), installedSettings...)
	second, err := setupCodexPreToolUseHook(home, sourceBinary, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Backups) != 0 {
		t.Fatalf("idempotent install created backups: %v", second.Backups)
	}
	if got := mustRead(t, settingsPath); !bytes.Equal(got, firstSettings) {
		t.Fatalf("idempotent install changed hooks.json:\n%s", got)
	}
	var finalRoot map[string]any
	if err := json.Unmarshal(mustRead(t, settingsPath), &finalRoot); err != nil {
		t.Fatal(err)
	}
	finalPreToolUse := finalRoot["hooks"].(map[string]any)["PreToolUse"].([]any)
	pithCount := 0
	for _, group := range finalPreToolUse {
		for _, handler := range group.(map[string]any)["hooks"].([]any) {
			entry := handler.(map[string]any)
			if entry["statusMessage"] == codexHookStatusMessage && entry["command"] == wantCommand {
				pithCount++
			}
		}
	}
	if pithCount != 1 {
		t.Fatalf("Pith handlers after repeat install = %d, want one", pithCount)
	}
}

func TestCodexHookInstallRejectsMalformedConfigWithoutOverwrite(t *testing.T) {
	cases := map[string]string{
		"invalid-json":        "{bad",
		"array-root":          "[]",
		"null-root":           "null",
		"duplicate-root-key":  `{"hooks":{},"hooks":{"SubagentStart":[]}}`,
		"duplicate-nested-key": `{"hooks":{"SubagentStart":[{"custom":{"x":1,"x":2}}]}}`,
		"null-hooks":          `{"hooks":null}`,
		"bad-event":           `{"hooks":{"PreToolUse":{}}}`,
		"bad-group":           `{"hooks":{"PreToolUse":[1]}}`,
		"missing-handlers":    `{"hooks":{"PreToolUse":[{"matcher":"Bash"}]}}`,
		"bad-handler-list":    `{"hooks":{"PreToolUse":[{"hooks":{}}]}}`,
		"bad-handler-object":  `{"hooks":{"PreToolUse":[{"hooks":[1]}]}}`,
	}
	cases["deep-json"] = `{"hooks":{"other":` + strings.Repeat("[", 130) + `null` + strings.Repeat("]", 130) + `}}`
	for name, malformed := range cases {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			home := filepath.Join(root, "fixture-home")
			if err := os.MkdirAll(filepath.Join(home, ".codex"), 0700); err != nil {
				t.Fatal(err)
			}
			settings := filepath.Join(home, ".codex", "hooks.json")
			if err := os.WriteFile(settings, []byte(malformed), 0600); err != nil {
				t.Fatal(err)
			}
			source := fakeExecutable(t, filepath.Join(root, "source", "pith"), []byte("fake"))
			_, err := setupCodexPreToolUseHook(home, source, []byte("adapter"))
			if err == nil {
				t.Fatal("malformed config unexpectedly accepted")
			}
			if got := mustRead(t, settings); string(got) != malformed {
				t.Fatalf("malformed config was changed: %q", got)
			}
			for _, path := range []string{
				filepath.Join(home, ".pith", "bin", "pith"),
				filepath.Join(home, ".pith", "hooks", "codex", "pre_tool_use_pith.py"),
			} {
				if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
					t.Fatalf("installer wrote %s before rejecting malformed config: %v", path, statErr)
				}
			}
		})
	}
}

func TestCodexHookInstallRefusesExistingLock(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	codexDir := filepath.Join(home, ".codex")
	if err := os.MkdirAll(codexDir, 0700); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(codexDir, "hooks.json")
	original := []byte(`{"hooks":{"SubagentStart":[{"matcher":"tldraw"}]}}`)
	if err := os.WriteFile(settings, original, 0600); err != nil {
		t.Fatal(err)
	}
	lock := settings + ".pith.lock"
	if err := os.WriteFile(lock, []byte("pid=123\n"), 0600); err != nil {
		t.Fatal(err)
	}
	source := fakeExecutable(t, filepath.Join(root, "source", "pith"), []byte("fake"))
	if _, err := setupCodexPreToolUseHook(home, source, []byte("adapter")); err == nil || !strings.Contains(err.Error(), "stale lock") {
		t.Fatalf("existing lock result = %v, want lock refusal", err)
	}
	if got := mustRead(t, settings); !bytes.Equal(got, original) {
		t.Fatalf("settings changed after lock refusal: %s", got)
	}
	if got := mustRead(t, lock); string(got) != "pid=123\n" {
		t.Fatalf("existing lock was removed or changed: %q", got)
	}
}

func TestCodexInstallTransactionDetectsConcurrentEditAndRollsBack(t *testing.T) {
	root := t.TempDir()
	firstPath := filepath.Join(root, "first")
	secondPath := filepath.Join(root, "second")
	if err := os.WriteFile(firstPath, []byte("original first"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, []byte("original second"), 0600); err != nil {
		t.Fatal(err)
	}
	firstSnapshot, err := captureFile(firstPath)
	if err != nil {
		t.Fatal(err)
	}
	secondSnapshot, err := captureFile(secondPath)
	if err != nil {
		t.Fatal(err)
	}
	concurrent := []byte("external edit must survive")
	changes := []desiredFile{
		{path: firstPath, data: []byte("replacement first"), mode: 0600, snapshot: &firstSnapshot},
		{path: secondPath, data: []byte("replacement second"), mode: 0600, snapshot: &secondSnapshot},
	}
	backups, err := applyFileTransactionWithHook(changes, func(path string) error {
		if path == secondPath {
			return os.WriteFile(secondPath, concurrent, 0600)
		}
		return nil
	})
	if err == nil || !strings.Contains(err.Error(), "changed during installation") {
		t.Fatalf("transaction error = %v, want concurrent-edit refusal", err)
	}
	if got := mustRead(t, firstPath); string(got) != "original first" {
		t.Fatalf("earlier replacement was not rolled back: %q", got)
	}
	if got := mustRead(t, secondPath); !bytes.Equal(got, concurrent) {
		t.Fatalf("concurrent edit was overwritten: %q", got)
	}
	if len(backups) != 2 {
		t.Fatalf("backup paths = %v, want both pre-transaction snapshots retained", backups)
	}
	if got := mustRead(t, backups[0]); string(got) != "original first" {
		t.Fatalf("first backup = %q", got)
	}
	if got := mustRead(t, backups[1]); string(got) != "original second" {
		t.Fatalf("second backup = %q", got)
	}
}

func TestCodexHookCommandPOSIXQuoting(t *testing.T) {
	got := quotePOSIX("/tmp/a b/'c")
	if got != `'/tmp/a b/'"'"'c'` {
		t.Fatalf("quoted path = %s", got)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func equalJSONValue(t *testing.T, left, right any) bool {
	t.Helper()
	leftJSON, err := json.Marshal(left)
	if err != nil {
		t.Fatal(err)
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Equal(leftJSON, rightJSON)
}

func TestCodexHookInstallRejectsSymlinkedHooksFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink fixture requires Unix permissions")
	}
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(filepath.Join(home, ".codex"), 0700); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(root, "external-hooks.json")
	original := []byte(`{"hooks":{"SubagentStart":[{"matcher":"tldraw"}]}}`)
	if err := os.WriteFile(external, original, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, filepath.Join(home, ".codex", "hooks.json")); err != nil {
		t.Fatal(err)
	}
	source := fakeExecutable(t, filepath.Join(root, "source", "pith"), []byte("fake"))
	if _, err := setupCodexPreToolUseHook(home, source, []byte("adapter")); err == nil {
		t.Fatal("symlinked Codex hooks file unexpectedly accepted")
	}
	if got := mustRead(t, external); !bytes.Equal(got, original) {
		t.Fatalf("external target was modified: %s", got)
	}
}

func TestCodexHookInstallRefusesNonExecutablePithInput(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	if err := os.MkdirAll(home, 0700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "pith")
	if err := os.WriteFile(source, []byte("not executable"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := setupCodexPreToolUseHook(home, source, []byte("adapter")); err == nil {
		t.Fatal("non-executable Pith input unexpectedly accepted")
	}
}

func TestCodexHookInstallRejectsRelativeInputPaths(t *testing.T) {
	if _, err := setupCodexPreToolUseHook(t.TempDir(), "pith", []byte("adapter")); err == nil {
		t.Fatal("relative executable paths unexpectedly accepted")
	}
}

func TestCodexConfigReadRejectsMalformedJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "hooks.json")
	if err := os.WriteFile(path, []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := readCodexHookConfig(path); err == nil {
		t.Fatal("malformed JSON unexpectedly accepted")
	}
}

func TestCodexHookInstallRejectsEmptyAdapter(t *testing.T) {
	root := t.TempDir()
	home := filepath.Join(root, "home")
	source := fakeExecutable(t, filepath.Join(root, "source", "pith"), []byte("fake"))
	_, err := setupCodexPreToolUseHook(home, source, nil)
	if err == nil || !strings.Contains(err.Error(), "Codex adapter is empty") {
		t.Fatalf("empty adapter error = %v", err)
	}
}

func TestSetupCodexHookRefusesGoTestExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Codex hook command uses a POSIX shell on supported installer platforms")
	}
	if _, err := SetupCodexPreToolUseHook([]byte("fixture adapter")); err == nil || !strings.Contains(err.Error(), "Go test executable") {
		t.Fatalf("test executable install result = %v, want refusal before filesystem changes", err)
	}
}
