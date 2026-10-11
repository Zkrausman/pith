package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

const codexHookStatusMessage = "Pith: checking Bash command (managed by pith --hooks)"

// CodexHookInstallResult describes the stable paths written by pith --hooks.
type CodexHookInstallResult struct {
	SettingsPath string
	AdapterPath  string
	BinaryPath   string
	Backups      []string
}

// SetupCodexPreToolUseHook installs the current Pith executable and embedded
// Codex adapter at stable user paths, then adds one PreToolUse/Bash handler.
// Codex trust is deliberately left to the user.
func SetupCodexPreToolUseHook(adapter []byte) (CodexHookInstallResult, error) {
	if runtime.GOOS == "windows" {
		return CodexHookInstallResult{}, errors.New("pith --hooks currently supports Linux and macOS only")
	}
	if len(adapter) == 0 {
		return CodexHookInstallResult{}, errors.New("embedded Codex adapter is empty")
	}
	executable, err := os.Executable()
	if err != nil {
		return CodexHookInstallResult{}, fmt.Errorf("locate Pith executable: %w", err)
	}
	if isGoTestExecutable(executable) {
		return CodexHookInstallResult{}, errors.New("refusing to install from a Go test executable")
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return CodexHookInstallResult{}, fmt.Errorf("resolve Pith executable: %w", err)
	}
	pythonExecutable, err := findCodexHookPython()
	if err != nil {
		return CodexHookInstallResult{}, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return CodexHookInstallResult{}, fmt.Errorf("locate user home: %w", err)
	}
	return setupCodexPreToolUseHookWithPython(home, executable, pythonExecutable, adapter)
}

func setupCodexPreToolUseHook(home, sourceBinary string, adapter []byte) (result CodexHookInstallResult, returnErr error) {
	return setupCodexPreToolUseHookWithPython(home, sourceBinary, filepath.Join(string(os.PathSeparator), "usr", "bin", "python3"), adapter)
}

func findCodexHookPython() (string, error) {
	pythonExecutable, err := exec.LookPath("python3")
	if err != nil {
		return "", fmt.Errorf("find supported Python 3 interpreter: %w", err)
	}
	pythonExecutable, err = filepath.Abs(pythonExecutable)
	if err != nil {
		return "", fmt.Errorf("resolve supported Python 3 interpreter path: %w", err)
	}
	pythonExecutable, err = filepath.EvalSymlinks(pythonExecutable)
	if err != nil {
		return "", fmt.Errorf("resolve supported Python 3 interpreter: %w", err)
	}
	info, err := os.Stat(pythonExecutable)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 {
		return "", fmt.Errorf("supported Python 3 interpreter is unavailable or not executable: %s", pythonExecutable)
	}
	return pythonExecutable, nil
}

func setupCodexPreToolUseHookWithPython(home, sourceBinary, pythonExecutable string, adapter []byte) (result CodexHookInstallResult, returnErr error) {
	if runtime.GOOS == "windows" {
		return CodexHookInstallResult{}, errors.New("pith --hooks currently supports Linux and macOS only")
	}
	if len(adapter) == 0 {
		return CodexHookInstallResult{}, errors.New("Codex adapter is empty")
	}
	var err error
	if home, err = filepath.Abs(home); err != nil {
		return result, fmt.Errorf("resolve home directory: %w", err)
	}
	if !filepath.IsAbs(sourceBinary) {
		return result, errors.New("Pith path must be absolute")
	}
	if !filepath.IsAbs(pythonExecutable) {
		return result, errors.New("Python 3 interpreter path must be absolute")
	}
	binaryInfo, err := os.Stat(sourceBinary)
	if err != nil || !binaryInfo.Mode().IsRegular() || binaryInfo.Mode().Perm()&0111 == 0 {
		return result, fmt.Errorf("Pith executable is unavailable or not executable: %s", sourceBinary)
	}
	binaryData, err := os.ReadFile(sourceBinary)
	if err != nil {
		return result, fmt.Errorf("read Pith executable: %w", err)
	}

	result.BinaryPath = filepath.Join(home, ".pith", "bin", installedBinaryName())
	result.AdapterPath = filepath.Join(home, ".pith", "hooks", "codex", "pre_tool_use_pith.py")
	result.SettingsPath = filepath.Join(home, ".codex", "hooks.json")
	for _, dir := range []string{
		filepath.Dir(result.BinaryPath), filepath.Dir(result.AdapterPath), filepath.Dir(result.SettingsPath),
	} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			return result, fmt.Errorf("create Codex hook directory %s: %w", dir, err)
		}
	}

	lock, err := acquireCodexHookLock(result.SettingsPath + ".pith.lock")
	if err != nil {
		return result, err
	}
	defer func() {
		if err := lock.release(); err != nil {
			returnErr = errors.Join(returnErr, err)
		}
	}()

	settings, snapshot, err := readCodexHookConfig(result.SettingsPath)
	if err != nil {
		return result, err
	}
	command := quotePOSIX(pythonExecutable) + " -I -S " + quotePOSIX(result.AdapterPath) + " --pith-binary " + quotePOSIX(result.BinaryPath)
	updatedSettings, err := withCodexPreToolUseHook(settings, command, result.AdapterPath, result.BinaryPath)
	if err != nil {
		return result, err
	}
	settingsMode := fs.FileMode(0600)
	if snapshot.exists {
		settingsMode = snapshot.info.Mode().Perm()
	}

	changes := []desiredFile{
		{path: result.BinaryPath, data: binaryData, mode: 0755},
		{path: result.AdapterPath, data: adapter, mode: 0644},
		{path: result.SettingsPath, data: updatedSettings, mode: settingsMode, snapshot: &snapshot},
	}
	result.Backups, err = applyFileTransaction(changes)
	if err != nil {
		return result, err
	}
	return result, nil
}

func quotePOSIX(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\"'\"'") + "'"
}

type codexHookLock struct {
	path string
	file *os.File
}

func acquireCodexHookLock(path string) (*codexHookLock, error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("another pith --hooks install is active, or a stale lock exists at %s; verify no install is running before removing it: %w", path, err)
		}
		return nil, fmt.Errorf("create Codex hooks lock %s: %w", path, err)
	}
	if _, err := fmt.Fprintf(file, "pid=%d\n", os.Getpid()); err != nil {
		file.Close()
		os.Remove(path)
		return nil, fmt.Errorf("write Codex hooks lock: %w", err)
	}
	if err := file.Sync(); err != nil {
		file.Close()
		os.Remove(path)
		return nil, fmt.Errorf("sync Codex hooks lock: %w", err)
	}
	return &codexHookLock{path: path, file: file}, nil
}

func (lock *codexHookLock) release() error {
	if lock == nil {
		return nil
	}
	closeErr := lock.file.Close()
	removeErr := os.Remove(lock.path)
	if errors.Is(removeErr, os.ErrNotExist) {
		removeErr = nil
	}
	if err := errors.Join(closeErr, removeErr); err != nil {
		return fmt.Errorf("Codex hooks install left a lock that may need cleanup at %s: %w", lock.path, err)
	}
	return nil
}

type fileSnapshot struct {
	exists bool
	data   []byte
	info   os.FileInfo
}

func captureFile(path string) (fileSnapshot, error) {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return fileSnapshot{}, nil
	}
	if err != nil {
		return fileSnapshot{}, fmt.Errorf("inspect %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return fileSnapshot{}, fmt.Errorf("refusing to replace non-regular file %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileSnapshot{}, fmt.Errorf("read %s: %w", path, err)
	}
	after, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, after) || info.Mode() != after.Mode() {
		return fileSnapshot{}, fmt.Errorf("%s changed while it was being read", path)
	}
	return fileSnapshot{exists: true, data: data, info: info}, nil
}

func sameFileSnapshot(path string, before fileSnapshot) (bool, error) {
	current, err := captureFile(path)
	if err != nil {
		return false, err
	}
	if current.exists != before.exists {
		return false, nil
	}
	if !before.exists {
		return true, nil
	}
	return os.SameFile(current.info, before.info) && current.info.Mode() == before.info.Mode() && bytes.Equal(current.data, before.data), nil
}

type desiredFile struct {
	path     string
	data     []byte
	mode     fs.FileMode
	snapshot *fileSnapshot
}

type stagedFile struct {
	desired   desiredFile
	before    fileSnapshot
	tempPath  string
	backup    string
	after     os.FileInfo
	changed   bool
	committed bool
}

func applyFileTransaction(desired []desiredFile) ([]string, error) {
	return applyFileTransactionWithHook(desired, nil)
}

func applyFileTransactionWithHook(desired []desiredFile, beforeReplace func(string) error) ([]string, error) {
	staged := make([]*stagedFile, 0, len(desired))
	cleanupTemps := func() {
		for _, file := range staged {
			if file.tempPath != "" {
				_ = os.Remove(file.tempPath)
			}
		}
	}
	for _, item := range desired {
		before, err := captureFile(item.path)
		if err != nil {
			cleanupTemps()
			return nil, err
		}
		if item.snapshot != nil {
			before = *item.snapshot
		}
		change := &stagedFile{desired: item, before: before}
		modeMatches := before.exists && before.info.Mode().Perm() == item.mode.Perm()
		if before.exists && modeMatches && bytes.Equal(before.data, item.data) {
			staged = append(staged, change)
			continue
		}
		stage, err := stageContent(item.path, item.data, item.mode)
		if err != nil {
			cleanupTemps()
			return nil, err
		}
		change.tempPath = stage
		change.changed = true
		staged = append(staged, change)
	}

	var backups []string
	rollback := func() []error {
		var failures []error
		for i := len(staged) - 1; i >= 0; i-- {
			file := staged[i]
			if file.committed {
				if err := rollbackStagedFile(file); err != nil {
					failures = append(failures, err)
				}
			}
		}
		return failures
	}
	for _, file := range staged {
		if !file.changed {
			continue
		}
		if file.before.exists {
			backup, err := createBackup(file.desired.path, file.before)
			if err != nil {
				cleanupTemps()
				return backups, errors.Join(append([]error{err}, rollback()...)...)
			}
			file.backup = backup
			backups = append(backups, backup)
			if err := syncDirectory(filepath.Dir(file.desired.path)); err != nil {
				cleanupTemps()
				return backups, errors.Join(append([]error{fmt.Errorf("sync backup directory for %s: %w", file.desired.path, err)}, rollback()...)...)
			}
		}
		if beforeReplace != nil {
			if err := beforeReplace(file.desired.path); err != nil {
				cleanupTemps()
				return backups, errors.Join(append([]error{fmt.Errorf("before replacing %s: %w", file.desired.path, err)}, rollback()...)...)
			}
		}
		same, err := sameFileSnapshot(file.desired.path, file.before)
		if err != nil || !same {
			if err == nil {
				err = fmt.Errorf("%s changed during installation; no concurrent edit was overwritten", file.desired.path)
			}
			cleanupTemps()
			return backups, errors.Join(append([]error{err}, rollback()...)...)
		}
		if err := os.Rename(file.tempPath, file.desired.path); err != nil {
			cleanupTemps()
			return backups, errors.Join(append([]error{fmt.Errorf("replace %s: %w", file.desired.path, err)}, rollback()...)...)
		}
		file.tempPath = ""
		file.committed = true
		file.after, _ = os.Lstat(file.desired.path)
		if err := syncDirectory(filepath.Dir(file.desired.path)); err != nil {
			cleanupTemps()
			return backups, errors.Join(append([]error{fmt.Errorf("sync directory for %s: %w", file.desired.path, err)}, rollback()...)...)
		}
	}
	cleanupTemps()
	return backups, nil
}

func stageContent(target string, data []byte, mode fs.FileMode) (string, error) {
	dir := filepath.Dir(target)
	file, err := os.CreateTemp(dir, ".pith-stage-*")
	if err != nil {
		return "", fmt.Errorf("stage %s: %w", target, err)
	}
	path := file.Name()
	failed := func(err error) (string, error) {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("stage %s: %w", target, err)
	}
	n, err := file.Write(data)
	if err != nil {
		return failed(err)
	}
	if n != len(data) {
		return failed(fmt.Errorf("short write: wrote %d of %d bytes", n, len(data)))
	}
	if err := file.Chmod(mode.Perm()); err != nil {
		return failed(err)
	}
	if err := file.Sync(); err != nil {
		return failed(err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close staged %s: %w", target, err)
	}
	return path, nil
}

func createBackup(target string, snapshot fileSnapshot) (string, error) {
	file, err := os.CreateTemp(filepath.Dir(target), filepath.Base(target)+".pith-backup-*")
	if err != nil {
		return "", fmt.Errorf("create backup for %s: %w", target, err)
	}
	path := file.Name()
	remove := func(err error) (string, error) {
		_ = file.Close()
		_ = os.Remove(path)
		return "", fmt.Errorf("write backup for %s: %w", target, err)
	}
	n, err := file.Write(snapshot.data)
	if err != nil {
		return remove(err)
	}
	if n != len(snapshot.data) {
		return remove(fmt.Errorf("short write: wrote %d of %d bytes", n, len(snapshot.data)))
	}
	if err := file.Chmod(snapshot.info.Mode().Perm()); err != nil {
		return remove(err)
	}
	if err := file.Sync(); err != nil {
		return remove(err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close backup for %s: %w", target, err)
	}
	return path, nil
}

func rollbackStagedFile(file *stagedFile) error {
	current, err := captureFile(file.desired.path)
	if err != nil {
		return fmt.Errorf("inspect %s before rollback: %w", file.desired.path, err)
	}
	if !current.exists || !bytes.Equal(current.data, file.desired.data) || current.info.Mode().Perm() != file.desired.mode.Perm() || (file.after != nil && !os.SameFile(current.info, file.after)) {
		backupNote := "no original backup exists"
		if file.backup != "" {
			backupNote = "backup retained at " + file.backup
		}
		return fmt.Errorf("cannot roll back %s because it changed after replacement; %s", file.desired.path, backupNote)
	}
	if file.before.exists {
		restore, err := stageContent(file.desired.path, file.before.data, file.before.info.Mode())
		if err != nil {
			return err
		}
		if err := os.Rename(restore, file.desired.path); err != nil {
			_ = os.Remove(restore)
			return fmt.Errorf("restore %s from backup %s: %w", file.desired.path, file.backup, err)
		}
	} else if err := os.Remove(file.desired.path); err != nil {
		return fmt.Errorf("remove new %s during rollback: %w", file.desired.path, err)
	}
	if err := syncDirectory(filepath.Dir(file.desired.path)); err != nil {
		return fmt.Errorf("sync rollback for %s: %w", file.desired.path, err)
	}
	return nil
}

func syncDirectory(path string) error {
	dir, err := os.Open(path)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

func readCodexHookConfig(path string) (map[string]json.RawMessage, fileSnapshot, error) {
	snapshot, err := captureFile(path)
	if err != nil {
		return nil, snapshot, err
	}
	if !snapshot.exists {
		return make(map[string]json.RawMessage), snapshot, nil
	}
	if err := rejectDuplicateJSONKeys(snapshot.data); err != nil {
		return nil, snapshot, fmt.Errorf("refusing to overwrite ambiguous Codex hooks file %s: %w", path, err)
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(snapshot.data, &root); err != nil {
		return nil, snapshot, fmt.Errorf("refusing to overwrite malformed Codex hooks file %s: %w", path, err)
	}
	if root == nil {
		return nil, snapshot, fmt.Errorf("refusing to overwrite non-object Codex hooks file %s", path)
	}
	return root, snapshot, nil
}

func rejectDuplicateJSONKeys(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := consumeJSONValue(decoder, 0); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		return errors.New("multiple JSON values")
	}
	return nil
}

func consumeJSONValue(decoder *json.Decoder, depth int) error {
	if depth > 128 {
		return errors.New("JSON nesting exceeds 128 levels")
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate object key %q", key)
			}
			seen[key] = struct{}{}
			if err := consumeJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim('}') {
			return errors.New("unterminated object")
		}
	case '[':
		for decoder.More() {
			if err := consumeJSONValue(decoder, depth+1); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil || closing != json.Delim(']') {
			return errors.New("unterminated array")
		}
	default:
		return fmt.Errorf("unexpected delimiter %q", delimiter)
	}
	return nil
}

func withCodexPreToolUseHook(root map[string]json.RawMessage, command, adapterPath, binaryPath string) ([]byte, error) {
	root = cloneRawMap(root)
	var hooks map[string]json.RawMessage
	if raw, ok := root["hooks"]; ok {
		if err := json.Unmarshal(raw, &hooks); err != nil || hooks == nil {
			return nil, errors.New("refusing to overwrite malformed hooks object in Codex hooks.json")
		}
	} else {
		hooks = make(map[string]json.RawMessage)
	}

	var groups []json.RawMessage
	if raw, ok := hooks["PreToolUse"]; ok {
		if err := json.Unmarshal(raw, &groups); err != nil || groups == nil {
			return nil, errors.New("refusing to overwrite malformed PreToolUse list in Codex hooks.json")
		}
	}
	newHandler, err := json.Marshal(map[string]string{
		"type":          "command",
		"command":       command,
		"statusMessage": codexHookStatusMessage,
	})
	if err != nil {
		return nil, err
	}

	updatedGroups := make([]json.RawMessage, 0, len(groups)+1)
	managedHandlerFound := false
	for _, rawGroup := range groups {
		var group map[string]json.RawMessage
		if err := json.Unmarshal(rawGroup, &group); err != nil || group == nil {
			return nil, errors.New("refusing to overwrite malformed PreToolUse matcher group in Codex hooks.json")
		}
		rawHandlers, ok := group["hooks"]
		if !ok {
			return nil, errors.New("refusing to overwrite PreToolUse matcher group without handlers in Codex hooks.json")
		}
		var handlers []json.RawMessage
		if err := json.Unmarshal(rawHandlers, &handlers); err != nil || handlers == nil {
			return nil, errors.New("refusing to overwrite malformed PreToolUse handlers in Codex hooks.json")
		}
		filtered := make([]json.RawMessage, 0, len(handlers))
		groupHasManagedHandler := false
		for _, rawHandler := range handlers {
			var handler map[string]json.RawMessage
			if err := json.Unmarshal(rawHandler, &handler); err != nil || handler == nil {
				return nil, errors.New("refusing to overwrite malformed PreToolUse handler in Codex hooks.json")
			}
			var status string
			var existingCommand string
			if marker, ok := handler["statusMessage"]; ok && json.Unmarshal(marker, &status) == nil && status == codexHookStatusMessage && json.Unmarshal(handler["command"], &existingCommand) == nil && isManagedCodexCommand(existingCommand, adapterPath, binaryPath) {
				if managedHandlerFound {
					return nil, errors.New("refusing to rewrite duplicate managed Pith PreToolUse handlers in Codex hooks.json")
				}
				managedHandlerFound = true
				groupHasManagedHandler = true
				handler["type"], err = json.Marshal("command")
				if err != nil {
					return nil, err
				}
				handler["command"], err = json.Marshal(command)
				if err != nil {
					return nil, err
				}
				handler["statusMessage"], err = json.Marshal(codexHookStatusMessage)
				if err != nil {
					return nil, err
				}
				updatedHandler, err := marshalRawObject(handler)
				if err != nil {
					return nil, err
				}
				filtered = append(filtered, updatedHandler)
				continue
			}
			filtered = append(filtered, rawHandler)
		}
		if !groupHasManagedHandler {
			updatedGroups = append(updatedGroups, rawGroup)
			continue
		}
		group["hooks"], err = json.Marshal(filtered)
		if err != nil {
			return nil, err
		}
		updatedGroup, err := marshalRawObject(group)
		if err != nil {
			return nil, err
		}
		updatedGroups = append(updatedGroups, updatedGroup)
	}
	if !managedHandlerFound {
		newGroup, err := json.Marshal(map[string]any{
			"matcher": "^Bash$",
			"hooks":   []json.RawMessage{newHandler},
		})
		if err != nil {
			return nil, err
		}
		updatedGroups = append(updatedGroups, newGroup)
	}
	hooks["PreToolUse"], err = json.Marshal(updatedGroups)
	if err != nil {
		return nil, err
	}
	root["hooks"], err = json.Marshal(hooks)
	if err != nil {
		return nil, err
	}
	result, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(result, '\n'), nil
}

func isManagedCodexCommand(command, adapterPath, binaryPath string) bool {
	suffix := " -I -S " + quotePOSIX(adapterPath) + " --pith-binary " + quotePOSIX(binaryPath)
	if !strings.HasSuffix(command, suffix) {
		return false
	}
	interpreter := strings.TrimSuffix(command, suffix)
	if interpreter == quotePOSIX("python3") {
		return true // Recognize hooks written by the earlier pith --hooks release.
	}
	pythonPath, ok := unquotePOSIX(interpreter)
	return ok && filepath.IsAbs(pythonPath)
}

func unquotePOSIX(value string) (string, bool) {
	if len(value) < 2 || value[0] != '\'' {
		return "", false
	}
	var result strings.Builder
	for i := 1; i < len(value); {
		if value[i] != '\'' {
			result.WriteByte(value[i])
			i++
			continue
		}
		if i == len(value)-1 {
			return result.String(), true
		}
		const escapedQuote = `'"'"'`
		if !strings.HasPrefix(value[i:], escapedQuote) {
			return "", false
		}
		result.WriteByte('\'')
		i += len(escapedQuote)
	}
	return "", false
}

func cloneRawMap(input map[string]json.RawMessage) map[string]json.RawMessage {
	copy := make(map[string]json.RawMessage, len(input)+1)
	for key, value := range input {
		copy[key] = append(json.RawMessage(nil), value...)
	}
	return copy
}

func marshalRawObject(value map[string]json.RawMessage) (json.RawMessage, error) {
	return json.Marshal(value)
}
