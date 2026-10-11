#!/usr/bin/env python3
"""Narrow Codex Bash PreToolUse adapter for completed Go test output."""

from __future__ import annotations

import argparse
import errno
import json
import os
import re
import selectors
import shlex
import signal
import subprocess
import sys
import time
from typing import Callable, Sequence


CAPTURE_LIMIT = 1 << 20
READ_CHUNK = 32 << 10
BUFFER_SECONDS = 2.0
TRANSFORM_OUTPUT_LIMIT = 1 << 20
HOOK_INPUT_LIMIT = 64 << 10
_CANCEL_SIGNAL: int | None = None
_SUMMARY_OK = re.compile(r"ok\s+\S+\s+\d+(?:\.\d+)?s\Z")
_PACKAGE = re.compile(r"(?:\.|\./[A-Za-z0-9_./-]+|[A-Za-z0-9_./-]+)\Z")
_RUN = re.compile(r"=== RUN\s+(\S+)\Z")
_PAUSE = re.compile(r"=== PAUSE\s+\S+\Z")
_CONT = re.compile(r"=== CONT\s+\S+\Z")
_PASS = re.compile(r"--- PASS:\s+(\S+)\s+\(\d+(?:\.\d+)?s\)\Z")


def _absolute_go_test(command: str) -> list[str] | None:
    """Accept one literal argv form; shell metacharacters are never evaluated."""
    if not command or any(ch in command for ch in "\x00\r\n;&|()<>`$*?[]{}~"):
        return None
    try:
        argv = shlex.split(command, posix=True)
    except ValueError:
        return None
    if len(argv) != 4:
        return None
    executable, subcommand, verbose, package = argv
    if (
        not os.path.isabs(executable)
        or os.path.basename(executable) != "go"
        or subcommand != "test"
        or verbose != "-v"
        or not _PACKAGE.fullmatch(package)
        or "..." in package
        or package.startswith("-")
    ):
        return None
    return argv


def _allow(with_update: dict[str, object] | None = None) -> dict[str, object]:
    specific: dict[str, object] = {
        "hookEventName": "PreToolUse",
        "permissionDecision": "allow",
    }
    if with_update is not None:
        specific["updatedInput"] = with_update
    return {"hookSpecificOutput": specific}


def rewrite_hook_input(payload: object, pith_binary: str) -> dict[str, object]:
    if not isinstance(payload, dict):
        return _allow()
    if payload.get("tool_name") != "Bash":
        return _allow()
    tool_input = payload.get("tool_input")
    if not isinstance(tool_input, dict) or not isinstance(tool_input.get("command"), str):
        return _allow()
    argv = _absolute_go_test(tool_input["command"])
    if argv is None or not os.path.isabs(pith_binary) or not os.access(pith_binary, os.X_OK):
        return _allow()

    python = os.path.realpath(sys.executable)
    script = os.path.realpath(__file__)
    if not os.path.isabs(python) or not os.path.isabs(script):
        return _allow()
    wrapper = [
        python, "-I", "-S", script, "--run", "--pith-binary", pith_binary,
        "--", *argv,
    ]
    return _allow({"command": shlex.join(wrapper)})


def _fd_usable(fd: int) -> bool:
    os.fstat(fd)
    return True


def _must_exec_original() -> bool:
    try:
        for fd in (0, 1, 2):
            _fd_usable(fd)
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
    for raw_line in text.splitlines():
        line = raw_line.strip()
        if not line:
            continue
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
            # Diagnostics, test logs, failures, structured output, and unknown
            # lines make the aggregate parser unsafe for this invocation.
            return None
    if not runs or len(runs) != len(passes) or sorted(runs) != sorted(passes):
        return None
    if not summary_pass or not summary_ok:
        return None
    return "PASS", summary_ok


def _selected_output(raw: bytes, response_bytes: bytes) -> bytes | None:
    summary = _valid_go_verbose_success(raw)
    if summary is None:
        return None
    try:
        response = json.loads(response_bytes.decode("utf-8", errors="strict"))
    except (UnicodeDecodeError, json.JSONDecodeError):
        return None
    if not isinstance(response, dict):
        return None
    selected = response.get("output")
    if (
        not isinstance(selected, str)
        or response.get("parser") != "tests"
        or response.get("passthrough") is not False
    ):
        return None
    lines = [line.strip() for line in selected.splitlines() if line.strip()]
    if lines != [summary[0], summary[1]]:
        return None
    encoded = selected.encode("utf-8")
    if not encoded or len(encoded) >= len(raw):
        return None
    return encoded


def _run_transform(
    pith_binary: str,
    command: str,
    output: bytes,
    deadline: float,
    output_limit: int = TRANSFORM_OUTPUT_LIMIT,
) -> bytes | None:
    try:
        text = output.decode("utf-8", errors="strict")
    except UnicodeDecodeError:
        return None
    request = json.dumps(
        {
            "command": command,
            "output": text,
            "exitCode": 0,
            "rawBypass": False,
            "telemetryEnabled": False,
        },
        ensure_ascii=False,
        separators=(",", ":"),
    ).encode("utf-8")
    try:
        proc = subprocess.Popen(
            [pith_binary, "pi", "transform"],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL,
            bufsize=0,
        )
    except OSError:
        return None
    assert proc.stdin is not None and proc.stdout is not None
    selector = selectors.DefaultSelector()
    in_fd, out_fd = proc.stdin.fileno(), proc.stdout.fileno()
    sent = 0
    response = bytearray()
    failed = False
    try:
        os.set_blocking(in_fd, False)
        os.set_blocking(out_fd, False)
        selector.register(out_fd, selectors.EVENT_READ, "stdout")
        if request:
            selector.register(in_fd, selectors.EVENT_WRITE, "stdin")
        else:
            proc.stdin.close()
        while selector.get_map() or proc.poll() is None:
            remaining = deadline - time.monotonic()
            if remaining <= 0 or _CANCEL_SIGNAL is not None:
                failed = True
                break
            events = selector.select(min(remaining, 0.05))
            for key, _ in events:
                if key.data == "stdin":
                    try:
                        sent += os.write(in_fd, request[sent:sent + READ_CHUNK])
                    except InterruptedError:
                        continue
                    except BrokenPipeError:
                        selector.unregister(in_fd)
                        proc.stdin.close()
                        continue
                    if sent == len(request):
                        selector.unregister(in_fd)
                        proc.stdin.close()
                else:
                    try:
                        chunk = os.read(out_fd, min(READ_CHUNK, output_limit - len(response) + 1))
                    except InterruptedError:
                        continue
                    if not chunk:
                        selector.unregister(out_fd)
                        proc.stdout.close()
                    elif len(response) + len(chunk) > output_limit:
                        failed = True
                        break
                    else:
                        response.extend(chunk)
            if failed:
                break
        if proc.poll() is None:
            failed = True
    except OSError:
        failed = True
    finally:
        selector.close()
        if failed and proc.poll() is None:
            try:
                proc.kill()
            except ProcessLookupError:
                pass
        try:
            proc.wait(timeout=0.5)
        except subprocess.TimeoutExpired:
            try:
                proc.kill()
            except ProcessLookupError:
                pass
            proc.wait()
        for stream in (proc.stdin, proc.stdout):
            if stream is not None and not stream.closed:
                stream.close()
    if failed or proc.returncode != 0:
        return None
    return bytes(response)


def _reraise(signum: int) -> None:
    signal.signal(signum, signal.SIG_DFL)
    os.kill(os.getpid(), signum)


def run_native(
    argv: Sequence[str],
    pith_binary: str,
    *,
    capture_limit: int = CAPTURE_LIMIT,
    buffer_seconds: float = BUFFER_SECONDS,
    stdout_fd: int = 1,
    writer: Callable[[int, bytes], bool] = _write_all,
) -> int:
    """Run once, then transform complete eligible output or relay exact bytes."""
    global _CANCEL_SIGNAL
    if not argv or not os.path.isabs(argv[0]):
        return 126
    if _must_exec_original():
        os.execve(argv[0], list(argv), os.environ)

    _CANCEL_SIGNAL = None
    previous: dict[int, object] = {}

    def note_cancel(signum: int, _frame: object) -> None:
        global _CANCEL_SIGNAL
        if _CANCEL_SIGNAL is None:
            _CANCEL_SIGNAL = signum

    for signum in (signal.SIGINT, signal.SIGTERM):
        previous[signum] = signal.signal(signum, note_cancel)

    started = time.monotonic()
    deadline = started + buffer_seconds
    try:
        # Deliberately inherit cwd, environment, stdin, stderr, and process
        # group. No shell, session, or group is introduced.
        proc = subprocess.Popen(list(argv), stdin=None, stdout=subprocess.PIPE, stderr=None, bufsize=0)
    except OSError:
        for signum, handler in previous.items():
            signal.signal(signum, handler)
        return 127

    assert proc.stdout is not None
    selector = selectors.DefaultSelector()
    selector.register(proc.stdout.fileno(), selectors.EVENT_READ)
    captured = bytearray()
    eligible = True
    eof = False
    delivery_ok = True
    try:
        while not eof or proc.poll() is None:
            remaining = deadline - time.monotonic()
            if eligible and (remaining <= 0 or _CANCEL_SIGNAL is not None):
                eligible = False
                if captured and not writer(stdout_fd, bytes(captured)):
                    delivery_ok = False
                captured.clear()

            wait = min(0.05, max(0.0, remaining)) if eligible else 0.05
            events = selector.select(wait)
            if not events:
                continue
            max_read = min(READ_CHUNK, max(1, capture_limit - len(captured) + 1)) if eligible else READ_CHUNK
            try:
                chunk = os.read(proc.stdout.fileno(), max_read)
            except InterruptedError:
                continue
            if not chunk:
                eof = True
                selector.unregister(proc.stdout.fileno())
                proc.stdout.close()
                continue
            if eligible and len(captured) + len(chunk) <= capture_limit:
                captured.extend(chunk)
                continue
            if eligible:
                eligible = False
                if captured and not writer(stdout_fd, bytes(captured)):
                    delivery_ok = False
                captured.clear()
            if delivery_ok and not writer(stdout_fd, chunk):
                # Keep draining to avoid changing the native process with a
                # broken pipe; the host reports incomplete stdout delivery.
                delivery_ok = False
        native_status = proc.wait()
    except OSError:
        # This is an infrastructure failure after spawn. Never replay. Drain
        # what can still be read and retain the native wait status.
        eligible = False
        if captured and delivery_ok and not writer(stdout_fd, bytes(captured)):
            delivery_ok = False
        captured.clear()
        if proc.poll() is None:
            proc.wait()
        native_status = proc.returncode if proc.returncode is not None else 125
    finally:
        selector.close()
        if proc.stdout is not None and not proc.stdout.closed:
            proc.stdout.close()

    if eligible and native_status == 0 and _CANCEL_SIGNAL is None:
        command = shlex.join(list(argv))
        transformed = _run_transform(pith_binary, command, bytes(captured), deadline)
        selected = (
            _selected_output(bytes(captured), transformed)
            if transformed is not None and _CANCEL_SIGNAL is None and time.monotonic() < deadline
            else None
        )
        if selected is not None:
            if delivery_ok:
                delivery_ok = writer(stdout_fd, selected)
        elif delivery_ok:
            delivery_ok = writer(stdout_fd, bytes(captured))
    elif eligible and delivery_ok:
        delivery_ok = writer(stdout_fd, bytes(captured))

    for signum, handler in previous.items():
        signal.signal(signum, handler)
    if _CANCEL_SIGNAL is not None:
        _reraise(_CANCEL_SIGNAL)
    if native_status < 0:
        _reraise(-native_status)
    # A closed output sink cannot be repaired after spawn; retain native status.
    _ = delivery_ok
    return native_status


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--run", action="store_true")
    parser.add_argument("--pith-binary", required=True)
    args, rest = parser.parse_known_args(argv)
    if args.run:
        native = rest[1:] if rest and rest[0] == "--" else rest
        return run_native(native, args.pith_binary)
    try:
        raw_payload = sys.stdin.buffer.read(HOOK_INPUT_LIMIT + 1)
        if len(raw_payload) > HOOK_INPUT_LIMIT:
            raise ValueError("hook input exceeds limit")
        payload = json.loads(raw_payload.decode("utf-8", errors="strict"))
    except (json.JSONDecodeError, UnicodeDecodeError, OSError, ValueError):
        result = _allow()
    else:
        result = rewrite_hook_input(payload, args.pith_binary)
    sys.stdout.write(json.dumps(result, separators=(",", ":")) + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
