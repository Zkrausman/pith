#!/usr/bin/env python3
"""Codex Bash PreToolUse adapter for bounded literal external commands."""

from __future__ import annotations

import argparse
import errno
import json
import os
import re
import select
import shlex
import signal
import subprocess
import sys
import time
from typing import Callable, Sequence


CAPTURE_LIMIT = 1 << 20
READ_CHUNK = 32 << 10
CAPTURE_SECONDS = 30.0
TRANSFORM_SECONDS = 2.0
TRANSFORM_OUTPUT_LIMIT = 1 << 20
HOOK_INPUT_LIMIT = 64 << 10
RECORD_TIMEOUT_SECONDS = 0.15
RECORD_MAX_RUNES = 1 << 20
RECORD_MAX_DURATION_MS = 24 * 60 * 60 * 1000
RECORD_MAX_BYTES = 1 << 40
RECORD_MAX_REQUEST = 2048
_CANCEL_SIGNALS = {signal.SIGINT, signal.SIGTERM}
_CANCEL_SIGNAL: int | None = None
_SUMMARY_OK = re.compile(r"ok\s+\S+\s+\d+(?:\.\d+)?s\Z")
_SELECTION_ERROR = re.compile(r"(?i)\[FAIL\]|FAILED|ERROR|panic|traceback|exception|fatal")
_SELECTION_WARNING = re.compile(r"(?im)^\s*(warning|warn(ing)?\s*:|npm\s+warn\b|\[[^\]]*\bwarn(?:ing)?\b[^\]]*\]|\d{4}-\d\d-\d\d[^\n]*(warning|warn)|⚠)")
_SELECTION_DIFF = re.compile(r"(?m)^(?:diff --git|@@ |--- (?!PASS:)|\+\+\+ )")
_SELECTION_TRUNCATION = re.compile(r"(?i)(output|content|results?)\s+(was\s+)?(truncated|trimmed)|truncated\s+by\s+(the\s+)?(host|tool|runner)|\.\.\.\s*output\s+truncated|\.\.\.\s*\(truncated|\[showing\s+(lines|last)\b[^\]]*full output\s*:")
_RUN = re.compile(r"=== RUN\s+(\S+)\Z")
_PAUSE = re.compile(r"=== PAUSE\s+\S+\Z")
_CONT = re.compile(r"=== CONT\s+\S+\Z")
_PASS = re.compile(r"--- PASS:\s+(\S+)\s+\(\d+(?:\.\d+)?s\)\Z")

_FAMILY_OTHER = "other/compound"
_BYPASS_REASONS = frozenset({
    "invalid_literal", "shell_syntax", "shell_reserved_or_assignment",
    "interpreter", "recursive_wrapper", "shell_builtin", "shell_environment",
    "execution_cwd_unavailable", "executable_unresolved_or_format", "wrapper_encoding_failed",
})
_COMMAND_SUBCOMMANDS = {
    "git": {"status", "diff", "log", "show", "branch"},
    "go": {"test", "build", "run", "vet"},
    "npm": {"test", "run", "install", "ci"},
    "pnpm": {"test", "run", "install"},
    "yarn": {"test", "run", "install"},
    "bun": {"test", "run", "install"},
}
_SIMPLE_FAMILIES = {
    "pytest": "pytest", "pytest-3": "pytest", "node": "node", "npx": "npx",
    "cargo": "cargo", "rustc": "rustc", "make": "make", "cmake": "cmake",
    "docker": "docker", "kubectl": "kubectl", "terraform": "terraform",
    "rg": "rg", "grep": "grep", "find": "find", "ls": "ls", "cat": "cat",
}
_CODEX_PROPOSAL_FAMILIES = {
    "git status", "git diff", "git log", "git show", "git branch",
    "go test", "go build", "go run", "go vet",
    "npm test", "npm run", "npm install", "npm ci",
    "pnpm test", "pnpm run", "pnpm install",
    "yarn test", "yarn run", "yarn install",
    "bun test", "bun run", "bun install",
    "python", "pytest", "node", "npx", "cargo", "rustc", "make", "cmake",
    "docker", "kubectl", "terraform", "rg", "grep", "find", "ls", "cat", _FAMILY_OTHER,
}
_PARSER_REGISTRY_NAMES = frozenset({
    "git_composite", "git_status", "git_log", "git_show", "git_diff", "git_branch",
    "ls", "find", "tree", "du", "grep", "minify", "source", "env", "docker_ps",
    "gh_release", "github", "dependencies", "tests", "go_cover", "web-content",
    "pith-internal", "windows-shell", "get-content", "go", "vitest", "bd",
    "promptfoo", "snag", "node", "npm", "chain",
})
# This is a selection-boundary allowlist, not a replacement parser registry:
# Go still performs matching and transformation. It prevents a transform
# response from claiming a registered parser that cannot apply to the argv
# that the adapter actually executed.
_DIRECT_PARSER_EXECUTABLES = {
    "git_status": {"git"}, "git_log": {"git"}, "git_show": {"git"},
    "git_diff": {"git"}, "git_branch": {"git"},
    "find": {"find", "where"}, "tree": {"tree"}, "du": {"du"},
    "grep": {"rg", "grep", "ripgrep"}, "minify": {"cat", "type"},
    "docker_ps": {"docker"}, "gh_release": {"gh"}, "github": {"gh"},
    "dependencies": {"npm", "pip"}, "tests": {"go", "npm", "pytest"},
    "go_cover": {"go"}, "web-content": {"curl", "wget", "invoke-webrequest", "iwr"},
    "go": {"go"}, "vitest": {"vitest", "npx"}, "bd": {"bd"},
    "promptfoo": {"promptfoo", "npx"}, "snag": {"snag"}, "npm": {"npm"},
}

_INTERPRETERS = {
    "ash", "awk", "bash", "cmd", "cmd.exe", "clisp", "clojure", "csi", "dash", "deno",
    "dotnet", "elixir", "erl", "escript", "fish", "ghci", "groovy", "iex", "java", "js",
    "julia", "ksh", "lein", "lua", "node", "nodejs", "ocaml", "ocamlrun", "perl", "php",
    "powershell", "powershell.exe", "pwsh", "python", "python2", "python3", "ruby", "r", "rscript",
    "runghc", "sbcl", "scala", "scalac", "scheme", "sh", "swift", "swiftc", "tclsh", "tcsh", "zsh",
}
_VERSIONED_INTERPRETER = re.compile(r"^(?:python\d+(?:\.\d+)*|pypy\d*|nodejs?|ruby\d+(?:\.\d+)*|php\d+(?:\.\d+)*|perl\d+(?:\.\d+)*)$")
_RECURSIVE_WRAPPERS = {"pith", "env", "command", "exec", "nohup", "nice", "setsid", "sudo", "doas", "timeout", "xargs", "busybox"}
_SHELL_BUILTINS = {
    ".", "alias", "bg", "break", "builtin", "cd", "command", "continue", "declare",
    "echo", "eval", "exec", "exit", "export", "false", "fc", "fg", "getopts", "hash",
    "help", "history", "jobs", "let", "local", "logout", "mapfile", "popd", "printf",
    "pushd", "pwd", "read", "readarray", "readonly", "return", "set", "shift", "source",
    "suspend", "test", "times", "trap", "true", "type", "typeset", "ulimit", "umask",
    "unalias", "unset", "wait", "[",
}
_SHELL_RESERVED = {"!", "[[", "]]", "{", "}", "case", "coproc", "do", "done", "elif", "else", "esac", "fi", "for", "function", "if", "select", "then", "time", "until", "while"}
_ASSIGNMENT = re.compile(r"^[A-Za-z_][A-Za-z0-9_]*=")


def _literal_external_argv(command: str, on_bypass: Callable[[str], None] | None = None) -> list[str] | None:
    """Accept a bounded argv-only external command, never shell evaluation."""
    def reject(reason: str) -> None:
        if on_bypass is not None:
            on_bypass(reason)
        return None

    if not command:
        return reject("invalid_literal")
    try:
        if len(command.encode("utf-8", errors="strict")) > HOOK_INPUT_LIMIT:
            return reject("invalid_literal")
    except UnicodeError:
        return reject("invalid_literal")
    quote: str | None = None
    escaped = False
    for ch in command:
        if ch in "\x00\r\n":
            return reject("shell_syntax")
        if escaped:
            if quote == '"' and ch not in '$`"\\':
                # Bash preserves the backslash here; shlex.split does not.
                # Avoid executing a different argv than the original shell.
                return reject("invalid_literal")
            escaped = False
            continue
        if quote == "'":
            if ch == "'":
                quote = None
            continue
        if ch == "\\" and quote != "'":
            escaped = True
            continue
        if quote == '"':
            if ch == '"':
                quote = None
            elif ch in "$`":
                return reject("shell_syntax")
            continue
        if quote is None:
            if ch in "'\"":
                quote = ch
                continue
            if ch in ";|&()<>`$*?[]{}~#!":
                return reject("shell_syntax")
    if quote is not None or escaped:
        return reject("invalid_literal")
    try:
        argv = shlex.split(command, posix=True)
        encoded = [part.encode("utf-8", errors="strict") for part in argv]
    except (ValueError, UnicodeError):
        return reject("invalid_literal")
    if not argv or len(argv) > 256 or sum(map(len, encoded)) > 64 << 10:
        return reject("invalid_literal")
    if argv[0] in _SHELL_RESERVED or _ASSIGNMENT.match(argv[0]):
        return reject("shell_reserved_or_assignment")
    if not argv[0] or any("\x00" in part for part in argv):
        return reject("invalid_literal")
    return argv


def _family_for_argv(argv: Sequence[str]) -> str:
    executable = os.path.basename(argv[0].replace("\\", "/")).lower()
    if executable.endswith(".exe"):
        executable = executable[:-4]
    if executable in _COMMAND_SUBCOMMANDS:
        subcommand = argv[1].lower() if len(argv) > 1 else ""
        return f"{executable} {subcommand}" if subcommand in _COMMAND_SUBCOMMANDS[executable] else _FAMILY_OTHER
    if executable == "python" or (executable.startswith("python") and _VERSIONED_INTERPRETER.fullmatch(executable)):
        if len(argv) >= 3 and argv[1:3] == ["-m", "pytest"]:
            return "pytest"
        return "python"
    return _SIMPLE_FAMILIES.get(executable, _FAMILY_OTHER)


def _is_recursive_pith_form(argv: Sequence[str]) -> bool:
    name = os.path.basename(argv[0]).lower()
    command_name = name[:-4] if name.endswith(".exe") else name
    if command_name in _RECURSIVE_WRAPPERS or (command_name == "go" and len(argv) > 1 and argv[1] == "run"):
        return True
    package_launchers = {"npm", "npx", "pnpm", "yarn", "bun"}
    if command_name in package_launchers and any(
        os.path.basename(value.replace("\\", "/")).lower() in {"pith", "pith.exe"}
        for value in argv[1:]
    ):
        return True
    if command_name == "find":
        invokes_program = any(value in {"-exec", "-execdir", "-ok", "-okdir"} for value in argv[1:])
        invokes_pith = any(
            os.path.basename(value.replace("\\", "/")).lower() in {"pith", "pith.exe"}
            for value in argv[1:]
        )
        return invokes_program and invokes_pith
    return False


def _execution_cwd(tool_input: dict[str, object]) -> str | None:
    if "workdir" not in tool_input:
        try:
            return os.path.realpath(os.getcwd())
        except OSError:
            return None
    workdir = tool_input["workdir"]
    if not isinstance(workdir, str) or not os.path.isabs(workdir):
        return None
    resolved = os.path.realpath(workdir)
    return resolved if os.path.isdir(resolved) else None


def _supported_executable_format(path: str) -> bool:
    """Mirror Bash's direct-exec formats that this wrapper can preserve."""
    try:
        with open(path, "rb") as executable:
            magic = executable.read(4)
    except OSError:
        return False
    # On the supported Linux host, ELF binaries and kernel-interpreted
    # shebang scripts preserve Bash's native exec behavior. Plain executable
    # text relies on Bash's ENOEXEC shell fallback, which execve does not do.
    return magic.startswith(b"#!") or magic == b"\x7fELF"


def _resolve_literal_executable(
    argv: Sequence[str], *, cwd: str | None = None,
    on_bypass: Callable[[str], None] | None = None,
) -> str | None:
    def reject(reason: str) -> None:
        if on_bypass is not None:
            on_bypass(reason)
        return None

    name = os.path.basename(argv[0]).lower()
    command_name = name[:-4] if name.endswith(".exe") else name
    if name in _INTERPRETERS or command_name in _INTERPRETERS or _VERSIONED_INTERPRETER.fullmatch(command_name):
        return reject("interpreter")
    if _is_recursive_pith_form(argv):
        return reject("recursive_wrapper")
    if name in _SHELL_BUILTINS:
        return reject("shell_builtin")
    shell_options = set(os.environ.get("SHELLOPTS", "").split(":"))
    bash_options = set(os.environ.get("BASHOPTS", "").split(":"))
    if os.environ.get("BASH_ENV") or os.environ.get("ENV") or "expand_aliases" in shell_options | bash_options:
        return reject("shell_environment")
    if any(key.startswith("BASH_FUNC_") for key in os.environ):
        return reject("shell_environment")
    if cwd is None:
        try:
            cwd = os.path.realpath(os.getcwd())
        except OSError:
            return reject("execution_cwd_unavailable")
    if "/" in argv[0] or (os.altsep and os.altsep in argv[0]):
        execution_path = argv[0]
        validation_path = execution_path if os.path.isabs(execution_path) else os.path.join(cwd, execution_path)
        if (
            os.path.isfile(validation_path) and os.access(validation_path, os.X_OK) and
            _supported_executable_format(validation_path)
        ):
            # Keep the path spelling Bash supplied to execve. For shebang
            # scripts the kernel passes this filename to the interpreter, so
            # canonicalizing it changes $0 and relative resource lookup.
            return execution_path
        return reject("executable_unresolved_or_format")
    path_value = os.environ.get("PATH")
    if not path_value:
        return reject("executable_unresolved_or_format")
    for entry in path_value.split(os.pathsep):
        # Bash uses ./name for an empty PATH component and retains relative or
        # symlinked PATH components in the shebang filename passed to execve.
        directory = entry or "."
        separator = "" if directory.endswith(os.sep) else os.sep
        execution_path = directory + separator + argv[0]
        validation_path = (
            execution_path if os.path.isabs(execution_path)
            else os.path.join(cwd, execution_path)
        )
        if not os.path.isfile(validation_path) or not os.access(validation_path, os.X_OK):
            continue
        if _supported_executable_format(validation_path):
            return execution_path
        return reject("executable_unresolved_or_format")
    return reject("executable_unresolved_or_format")


def _allow(with_update: dict[str, object] | None = None) -> dict[str, object]:
    specific: dict[str, object] = {
        "hookEventName": "PreToolUse",
        "permissionDecision": "allow",
    }
    if with_update is not None:
        specific["updatedInput"] = with_update
    return {"hookSpecificOutput": specific}


def _proposal_family(command: str) -> str:
    """Map a proposed Bash command to a fixed label without retaining its text."""
    if not command or any(ch in command for ch in "\x00\r\n;&|()<>`$*?[]{}~"):
        return _FAMILY_OTHER
    try:
        argv = shlex.split(command, posix=True)
        if not argv:
            return _FAMILY_OTHER
        for value in argv:
            value.encode("utf-8", errors="strict")
    except (ValueError, UnicodeError):
        return _FAMILY_OTHER
    executable = os.path.basename(argv[0].replace("\\", "/")).lower()
    if executable.endswith(".exe"):
        executable = executable[:-4]
    if executable in _COMMAND_SUBCOMMANDS:
        subcommand = argv[1].lower() if len(argv) > 1 else ""
        return f"{executable} {subcommand}" if subcommand in _COMMAND_SUBCOMMANDS[executable] else _FAMILY_OTHER
    if executable == "python" or (executable.startswith("python") and _VERSIONED_INTERPRETER.fullmatch(executable)):
        if len(argv) >= 3 and argv[1:3] == ["-m", "pytest"]:
            return "pytest"
        return "python"
    return _SIMPLE_FAMILIES.get(executable, _FAMILY_OTHER)


def rewrite_hook_input(
    payload: object, pith_binary: str,
    record_proposal: Callable[[str, str], None] | None = None,
    record_bypass: Callable[[str, str, str], None] | None = None,
) -> dict[str, object]:
    if not isinstance(payload, dict) or payload.get("tool_name") != "Bash":
        return _allow()
    tool_input = payload.get("tool_input")
    if not isinstance(tool_input, dict) or not isinstance(tool_input.get("command"), str):
        return _allow()
    if not isinstance(pith_binary, str):
        return _allow()
    try:
        pith_available = os.path.isabs(pith_binary) and os.access(pith_binary, os.X_OK)
    except (OSError, UnicodeError, ValueError):
        pith_available = False
    if not pith_available:
        return _allow()
    command = tool_input["command"]
    reasons: list[str] = []
    native = _literal_external_argv(command, reasons.append)
    execution_cwd = _execution_cwd(tool_input)
    executable = _resolve_literal_executable(native, cwd=execution_cwd, on_bypass=reasons.append) if native is not None and execution_cwd else None
    if native is None or executable is None:
        reason = reasons[0] if reasons else "execution_cwd_unavailable"
        if record_bypass is not None:
            record_bypass(pith_binary, _proposal_family(command), reason)
        elif record_proposal is not None:
            record_proposal(pith_binary, _proposal_family(command))
        return _allow()
    wrapper = [
        os.path.realpath(sys.executable), "-I", "-S", os.path.realpath(__file__),
        "--run", "--pith-binary", pith_binary, "--resolved-executable", executable, "--", *native,
    ]
    try:
        updated = shlex.join(wrapper)
        updated.encode("utf-8", errors="strict")
    except UnicodeError:
        if record_bypass is not None:
            record_bypass(pith_binary, _proposal_family(command), "wrapper_encoding_failed")
        elif record_proposal is not None:
            record_proposal(pith_binary, _proposal_family(command))
        return _allow()
    # Preserve every caller-supplied option and change only the command.
    updated_input = dict(tool_input)
    updated_input["command"] = updated
    return _allow(updated_input)


def _must_exec_original() -> bool:
    try:
        for fd in (0, 1, 2):
            os.fstat(fd)
            if os.isatty(fd):
                return True
        return False
    except OSError:
        return True


def _write_all(fd: int, data: bytes, write: Callable[[int, bytes], int] = os.write) -> bool:
    view = memoryview(data)
    while view:
        try:
            count = write(fd, view)
        except InterruptedError:
            continue
        except OSError as exc:
            if exc.errno == errno.EINTR:
                continue
            return False
        if count <= 0:
            return False
        view = view[count:]
    return True


def _warn_delivery(message: str) -> None:
    _write_all(2, ("pith adapter: " + message + "\n").encode("utf-8", errors="replace"))


def _valid_go_verbose_success(raw: bytes) -> tuple[str, str] | None:
    try:
        text = raw.decode("utf-8", errors="strict")
    except UnicodeDecodeError:
        return None
    lines = [line.strip() for line in text.splitlines() if line.strip()]
    if len(lines) < 2 or lines[-2] != "PASS" or not _SUMMARY_OK.fullmatch(lines[-1]):
        return None
    runs: list[str] = []
    passes: list[str] = []
    summary_pass = False
    summary_ok = ""
    for line in lines:
        run_match = _RUN.fullmatch(line)
        pass_match = _PASS.fullmatch(line)
        if run_match:
            runs.append(run_match.group(1))
        elif _PAUSE.fullmatch(line) or _CONT.fullmatch(line):
            continue
        elif pass_match:
            passes.append(pass_match.group(1))
        elif line == "PASS":
            summary_pass = True
        elif _SUMMARY_OK.fullmatch(line):
            summary_ok = line
        else:
            return None
    if not runs or len(runs) != len(passes) or sorted(runs) != sorted(passes):
        return None
    if not summary_pass or not summary_ok:
        return None
    return "PASS", summary_ok


def _encode_selected_utf8(value: str) -> bytes | None:
    try:
        return value.encode("utf-8", errors="strict")
    except UnicodeEncodeError:
        return None


def _reject_json_constant(value: str) -> None:
    raise ValueError(value)


def _is_strict_json(value: str) -> bool:
    if not value.strip():
        return False
    try:
        json.loads(value, parse_constant=_reject_json_constant)
    except (json.JSONDecodeError, RecursionError, ValueError):
        return False
    return True


_GIT_GLOBAL_OPTIONS_WITH_VALUE = {
    "-C", "-c", "--git-dir", "--work-tree", "--namespace", "--super-prefix",
    "--config-env", "--attr-source",
}


def _git_subcommand_parts(argv: Sequence[str]) -> tuple[str, list[str]]:
    args = list(argv[1:])
    index = 0
    while index < len(args):
        arg = args[index]
        if not arg.startswith("-"):
            return arg, args[index:]
        if arg in _GIT_GLOBAL_OPTIONS_WITH_VALUE:
            index += 1
        index += 1
    return "", []


def _argv_program(argv: Sequence[str]) -> str:
    if not argv:
        return ""
    name = os.path.basename(argv[0].replace("\\", "/")).lower()
    return name[:-4] if name.endswith(".exe") else name


def _go_verbose_success_argv(argv: Sequence[str]) -> bool:
    if len(argv) != 4 or not os.path.isabs(argv[0]) or _argv_program(argv) != "go":
        return False
    package = argv[3]
    return (
        argv[1:3] == ["test", "-v"] and bool(package) and
        not package.startswith("-") and "..." not in package and
        re.fullmatch(r"(?:\.|\./[A-Za-z0-9_./-]+|[A-Za-z0-9_./-]+)", package) is not None
    )


def _direct_parser_identity_allowed(parser_name: str, argv: Sequence[str], raw_text: str) -> bool:
    if parser_name not in _PARSER_REGISTRY_NAMES or parser_name not in _DIRECT_PARSER_EXECUTABLES:
        return False
    executable = _argv_program(argv)
    if executable not in _DIRECT_PARSER_EXECUTABLES[parser_name]:
        return False
    args = list(argv[1:])
    if parser_name.startswith("git_"):
        subcommand, git_args = _git_subcommand_parts(argv)
        expected = {
            "git_status": {"status", "add", "commit", "push"},
            "git_log": {"log"}, "git_show": {"show"},
            "git_diff": {"diff"}, "git_branch": {"branch"},
        }.get(parser_name, set())
        if subcommand not in expected:
            return False
        if parser_name == "git_show":
            return len(git_args) == 1 or (len(git_args) == 2 and git_args[1] == "HEAD")
        if parser_name == "git_status" and subcommand == "status":
            if "->" in raw_text:
                return False
            if any(arg in {"-z", "--porcelain"} or arg.startswith("--porcelain=") for arg in git_args):
                records = re.split(r"[\x00\n]", raw_text)
                if any(record.startswith(("2 ", "R ", " R", "C ", " C")) for record in records):
                    return False
        return True
    if parser_name == "minify":
        return len(args) == 1 and os.path.splitext(args[0])[1].lower() == ".json"
    if parser_name == "docker_ps":
        return bool(args) and args[0] in {"ps", "images"}
    if parser_name == "gh_release":
        return len(args) >= 2 and args[0] == "release" and args[1] in {"view", "list"}
    if parser_name == "github":
        return bool(args) and args[0] == "status"
    if parser_name == "dependencies":
        return bool(args) and ((executable == "npm" and args[0] == "list") or
                               (executable == "pip" and args[0] == "list"))
    if parser_name == "tests":
        return executable == "pytest" or bool(args) and (
            (executable == "go" and args[0] == "test") or
            (executable == "npm" and args[0] == "test")
        )
    if parser_name == "go_cover":
        return len(args) >= 3 and args[:2] == ["tool", "cover"]
    if parser_name == "vitest":
        return executable == "vitest" or (executable == "npx" and bool(args) and _argv_program(args) == "vitest")
    if parser_name == "promptfoo":
        return executable == "promptfoo" or (executable == "npx" and bool(args) and _argv_program(args) == "promptfoo")
    return True


def _selected_output(
    raw: bytes, response_bytes: bytes, argv: Sequence[str],
) -> tuple[bytes, str] | None:
    try:
        original_text = raw.decode("utf-8", errors="strict")
    except UnicodeDecodeError:
        return None
    try:
        response = json.loads(response_bytes.decode("utf-8", errors="strict"))
    except (UnicodeDecodeError, json.JSONDecodeError, RecursionError):
        return None
    if not isinstance(response, dict):
        return None
    selected = response.get("output")
    parser_name = response.get("parser")
    if not isinstance(parser_name, str) or not parser_name or response.get("passthrough") is not False or not isinstance(selected, str):
        return None
    if not _direct_parser_identity_allowed(parser_name, argv, original_text):
        return None
    response_truncated = response.get("upstreamTruncated", False)
    if response_truncated is not False:
        return None
    for count_name in ("omittedLineCount", "omittedByteCount"):
        if count_name in response:
            count = response[count_name]
            if not isinstance(count, int) or isinstance(count, bool) or count != 0:
                return None
    validated_go_success = parser_name == "tests" and _go_verbose_success_argv(argv) and _valid_go_verbose_success(raw) is not None
    if parser_name == "tests" and _argv_program(argv) == "go" and not validated_go_success:
        return None
    if _SELECTION_ERROR.search(original_text) and not validated_go_success:
        return None
    if (
        _SELECTION_WARNING.search(original_text) or _SELECTION_DIFF.search(original_text) or
        _SELECTION_TRUNCATION.search(original_text)
    ):
        return None
    if _is_strict_json(original_text) and parser_name != "minify":
        return None
    encoded = _encode_selected_utf8(selected)
    if encoded is None:
        return None
    original_count = response.get("originalByteCount")
    retained_count = response.get("retainedByteCount")
    if (
        not isinstance(original_count, int) or isinstance(original_count, bool) or
        not isinstance(retained_count, int) or isinstance(retained_count, bool) or
        original_count != len(raw) or retained_count != len(encoded)
    ):
        return None
    if not encoded or len(encoded) >= len(raw) or len(selected) >= len(original_text):
        return None
    return encoded, parser_name


def _record_wrapper_outcome(
    pith_binary: str, original: bytes, delivered: bytes, duration_ms: int,
    outcome: str, family: str, parser_name: str, *, timeout: float = RECORD_TIMEOUT_SECONDS,
) -> None:
    """Best-effort aggregate accounting after stdout publication; never retain text."""
    if (
        outcome not in ("transformed", "raw_fallback") or family not in _CODEX_PROPOSAL_FAMILIES or
        duration_ms < 0 or duration_ms > RECORD_MAX_DURATION_MS or
        (outcome == "transformed" and not parser_name) or
        (outcome == "raw_fallback" and parser_name)
    ):
        return
    try:
        original_text = original.decode("utf-8", errors="strict")
        delivered_text = delivered.decode("utf-8", errors="strict")
        original_runes = len(original_text)
        delivered_runes = len(delivered_text)
        if original_runes > RECORD_MAX_RUNES or delivered_runes > RECORD_MAX_RUNES:
            return
        if outcome == "raw_fallback" and (original != delivered or original_runes != delivered_runes):
            return
        if outcome == "transformed" and (len(delivered) >= len(original) or delivered_runes >= original_runes):
            return
        request = json.dumps(
            {"kind": "execution", "family": family, "parser": parser_name,
             "originalRunes": original_runes, "deliveredRunes": delivered_runes,
             "durationMs": duration_ms, "outcome": outcome},
            separators=(",", ":"), allow_nan=False,
        ).encode("ascii")
        if len(request) > RECORD_MAX_REQUEST or not os.path.isabs(pith_binary):
            return
        deadline = time.monotonic() + max(0.001, timeout)
        process = subprocess.Popen(
            [pith_binary, "_record-codex-wrapper"], stdin=subprocess.PIPE,
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, close_fds=True,
        )
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            try:
                process.kill()
            except OSError:
                pass
            try:
                process.communicate(timeout=0.05)
            except (OSError, subprocess.TimeoutExpired):
                pass
            return
        try:
            process.communicate(request, timeout=remaining)
        except subprocess.TimeoutExpired:
            try:
                process.kill()
            except OSError:
                pass
            try:
                process.communicate(timeout=0.05)
            except (OSError, subprocess.TimeoutExpired):
                pass
    except (
        UnicodeDecodeError, UnicodeEncodeError, OSError, ValueError,
        subprocess.SubprocessError, RecursionError,
    ):
        return


def _record_private_observation(
    pith_binary: str, request: dict[str, object], *, timeout: float = RECORD_TIMEOUT_SECONDS,
) -> None:
    """Submit a tiny aggregate-only request with the existing bounded reaper."""
    try:
        encoded = json.dumps(request, separators=(",", ":"), allow_nan=False).encode("ascii")
        if len(encoded) > RECORD_MAX_REQUEST or not os.path.isabs(pith_binary):
            return
        deadline = time.monotonic() + max(0.001, timeout)
        process = subprocess.Popen(
            [pith_binary, "_record-codex-wrapper"], stdin=subprocess.PIPE,
            stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, close_fds=True,
        )
        remaining = deadline - time.monotonic()
        if remaining <= 0:
            try:
                process.kill()
            except OSError:
                pass
            try:
                process.communicate(timeout=0.05)
            except (OSError, subprocess.TimeoutExpired):
                pass
            return
        try:
            process.communicate(encoded, timeout=remaining)
        except subprocess.TimeoutExpired:
            try:
                process.kill()
            except OSError:
                pass
            try:
                process.communicate(timeout=0.05)
            except (OSError, subprocess.TimeoutExpired):
                pass
    except (OSError, ValueError, subprocess.SubprocessError, RecursionError):
        return


def _record_codex_proposal(pith_binary: str, family: str, bypass_reason: str = "", *, timeout: float = RECORD_TIMEOUT_SECONDS) -> None:
    if family not in _CODEX_PROPOSAL_FAMILIES:
        return
    if bypass_reason and bypass_reason not in _BYPASS_REASONS:
        return
    request = {"kind": "proposal", "family": family}
    if bypass_reason:
        request["bypassReason"] = bypass_reason
    _record_private_observation(pith_binary, request, timeout=timeout)


def _record_capture_observation(
    pith_binary: str, outcome: str, elapsed_ms: int | None, selection: str,
    original_bytes: int | None, delivered_bytes: int | None, *,
    timeout: float = RECORD_TIMEOUT_SECONDS,
) -> None:
    if outcome not in {"complete_within_window", "deadline_stream", "cap_stream", "incomplete"}:
        return
    if selection not in {"reduced", "raw", "unknown"}:
        return
    if (original_bytes is None) != (delivered_bytes is None):
        return
    if elapsed_ms is not None and not 0 <= elapsed_ms <= RECORD_MAX_DURATION_MS:
        elapsed_ms = None
    if original_bytes is not None and (
        not 0 <= original_bytes <= RECORD_MAX_BYTES or
        not 0 <= delivered_bytes <= RECORD_MAX_BYTES
    ):
        original_bytes = delivered_bytes = None
    if outcome == "incomplete":
        selection = "unknown"
        original_bytes = delivered_bytes = None
    elif outcome != "complete_within_window":
        selection = "raw"
    if selection == "reduced" and (
        original_bytes is None or delivered_bytes is None or delivered_bytes >= original_bytes
    ):
        return
    if selection == "raw" and original_bytes is not None and original_bytes != delivered_bytes:
        return
    _record_private_observation(pith_binary, {
        "kind": "capture", "captureOutcome": outcome, "elapsedMs": elapsed_ms,
        "selection": selection, "originalBytes": original_bytes,
        "deliveredBytes": delivered_bytes,
    }, timeout=timeout)


def _pending_cancel() -> bool:
    try:
        return bool(signal.sigpending().intersection(_CANCEL_SIGNALS))
    except (AttributeError, OSError, ValueError):
        return True


def _pending_cancel_signal() -> int | None:
    try:
        pending = signal.sigpending()
        return next((signum for signum in (signal.SIGINT, signal.SIGTERM) if signum in pending), None)
    except (AttributeError, OSError, ValueError):
        return signal.SIGTERM


def _reraise(signum: int) -> None:
    """Terminate this adapter with the same signal observed from the command."""
    signal.signal(signum, signal.SIG_DFL)
    signal.pthread_sigmask(signal.SIG_UNBLOCK, {signum})
    os.kill(os.getpid(), signum)
    raise SystemExit(128 + signum)


def _write_gate(fd: int, child_pid: int) -> None:
    if os.write(fd, b"G") != 1:
        raise OSError("could not release exec gate")


def _stderr_delivery_warning(reason: str, native_status: int | None) -> None:
    status = "unknown" if native_status is None else str(native_status)
    _warn_delivery(f"{reason}; native status={status}; stdout was not retried")


def _stop_reap(proc: subprocess.Popen[bytes], *, grace_seconds: float = 0.05) -> bool:
    """Stop a supporting child and reap it with a fixed, small wait bound."""
    if proc.poll() is not None:
        return True
    try:
        proc.kill()
    except ProcessLookupError:
        pass
    try:
        proc.wait(timeout=grace_seconds)
    except subprocess.TimeoutExpired:
        return proc.poll() is not None
    return proc.poll() is not None


def _gated_exec_main(gate_fd: int, executable: str, argv: Sequence[str]) -> int:
    for signum in _CANCEL_SIGNALS:
        signal.signal(signum, signal.SIG_DFL)
    try:
        token = os.read(gate_fd, 1)
    except OSError:
        token = b""
    try:
        os.close(gate_fd)
    except OSError:
        pass
    if token != b"G" or _pending_cancel():
        return 125
    _restore_native_signal_dispositions()
    signal.pthread_sigmask(signal.SIG_UNBLOCK, _CANCEL_SIGNALS)
    try:
        os.execve(executable, list(argv), os.environ)
    except FileNotFoundError:
        return 127
    except OSError:
        return 126
    return 126


def _spawn_gated(
    argv: Sequence[str], *, stdin: int | None, stdout: int, stderr: int | None,
    launch_deadline: float | None = None, executable: str | None = None,
) -> tuple[subprocess.Popen[bytes] | None, bool, bool, bool]:
    """Return process, gate released, canceled before release, spawn failure."""
    if launch_deadline is not None and time.monotonic() >= launch_deadline:
        return None, False, False, False
    try:
        gate_read, gate_write = os.pipe()
    except OSError:
        return None, False, False, True
    old_mask = signal.pthread_sigmask(signal.SIG_BLOCK, _CANCEL_SIGNALS)
    proc: subprocess.Popen[bytes] | None = None
    released = False
    canceled_before_release = False
    failed = False
    try:
        if _CANCEL_SIGNAL is not None or _pending_cancel():
            canceled_before_release = True
        else:
            command = [
                os.path.realpath(sys.executable), "-I", "-S", os.path.realpath(__file__),
                "--gated-exec", "--gate-fd", str(gate_read),
                "--executable", executable or argv[0], "--", *argv,
            ]
            try:
                proc = subprocess.Popen(
                    command, stdin=stdin, stdout=stdout, stderr=stderr,
                    close_fds=True, pass_fds=(gate_read,),
                )
            except OSError:
                failed = True
            if proc is not None:
                os.close(gate_read)
                gate_read = -1
                if _CANCEL_SIGNAL is not None or _pending_cancel():
                    canceled_before_release = True
                elif launch_deadline is not None and time.monotonic() >= launch_deadline:
                    pass
                else:
                    try:
                        _write_gate(gate_write, proc.pid)
                        released = True
                    except OSError:
                        failed = True
                    if _CANCEL_SIGNAL is not None or _pending_cancel():
                        canceled_before_release = True
    finally:
        for fd in (gate_read, gate_write):
            if fd >= 0:
                try:
                    os.close(fd)
                except OSError:
                    pass
        signal.pthread_sigmask(signal.SIG_SETMASK, old_mask)
    if _CANCEL_SIGNAL is not None:
        canceled_before_release = canceled_before_release or not released
    return proc, released, canceled_before_release, failed


def _new_pipe_poller(*fds: int) -> select.poll:
    poller = select.poll()
    for fd in fds:
        poller.register(fd, select.POLLIN | select.POLLOUT | select.POLLHUP | select.POLLERR)
    return poller


def _wait_fd(poller: select.poll, fd: int, timeout_ms: int) -> bool:
    ready_fds, wait_failed = _wait_fds(poller, (fd,), timeout_ms)
    return bool(ready_fds) or wait_failed


def _wait_fds(poller: select.poll, fds: Sequence[int], timeout_ms: int) -> tuple[set[int], bool]:
    try:
        return {ready_fd for ready_fd, _ in poller.poll(timeout_ms)}, False
    except InterruptedError:
        pass
    except OSError:
        pass
    try:
        readable, writable, exceptional = select.select(list(fds), list(fds), list(fds), timeout_ms / 1000)
        return set(readable + writable + exceptional), False
    except (OSError, ValueError):
        return set(), True


def _run_transform(
    pith_binary: str, command: str, output: bytes, deadline: float, *,
    argv: Sequence[str] = (),
    output_limit: int = TRANSFORM_OUTPUT_LIMIT,
    native_status: int | None = 0,
    warning_writer: Callable[[str, int | None], None] = _stderr_delivery_warning,
) -> bytes | None:
    if time.monotonic() >= deadline or native_status != 0 or _CANCEL_SIGNAL is not None:
        return None
    try:
        text = output.decode("utf-8", errors="strict")
    except UnicodeDecodeError:
        return None
    try:
        request = json.dumps(
            {"command": command, "argv": list(argv), "output": text, "exitCode": native_status,
             "rawBypass": False, "telemetryEnabled": False},
            ensure_ascii=False, separators=(",", ":"),
        ).encode("utf-8", errors="strict")
    except (UnicodeEncodeError, RecursionError, ValueError):
        return None
    if time.monotonic() >= deadline:
        return None
    try:
        pith_available = isinstance(pith_binary, str) and os.path.isabs(pith_binary) and os.access(pith_binary, os.X_OK)
    except (OSError, UnicodeError, ValueError):
        pith_available = False
    if not pith_available or _CANCEL_SIGNAL is not None:
        return None
    if time.monotonic() >= deadline:
        return None
    created: list[int] = []
    try:
        child_stdin, parent_stdin = os.pipe()
        created.extend((child_stdin, parent_stdin))
        parent_stdout, child_stdout = os.pipe()
        created.extend((parent_stdout, child_stdout))
        os.set_blocking(parent_stdin, False)
        os.set_blocking(parent_stdout, False)
        poller = _new_pipe_poller(parent_stdin, parent_stdout)
    except OSError:
        for fd in created:
            try:
                os.close(fd)
            except OSError:
                pass
        return None

    if time.monotonic() >= deadline or _CANCEL_SIGNAL is not None:
        for fd in created:
            try:
                os.close(fd)
            except OSError:
                pass
        return None

    proc, released, canceled, spawn_failed = _spawn_gated(
        [pith_binary, "pi", "transform"],
        stdin=child_stdin, stdout=child_stdout, stderr=subprocess.DEVNULL,
        launch_deadline=deadline,
    )
    os.close(child_stdin)
    os.close(child_stdout)
    if proc is None or not released or canceled or spawn_failed:
        if proc is not None:
            if not _stop_reap(proc):
                warning_writer("Pith transform child reap unsettled; raw stdout fallback used", native_status)
        for fd in (parent_stdin, parent_stdout):
            try:
                os.close(fd)
            except OSError:
                pass
        return None

    sent = 0
    response = bytearray()
    stdin_open = True
    stdout_open = True
    failed = False
    reap_settled = True
    try:
        while stdin_open or stdout_open or proc.poll() is None:
            remaining = deadline - time.monotonic()
            if remaining <= 0 or _CANCEL_SIGNAL is not None:
                failed = True
                break
            if stdin_open and sent < len(request) and _wait_fd(
                poller, parent_stdin, min(50, max(1, int((deadline - time.monotonic()) * 1000)))
            ):
                try:
                    sent += os.write(parent_stdin, request[sent:sent + READ_CHUNK])
                except (BlockingIOError, InterruptedError):
                    pass
                except BrokenPipeError:
                    stdin_open = False
                    poller.unregister(parent_stdin)
                    os.close(parent_stdin)
                except OSError:
                    failed = True
                    break
                if sent == len(request) and stdin_open:
                    stdin_open = False
                    poller.unregister(parent_stdin)
                    os.close(parent_stdin)
            elif stdin_open and sent == len(request):
                stdin_open = False
                poller.unregister(parent_stdin)
                os.close(parent_stdin)

            if stdout_open and _wait_fd(
                poller, parent_stdout, min(50, max(1, int((deadline - time.monotonic()) * 1000)))
            ):
                try:
                    chunk = os.read(parent_stdout, min(READ_CHUNK, output_limit - len(response) + 1))
                except (BlockingIOError, InterruptedError):
                    chunk = None
                except OSError:
                    failed = True
                    break
                if chunk == b"":
                    stdout_open = False
                    poller.unregister(parent_stdout)
                    os.close(parent_stdout)
                elif chunk is not None and len(response) + len(chunk) > output_limit:
                    failed = True
                    break
                elif chunk:
                    response.extend(chunk)
        return_code = proc.poll()
        if return_code is None:
            failed = True
    except OSError:
        failed = True
    finally:
        if failed and proc.poll() is None:
            reap_settled = _stop_reap(proc)
        for fd in (parent_stdin, parent_stdout):
            try:
                os.close(fd)
            except OSError:
                pass
        for fd in (parent_stdin, parent_stdout):
            try:
                poller.unregister(fd)
            except (KeyError, OSError):
                pass
    if not reap_settled:
        warning_writer("Pith transform child reap unsettled; raw stdout fallback used", native_status)
        return None
    if failed or proc.returncode != 0:
        return None
    return bytes(response)


def _restore_native_signal_dispositions() -> None:
    """Undo Python/Popen ignored signals before handing control to native code."""
    for name in ("SIGPIPE", "SIGXFZ", "SIGXFSZ"):
        signum = getattr(signal, name, None)
        if signum is not None:
            signal.signal(signum, signal.SIG_DFL)


def _exec_original(argv: Sequence[str], executable: str | None = None) -> int:
    try:
        _restore_native_signal_dispositions()
        if executable is not None:
            os.execve(executable, list(argv), os.environ)
        else:
            os.execvpe(argv[0], list(argv), os.environ)
    except FileNotFoundError:
        return 127
    except OSError:
        return 126
    return 126


def _exec_after_setup_failure(argv: Sequence[str], previous: dict[int, object], executable: str | None = None) -> int:
    """Choose cancellation or direct native fallback without a signal gap."""
    old_mask = signal.pthread_sigmask(signal.SIG_BLOCK, _CANCEL_SIGNALS)
    cancel = _CANCEL_SIGNAL or _pending_cancel_signal()
    if cancel is not None:
        for signum, handler in previous.items():
            signal.signal(signum, handler)
        signal.pthread_sigmask(signal.SIG_SETMASK, old_mask)
        _reraise(cancel)
        return 128 + cancel
    # A signal arriving after the check stays pending while blocked, then its
    # default disposition terminates this wrapper before native execve.
    for signum in _CANCEL_SIGNALS:
        signal.signal(signum, signal.SIG_DFL)
    signal.pthread_sigmask(signal.SIG_SETMASK, old_mask)
    return _exec_original(argv, executable)


def run_native(
    argv: Sequence[str], pith_binary: str, *,
    resolved_executable: str | None = None,
    capture_limit: int = CAPTURE_LIMIT, capture_seconds: float = CAPTURE_SECONDS,
    transform_seconds: float = TRANSFORM_SECONDS,
    stdout_fd: int = 1, writer: Callable[[int, bytes], bool] = _write_all,
    warning_writer: Callable[[str, int | None], None] = _stderr_delivery_warning,
    record_outcome: Callable[..., None] | None = None,
    record_observation: Callable[[str, str, int | None, str, int | None, int | None], None] | None = None,
) -> int:
    """Run the already-authorized native command once; transform eligible stdout."""
    global _CANCEL_SIGNAL
    if not argv or not (os.path.isabs(argv[0]) or resolved_executable):
        return 126
    if _must_exec_original():
        return _exec_original(argv, resolved_executable)
    if resolved_executable is not None and (
        not os.path.isfile(resolved_executable) or not os.access(resolved_executable, os.X_OK)
    ):
        return 126

    _CANCEL_SIGNAL = None
    previous: dict[int, object] = {}

    def note_cancel(signum: int, _frame: object) -> None:
        global _CANCEL_SIGNAL
        if _CANCEL_SIGNAL is None:
            _CANCEL_SIGNAL = signum

    for signum in _CANCEL_SIGNALS:
        previous[signum] = signal.signal(signum, note_cancel)

    try:
        stdout_read, stdout_write = os.pipe()
        stderr_read, stderr_write = os.pipe()
        os.set_blocking(stdout_read, False)
        os.set_blocking(stderr_read, False)
        poller = _new_pipe_poller(stdout_read, stderr_read)
    except OSError:
        for fd in (locals().get("stdout_read"), locals().get("stdout_write"),
                   locals().get("stderr_read"), locals().get("stderr_write")):
            if isinstance(fd, int):
                try:
                    os.close(fd)
                except OSError:
                    pass
        return _exec_after_setup_failure(argv, previous, resolved_executable)

    capture_deadline = time.monotonic() + capture_seconds
    native_started_ns = time.monotonic_ns()
    spawn_kwargs = {"stdin": None, "stdout": stdout_write, "stderr": stderr_write}
    if resolved_executable is not None:
        spawn_kwargs["executable"] = resolved_executable
    proc, released, canceled_before_release, spawn_failed = _spawn_gated(argv, **spawn_kwargs)
    os.close(stdout_write)
    os.close(stderr_write)
    if proc is None or spawn_failed:
        if proc is not None:
            _stop_reap(proc)
        for fd in (stdout_read, stderr_read):
            try:
                poller.unregister(fd)
            except (KeyError, OSError):
                pass
            try:
                os.close(fd)
            except OSError:
                pass
        return _exec_after_setup_failure(argv, previous, resolved_executable)

    captured = bytearray()
    eligible = released and not canceled_before_release
    capture_started = eligible
    capture_outcome = "incomplete"
    original_bytes = 0
    delivered_bytes = 0
    stdout_delivery_ok = True
    stderr_delivery_ok = True

    def publish(data: bytes) -> bool:
        nonlocal delivered_bytes, stdout_delivery_ok
        if not stdout_delivery_ok:
            return False
        succeeded = writer(stdout_fd, data)
        if succeeded:
            delivered_bytes += len(data)
        else:
            stdout_delivery_ok = False
        return succeeded

    stdout_eof = False
    stderr_eof = False
    stderr_observed = False
    read_failure = False
    stdout_read_errors = 0
    stderr_read_errors = 0
    native_status: int | None = None
    native_duration_ms = 0
    published_stdout: bytes | None = None
    published_outcome = "raw_fallback"
    selected_parser = ""

    def close_read_fd(fd: int) -> None:
        try:
            poller.unregister(fd)
        except (KeyError, OSError):
            pass
        try:
            os.close(fd)
        except OSError:
            pass

    try:
        while not (stdout_eof and stderr_eof) or proc.poll() is None:
            remaining = capture_deadline - time.monotonic()
            if eligible and (remaining <= 0 or _CANCEL_SIGNAL is not None):
                capture_outcome = "incomplete" if _CANCEL_SIGNAL is not None else "deadline_stream"
                eligible = False
                if captured and not publish(bytes(captured)):
                    stdout_delivery_ok = False
                captured.clear()

            timeout_ms = min(50, max(1, int(remaining * 1000))) if eligible else 50
            active_fds = tuple(
                fd for fd, eof in ((stdout_read, stdout_eof), (stderr_read, stderr_eof)) if not eof
            )
            ready_fds, wait_failed = _wait_fds(poller, active_fds, timeout_ms)
            if wait_failed:
                # Selector failure is uncertain capture, not a timeout. Switch
                # to raw and probe every nonblocking pipe so a native writer
                # cannot fill a pipe and deadlock. The native command is never
                # replayed after this point.
                read_failure = True
                if eligible:
                    eligible = False
                    capture_outcome = "incomplete"
                    if captured and not publish(bytes(captured)):
                        stdout_delivery_ok = False
                    captured.clear()
                ready_fds = set(active_fds)
            if not ready_fds:
                time.sleep(0.005)
            drained_stream = False
            # Drain stderr first. Its first byte disables transformation and
            # releases any buffered stdout before relaying stderr unchanged.
            if stderr_read in ready_fds and not stderr_eof:
                try:
                    stderr_chunk = os.read(stderr_read, READ_CHUNK)
                except (BlockingIOError, InterruptedError):
                    stderr_chunk = None
                except OSError:
                    stderr_read_errors += 1
                    read_failure = True
                    if eligible:
                        eligible = False
                        capture_outcome = "incomplete"
                        if captured and not publish(bytes(captured)):
                            stdout_delivery_ok = False
                        captured.clear()
                    if stderr_read_errors >= 3:
                        stderr_delivery_ok = False
                        stderr_eof = True
                        close_read_fd(stderr_read)
                    stderr_chunk = None
                if stderr_chunk is not None:
                    drained_stream = True
                if stderr_chunk == b"":
                    stderr_eof = True
                    close_read_fd(stderr_read)
                elif stderr_chunk:
                    stderr_observed = True
                    if eligible:
                        eligible = False
                        if captured and not publish(bytes(captured)):
                            stdout_delivery_ok = False
                        captured.clear()
                    # Continue relaying native stderr even if stdout delivery
                    # already failed; the streams have independent destinations.
                    if not _write_all(2, stderr_chunk):
                        stderr_delivery_ok = False
                if not stderr_eof and stderr_chunk is not None:
                    stderr_read_errors = 0

            if stdout_read in ready_fds and not stdout_eof:
                max_read = min(READ_CHUNK, max(1, capture_limit - len(captured) + 1)) if eligible else READ_CHUNK
                try:
                    chunk = os.read(stdout_read, max_read)
                except (BlockingIOError, InterruptedError):
                    chunk = None
                except OSError:
                    stdout_read_errors += 1
                    capture_outcome = "incomplete"
                    if eligible:
                        eligible = False
                        if captured and not publish(bytes(captured)):
                            stdout_delivery_ok = False
                        captured.clear()
                    if stdout_read_errors >= 3:
                        read_failure = True
                        stdout_delivery_ok = False
                        stdout_eof = True
                        close_read_fd(stdout_read)
                    chunk = None
                if chunk is not None:
                    drained_stream = True
                if chunk == b"":
                    stdout_eof = True
                    close_read_fd(stdout_read)
                elif chunk:
                    stdout_read_errors = 0
                    original_bytes += len(chunk)
                    if eligible and len(captured) + len(chunk) <= capture_limit:
                        captured.extend(chunk)
                    else:
                        if eligible:
                            capture_outcome = "cap_stream"
                            eligible = False
                            if captured and not publish(bytes(captured)):
                                stdout_delivery_ok = False
                            captured.clear()
                        if stdout_delivery_ok:
                            publish(chunk)
            if wait_failed and not drained_stream:
                time.sleep(0.005)
        native_status = proc.poll()
        native_duration_ms = max(0, (time.monotonic_ns() - native_started_ns) // 1_000_000)
        native_settled_at = time.monotonic()
        if stderr_observed and not read_failure and capture_outcome == "incomplete":
            capture_outcome = "complete_within_window" if native_settled_at < capture_deadline else "deadline_stream"
        if eligible and time.monotonic() >= capture_deadline:
            capture_outcome = "deadline_stream"
            eligible = False
            if captured and not publish(bytes(captured)):
                stdout_delivery_ok = False
            captured.clear()
        if eligible and native_status == 0 and _CANCEL_SIGNAL is None and stdout_delivery_ok and stderr_delivery_ok:
            transform_deadline = time.monotonic() + transform_seconds
            transformed = _run_transform(
                pith_binary, shlex.join(list(argv)), bytes(captured), transform_deadline,
                argv=argv, native_status=native_status, warning_writer=warning_writer,
            )
            selected = (
                _selected_output(bytes(captured), transformed, argv)
                if transformed is not None and _CANCEL_SIGNAL is None and time.monotonic() < transform_deadline
                else None
            )
            if selected is not None:
                published_stdout, selected_parser = selected
                published_outcome = "transformed"
                publish(published_stdout)
            elif stdout_delivery_ok:
                published_stdout = bytes(captured)
                publish(bytes(captured))
        elif eligible and stdout_delivery_ok and stderr_delivery_ok:
            published_stdout = bytes(captured)
            publish(bytes(captured))

        if capture_outcome not in ("deadline_stream", "cap_stream") and eligible:
            capture_outcome = "complete_within_window"
        delivery_ok = stdout_delivery_ok and stderr_delivery_ok
        if read_failure or not delivery_ok:
            capture_outcome = "incomplete"

        if not delivery_ok:
            warning_writer("captured stream read failed" if read_failure else "native stream publication incomplete", native_status)
    finally:
        if not stdout_eof:
            close_read_fd(stdout_read)
        if not stderr_eof:
            close_read_fd(stderr_read)
        try:
            poller.close()
        except (AttributeError, OSError):
            pass
        for signum, handler in previous.items():
            signal.signal(signum, handler)
    if _CANCEL_SIGNAL is not None:
        _reraise(_CANCEL_SIGNAL)
    if native_status is not None and native_status < 0:
        _reraise(-native_status)
    if eligible and delivery_ok and published_stdout is not None and native_status is not None:
        try:
            original_text = bytes(captured).decode("utf-8", errors="strict")
            delivered_text = published_stdout.decode("utf-8", errors="strict")
            original_runes, delivered_runes = len(original_text), len(delivered_text)
            known_counts = (
                original_runes <= RECORD_MAX_RUNES and delivered_runes <= RECORD_MAX_RUNES and
                native_duration_ms <= RECORD_MAX_DURATION_MS and
                ((published_outcome == "raw_fallback" and original_runes == delivered_runes) or
                 (published_outcome == "transformed" and delivered_runes <= original_runes))
            )
            if known_counts:
                recorder = record_outcome or _record_wrapper_outcome
                for signum in _CANCEL_SIGNALS:
                    signal.signal(signum, note_cancel)
                try:
                    recorder(
                        pith_binary, bytes(captured), published_stdout, native_duration_ms,
                        published_outcome, _family_for_argv(argv), selected_parser,
                    )
                except Exception:
                    # Existing gain accounting remains post-publication and best effort.
                    pass
                finally:
                    old_mask = signal.pthread_sigmask(signal.SIG_BLOCK, _CANCEL_SIGNALS)
                    cancel = _CANCEL_SIGNAL or _pending_cancel_signal()
                    for signum, handler in previous.items():
                        signal.signal(signum, handler)
                    signal.pthread_sigmask(signal.SIG_SETMASK, old_mask)
                if cancel is not None:
                    _reraise(cancel)
        except (UnicodeDecodeError, OverflowError):
            pass
    if capture_started and native_status is not None:
        outcome = "incomplete" if read_failure or not delivery_ok else capture_outcome
        selection = "unknown"
        known_original: int | None = None
        known_delivered: int | None = None
        if outcome != "incomplete":
            selection = "reduced" if published_outcome == "transformed" else "raw"
            if original_bytes <= RECORD_MAX_BYTES and delivered_bytes <= RECORD_MAX_BYTES:
                known_original, known_delivered = original_bytes, delivered_bytes
        known_duration: int | None = native_duration_ms
        if not 0 <= native_duration_ms <= RECORD_MAX_DURATION_MS:
            known_duration = None
        recorder = record_observation or _record_capture_observation
        for signum in _CANCEL_SIGNALS:
            signal.signal(signum, note_cancel)
        try:
            recorder(pith_binary, outcome, known_duration, selection, known_original, known_delivered)
        except Exception:
            # Telemetry is post-publication best effort and cannot change the
            # already completed native command result.
            pass
        finally:
            # Keep cancellation latched while the bounded recorder is running,
            # then restore the caller's handlers atomically with pending signals.
            old_mask = signal.pthread_sigmask(signal.SIG_BLOCK, _CANCEL_SIGNALS)
            cancel = _CANCEL_SIGNAL or _pending_cancel_signal()
            for signum, handler in previous.items():
                signal.signal(signum, handler)
            signal.pthread_sigmask(signal.SIG_SETMASK, old_mask)
        if cancel is not None:
            _reraise(cancel)
    return 125 if native_status is None else native_status


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--run", action="store_true")
    parser.add_argument("--pith-binary")
    parser.add_argument("--resolved-executable")
    parser.add_argument("--gated-exec", action="store_true")
    parser.add_argument("--gate-fd", type=int)
    parser.add_argument("--executable")
    args, rest = parser.parse_known_args(argv)
    native = rest[1:] if rest and rest[0] == "--" else rest
    if args.gated_exec:
        if args.gate_fd is None or not args.executable or not native:
            return 125
        return _gated_exec_main(args.gate_fd, args.executable, native)
    if args.run:
        if not args.pith_binary:
            return 126
        return run_native(native, args.pith_binary, resolved_executable=args.resolved_executable)
    if not args.pith_binary:
        result = _allow()
    else:
        try:
            raw_payload = sys.stdin.buffer.read(HOOK_INPUT_LIMIT + 1)
            if len(raw_payload) > HOOK_INPUT_LIMIT:
                raise ValueError("hook input exceeds limit")
            payload = json.loads(raw_payload.decode("utf-8", errors="strict"))
        except (json.JSONDecodeError, UnicodeDecodeError, OSError, ValueError, RecursionError):
            result = _allow()
        else:
            result = rewrite_hook_input(
                payload, args.pith_binary, record_bypass=_record_codex_proposal,
            )
    try:
        serialized = json.dumps(result, separators=(",", ":"))
    except (TypeError, ValueError, RecursionError):
        serialized = json.dumps(_allow(), separators=(",", ":"))
    sys.stdout.write(serialized + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
