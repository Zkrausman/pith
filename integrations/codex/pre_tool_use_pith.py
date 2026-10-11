#!/usr/bin/env python3
"""Codex Bash PreToolUse adapter for a narrow, literal Go test command."""

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
RECORD_MAX_REQUEST = 2048
_CANCEL_SIGNALS = {signal.SIGINT, signal.SIGTERM}
_CANCEL_SIGNAL: int | None = None
_SUMMARY_OK = re.compile(r"ok\s+\S+\s+\d+(?:\.\d+)?s\Z")
_PACKAGE = re.compile(r"(?:\.|\./[A-Za-z0-9_./-]+|[A-Za-z0-9_./-]+)\Z")
_RUN = re.compile(r"=== RUN\s+(\S+)\Z")
_PAUSE = re.compile(r"=== PAUSE\s+\S+\Z")
_CONT = re.compile(r"=== CONT\s+\S+\Z")
_PASS = re.compile(r"--- PASS:\s+(\S+)\s+\(\d+(?:\.\d+)?s\)\Z")


def _absolute_go_test(command: str) -> list[str] | None:
    """Accept one literal, single-package Go test command without shell syntax."""
    if not command or any(ch in command for ch in "\x00\r\n;&|()<>`$*?[]{}~"):
        return None
    try:
        argv = shlex.split(command, posix=True)
        for value in argv:
            value.encode("utf-8", errors="strict")
    except (ValueError, UnicodeError):
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
    native = _absolute_go_test(tool_input["command"])
    if native is None:
        return _allow()
    wrapper = [
        os.path.realpath(sys.executable), "-I", "-S", os.path.realpath(__file__),
        "--run", "--pith-binary", pith_binary, "--", *native,
    ]
    try:
        updated = shlex.join(wrapper)
        updated.encode("utf-8", errors="strict")
    except UnicodeError:
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


def _selected_output(raw: bytes, response_bytes: bytes) -> bytes | None:
    summary = _valid_go_verbose_success(raw)
    if summary is None:
        return None
    try:
        response = json.loads(response_bytes.decode("utf-8", errors="strict"))
    except (UnicodeDecodeError, json.JSONDecodeError, RecursionError):
        return None
    if not isinstance(response, dict):
        return None
    selected = response.get("output")
    if response.get("parser") != "tests" or response.get("passthrough") is not False or not isinstance(selected, str):
        return None
    encoded = _encode_selected_utf8(selected)
    if encoded is None:
        return None
    lines = [line.strip() for line in selected.splitlines() if line.strip()]
    if lines != [summary[0], summary[1]]:
        return None
    if not encoded or len(encoded) >= len(raw):
        return None
    return encoded


def _record_wrapper_outcome(
    pith_binary: str, original: bytes, delivered: bytes, duration_ms: int,
    outcome: str, *, timeout: float = RECORD_TIMEOUT_SECONDS,
) -> None:
    """Best-effort aggregate accounting after stdout publication; never retain text."""
    if outcome not in ("transformed", "raw_fallback") or duration_ms < 0 or duration_ms > RECORD_MAX_DURATION_MS:
        return
    try:
        original_text = original.decode("utf-8", errors="strict")
        delivered_text = delivered.decode("utf-8", errors="strict")
        original_runes = len(original_text)
        delivered_runes = len(delivered_text)
        if original_runes > RECORD_MAX_RUNES or delivered_runes > RECORD_MAX_RUNES:
            return
        if outcome == "raw_fallback" and original_runes != delivered_runes:
            return
        if outcome == "transformed" and delivered_runes > original_runes:
            return
        request = json.dumps(
            {"originalRunes": original_runes, "deliveredRunes": delivered_runes,
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
    launch_deadline: float | None = None,
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
                "--executable", argv[0], "--", *argv,
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
    try:
        return any(ready_fd == fd for ready_fd, _ in poller.poll(timeout_ms))
    except InterruptedError:
        return False
    except OSError:
        try:
            readable, writable, exceptional = select.select([fd], [fd], [fd], timeout_ms / 1000)
            return bool(readable or writable or exceptional)
        except (OSError, ValueError):
            return False


def _run_transform(
    pith_binary: str, command: str, output: bytes, deadline: float, *,
    output_limit: int = TRANSFORM_OUTPUT_LIMIT,
    native_status: int | None = 0,
    warning_writer: Callable[[str, int | None], None] = _stderr_delivery_warning,
) -> bytes | None:
    if time.monotonic() >= deadline or _valid_go_verbose_success(output) is None or _CANCEL_SIGNAL is not None:
        return None
    try:
        text = output.decode("utf-8", errors="strict")
    except UnicodeDecodeError:
        return None
    try:
        request = json.dumps(
            {"command": command, "output": text, "exitCode": 0, "rawBypass": False, "telemetryEnabled": False},
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


def _exec_original(argv: Sequence[str]) -> int:
    try:
        os.execve(argv[0], list(argv), os.environ)
    except FileNotFoundError:
        return 127
    except OSError:
        return 126
    return 126


def _exec_after_setup_failure(argv: Sequence[str], previous: dict[int, object]) -> int:
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
    return _exec_original(argv)


def run_native(
    argv: Sequence[str], pith_binary: str, *,
    capture_limit: int = CAPTURE_LIMIT, capture_seconds: float = CAPTURE_SECONDS,
    transform_seconds: float = TRANSFORM_SECONDS,
    stdout_fd: int = 1, writer: Callable[[int, bytes], bool] = _write_all,
    warning_writer: Callable[[str, int | None], None] = _stderr_delivery_warning,
    record_outcome: Callable[[str, bytes, bytes, int, str], None] | None = None,
) -> int:
    """Run the already-authorized native command once; transform eligible stdout."""
    global _CANCEL_SIGNAL
    if not argv or not os.path.isabs(argv[0]):
        return 126
    if _must_exec_original():
        return _exec_original(argv)
    if _absolute_go_test(shlex.join(list(argv))) != list(argv):
        return _exec_original(argv)

    _CANCEL_SIGNAL = None
    previous: dict[int, object] = {}

    def note_cancel(signum: int, _frame: object) -> None:
        global _CANCEL_SIGNAL
        if _CANCEL_SIGNAL is None:
            _CANCEL_SIGNAL = signum

    for signum in _CANCEL_SIGNALS:
        previous[signum] = signal.signal(signum, note_cancel)

    try:
        read_fd, write_fd = os.pipe()
        os.set_blocking(read_fd, False)
        poller = _new_pipe_poller(read_fd)
    except OSError:
        for fd in (locals().get("read_fd"), locals().get("write_fd")):
            if isinstance(fd, int):
                try:
                    os.close(fd)
                except OSError:
                    pass
        return _exec_after_setup_failure(argv, previous)

    capture_deadline = time.monotonic() + capture_seconds
    native_started_ns = time.monotonic_ns()
    proc, released, canceled_before_release, spawn_failed = _spawn_gated(
        argv, stdin=None, stdout=write_fd, stderr=None,
    )
    os.close(write_fd)
    if proc is None or spawn_failed:
        if proc is not None:
            _stop_reap(proc)
        os.close(read_fd)
        poller.unregister(read_fd)
        return _exec_after_setup_failure(argv, previous)

    captured = bytearray()
    eligible = released and not canceled_before_release
    eof = False
    delivery_ok = True
    read_failure = False
    read_errors = 0
    native_status: int | None = None
    native_duration_ms = 0
    published_stdout: bytes | None = None
    published_outcome = "raw_fallback"
    try:
        while not eof or proc.poll() is None:
            remaining = capture_deadline - time.monotonic()
            if eligible and (remaining <= 0 or _CANCEL_SIGNAL is not None):
                eligible = False
                if captured and not writer(stdout_fd, bytes(captured)):
                    delivery_ok = False
                captured.clear()

            if eof:
                time.sleep(0.005)
                continue
            timeout_ms = min(50, max(1, int(remaining * 1000))) if eligible else 50
            ready = _wait_fd(poller, read_fd, timeout_ms)
            if not ready:
                # If poll/select both failed, nonblocking reads still let us
                # drain whenever bytes arrive while the deadline advances.
                time.sleep(0.005)
                ready = True
            max_read = min(READ_CHUNK, max(1, capture_limit - len(captured) + 1)) if eligible else READ_CHUNK
            try:
                chunk = os.read(read_fd, max_read) if ready else None
            except (BlockingIOError, InterruptedError):
                continue
            except OSError:
                eligible = False
                if captured and not writer(stdout_fd, bytes(captured)):
                    delivery_ok = False
                captured.clear()
                read_errors += 1
                if read_errors < 3:
                    # A transient read error must not turn into a wait on the
                    # child with its pipe undrained. Retry briefly in raw mode.
                    time.sleep(0.005)
                    continue
                read_failure = True
                delivery_ok = False
                try:
                    poller.unregister(read_fd)
                except (KeyError, OSError):
                    pass
                try:
                    os.close(read_fd)
                except OSError:
                    pass
                eof = True
                continue
            if chunk == b"":
                eof = True
                poller.unregister(read_fd)
                os.close(read_fd)
                continue
            if chunk is None:
                continue
            read_errors = 0
            if eligible and len(captured) + len(chunk) <= capture_limit:
                captured.extend(chunk)
                continue
            if eligible:
                eligible = False
                if captured and not writer(stdout_fd, bytes(captured)):
                    delivery_ok = False
                captured.clear()
            if delivery_ok and not writer(stdout_fd, chunk):
                delivery_ok = False
        native_status = proc.poll()
        native_duration_ms = max(0, (time.monotonic_ns() - native_started_ns) // 1_000_000)
        if eligible and time.monotonic() >= capture_deadline:
            eligible = False
            if captured and not writer(stdout_fd, bytes(captured)):
                delivery_ok = False
            captured.clear()
        if eligible and native_status == 0 and _CANCEL_SIGNAL is None and delivery_ok:
            transform_deadline = time.monotonic() + transform_seconds
            transformed = _run_transform(
                pith_binary, shlex.join(list(argv)), bytes(captured), transform_deadline,
                native_status=native_status, warning_writer=warning_writer,
            )
            selected = (
                _selected_output(bytes(captured), transformed)
                if transformed is not None and _CANCEL_SIGNAL is None and time.monotonic() < transform_deadline
                else None
            )
            if selected is not None:
                published_stdout = selected
                published_outcome = "transformed"
                delivery_ok = writer(stdout_fd, selected)
            elif delivery_ok:
                published_stdout = bytes(captured)
                delivery_ok = writer(stdout_fd, bytes(captured))
        elif eligible and delivery_ok:
            published_stdout = bytes(captured)
            delivery_ok = writer(stdout_fd, bytes(captured))

        if not delivery_ok:
            warning_writer("stdout capture read failed" if read_failure else "stdout publication incomplete", native_status)
    finally:
        try:
            poller.unregister(read_fd)
        except (KeyError, OSError):
            pass
        try:
            os.close(read_fd)
        except OSError:
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
                    recorder(pith_binary, bytes(captured), published_stdout, native_duration_ms, published_outcome)
                except Exception:
                    # Telemetry is post-publication best effort and cannot change
                    # the already completed native command result.
                    pass
                finally:
                    # Keep cancellation latched while the bounded recorder is
                    # running, then restore the caller's handlers atomically
                    # with respect to pending signals.
                    old_mask = signal.pthread_sigmask(signal.SIG_BLOCK, _CANCEL_SIGNALS)
                    cancel = _CANCEL_SIGNAL or _pending_cancel_signal()
                    for signum, handler in previous.items():
                        signal.signal(signum, handler)
                    signal.pthread_sigmask(signal.SIG_SETMASK, old_mask)
                if cancel is not None:
                    _reraise(cancel)
        except (UnicodeDecodeError, OverflowError):
            pass
    return 125 if native_status is None else native_status


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--run", action="store_true")
    parser.add_argument("--pith-binary")
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
        return run_native(native, args.pith_binary)
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
            result = rewrite_hook_input(payload, args.pith_binary)
    try:
        serialized = json.dumps(result, separators=(",", ":"))
    except (TypeError, ValueError, RecursionError):
        serialized = json.dumps(_allow(), separators=(",", ":"))
    sys.stdout.write(serialized + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
