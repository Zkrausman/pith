#!/usr/bin/env python3
"""Profile-bound Codex Bash PreToolUse adapter for one Go test package."""

from __future__ import annotations

import argparse
import errno
import hashlib
import json
import os
import re
import select
import shlex
import signal
import stat
import subprocess
import sys
import time
from typing import Callable, Sequence


CAPTURE_LIMIT = 1 << 20
READ_CHUNK = 32 << 10
BUFFER_SECONDS = 2.0
TRANSFORM_OUTPUT_LIMIT = 1 << 20
HOOK_INPUT_LIMIT = 64 << 10
PROFILE_LIMIT = 16 << 10
_CANCEL_SIGNALS = {signal.SIGINT, signal.SIGTERM}
_CANCEL_SIGNAL: int | None = None
_HEX_40 = re.compile(r"[0-9a-f]{40}\Z")
_HEX_64 = re.compile(r"[0-9a-f]{64}\Z")
_SUMMARY_OK = re.compile(r"ok\s+\S+\s+\d+(?:\.\d+)?s\Z")
_PACKAGE = re.compile(r"(?:\.|\./[A-Za-z0-9_./-]+|[A-Za-z0-9_./-]+)\Z")
_RUN = re.compile(r"=== RUN\s+(\S+)\Z")
_PAUSE = re.compile(r"=== PAUSE\s+\S+\Z")
_CONT = re.compile(r"=== CONT\s+\S+\Z")
_PASS = re.compile(r"--- PASS:\s+(\S+)\s+\(\d+(?:\.\d+)?s\)\Z")


def _file_sha256(path: str) -> str | None:
    try:
        digest = hashlib.sha256()
        with open(path, "rb", buffering=0) as stream:
            while chunk := stream.read(READ_CHUNK):
                digest.update(chunk)
        return digest.hexdigest()
    except (OSError, UnicodeError):
        return None


def _identity_matches(identity: object, *, executable: bool) -> bool:
    if not isinstance(identity, dict) or set(identity) != {"path", "sha256"}:
        return False
    path, digest = identity["path"], identity["sha256"]
    if not isinstance(path, str) or not os.path.isabs(path) or os.path.realpath(path) != path:
        return False
    if not isinstance(digest, str) or not _HEX_64.fullmatch(digest):
        return False
    try:
        info = os.stat(path)
    except OSError:
        return False
    if not stat.S_ISREG(info.st_mode) or (executable and not info.st_mode & 0o111):
        return False
    return _file_sha256(path) == digest


def _load_profile(path: str, expected_sha256: str) -> dict[str, object] | None:
    if not os.path.isabs(path) or not _HEX_64.fullmatch(expected_sha256):
        return None
    try:
        canonical = os.path.realpath(path)
        info = os.stat(canonical)
        if (
            not stat.S_ISREG(info.st_mode)
            or info.st_uid != os.getuid()
            or stat.S_IMODE(info.st_mode) != 0o600
            or info.st_size > PROFILE_LIMIT
        ):
            return None
        with open(canonical, "rb") as stream:
            raw = stream.read(PROFILE_LIMIT + 1)
    except (OSError, UnicodeError):
        return None
    if len(raw) > PROFILE_LIMIT or hashlib.sha256(raw).hexdigest() != expected_sha256:
        return None
    try:
        profile = json.loads(raw.decode("utf-8", errors="strict"))
    except (UnicodeDecodeError, json.JSONDecodeError):
        return None
    required = {
        "schemaVersion", "cwd", "repoRoot", "sourceHead", "sourceTree", "package",
        "go", "pith", "git", "python", "adapter",
    }
    if not isinstance(profile, dict) or set(profile) != required or profile.get("schemaVersion") != 1:
        return None
    for key in ("cwd", "repoRoot"):
        value = profile.get(key)
        if not isinstance(value, str) or not os.path.isabs(value) or os.path.realpath(value) != value:
            return None
    if profile["cwd"] != profile["repoRoot"]:
        return None
    if not isinstance(profile.get("sourceHead"), str) or not _HEX_40.fullmatch(profile["sourceHead"]):
        return None
    if not isinstance(profile.get("sourceTree"), str) or not _HEX_40.fullmatch(profile["sourceTree"]):
        return None
    package = profile.get("package")
    if not isinstance(package, str) or not _PACKAGE.fullmatch(package) or "..." in package:
        return None
    for key in ("go", "pith", "git", "python"):
        if not _identity_matches(profile.get(key), executable=True):
            return None
    if not _identity_matches(profile.get("adapter"), executable=False):
        return None
    if profile["python"]["path"] != os.path.realpath(sys.executable):
        return None
    if profile["adapter"]["path"] != os.path.realpath(__file__):
        return None
    return profile


def _absolute_go_test(command: str, profile: dict[str, object]) -> list[str] | None:
    """Accept exactly the profile's executable and one literal package argv."""
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
    try:
        canonical = os.path.realpath(executable)
    except (OSError, UnicodeError):
        return None
    if (
        not os.path.isabs(executable)
        or canonical != profile["go"]["path"]
        or os.path.basename(executable) != "go"
        or subcommand != "test"
        or verbose != "-v"
        or package != profile["package"]
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


def rewrite_hook_input(payload: object, profile_path: str, profile_sha256: str) -> dict[str, object]:
    if not isinstance(payload, dict) or payload.get("tool_name") != "Bash":
        return _allow()
    tool_input = payload.get("tool_input")
    if not isinstance(tool_input, dict) or not isinstance(tool_input.get("command"), str):
        return _allow()
    profile = _load_profile(profile_path, profile_sha256)
    if profile is None:
        return _allow()
    native = _absolute_go_test(tool_input["command"], profile)
    if native is None:
        return _allow()
    wrapper = [
        profile["python"]["path"], "-I", "-S", profile["adapter"]["path"], "--run",
        "--profile", os.path.realpath(profile_path), "--profile-sha256", profile_sha256,
        "--", *native,
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


def _source_matches(profile: dict[str, object]) -> bool:
    root = profile["repoRoot"]
    if os.path.realpath(os.getcwd()) != profile["cwd"]:
        return False
    env = os.environ.copy()
    env["GIT_OPTIONAL_LOCKS"] = "0"
    prefix = [profile["git"]["path"], "-c", "core.fsmonitor=false", "-C", root]
    try:
        rev = subprocess.run(
            prefix + ["rev-parse", "--show-toplevel", "HEAD", "HEAD^{tree}"],
            cwd=root, env=env, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL, timeout=0.5, check=False,
        )
        if rev.returncode != 0 or len(rev.stdout) > 1024:
            return False
        lines = rev.stdout.decode("utf-8", errors="strict").splitlines()
        if len(lines) != 3 or os.path.realpath(lines[0]) != root:
            return False
        if lines[1] != profile["sourceHead"] or lines[2] != profile["sourceTree"]:
            return False
        return _git_status_clean(prefix, root, env)
    except (OSError, subprocess.TimeoutExpired, UnicodeDecodeError):
        return False


def _git_status_clean(prefix: list[str], root: str, env: dict[str, str]) -> bool:
    """Check status without buffering an arbitrarily large dirty-tree listing."""
    proc: subprocess.Popen[bytes] | None = None
    try:
        proc = subprocess.Popen(
            prefix + ["status", "--porcelain", "--untracked-files=normal"],
            cwd=root, env=env, stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
            stderr=subprocess.DEVNULL, close_fds=True,
        )
        assert proc.stdout is not None
        fd = proc.stdout.fileno()
        os.set_blocking(fd, False)
        poller = select.poll()
        poller.register(fd, select.POLLIN | select.POLLHUP | select.POLLERR)
        deadline = time.monotonic() + 0.5
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                _stop_reap(proc)
                return False
            try:
                events = poller.poll(max(1, int(remaining * 1000)))
            except InterruptedError:
                continue
            if not events:
                continue
            try:
                first = os.read(fd, 1)
            except (BlockingIOError, InterruptedError):
                continue
            if first:
                _stop_reap(proc)
                return False
            try:
                return proc.wait(timeout=max(0.001, deadline - time.monotonic())) == 0
            except subprocess.TimeoutExpired:
                _stop_reap(proc)
                return False
    except (OSError, ValueError):
        if proc is not None:
            _stop_reap(proc)
        return False
    finally:
        if proc is not None and proc.stdout is not None:
            proc.stdout.close()


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
    if response.get("parser") != "tests" or response.get("passthrough") is not False or not isinstance(selected, str):
        return None
    lines = [line.strip() for line in selected.splitlines() if line.strip()]
    if lines != [summary[0], summary[1]]:
        return None
    try:
        encoded = selected.encode("utf-8", errors="strict")
    except UnicodeEncodeError:
        return None
    if not encoded or len(encoded) >= len(raw):
        return None
    return encoded


def _pending_cancel() -> bool:
    try:
        return bool(signal.sigpending().intersection(_CANCEL_SIGNALS))
    except (AttributeError, OSError, ValueError):
        return True


def _reraise(signum: int) -> None:
    """Terminate this adapter with the same signal observed from the command."""
    signal.signal(signum, signal.SIG_DFL)
    signal.pthread_sigmask(signal.SIG_UNBLOCK, {signum})
    os.kill(os.getpid(), signum)
    raise SystemExit(128 + signum)


def _write_gate(fd: int, child_pid: int) -> None:
    if os.write(fd, b"G") != 1:
        raise OSError("could not release exec gate")


def _stop_reap(proc: subprocess.Popen[bytes], *, grace_seconds: float = 0.05) -> None:
    """Stop a supporting child and reap it with a fixed, small wait bound."""
    if proc.poll() is None:
        try:
            proc.kill()
        except ProcessLookupError:
            pass
    try:
        proc.wait(timeout=grace_seconds)
    except subprocess.TimeoutExpired:
        pass


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
    argv: Sequence[str], profile: dict[str, object], *,
    stdin: int | None, stdout: int, stderr: int | None,
) -> tuple[subprocess.Popen[bytes] | None, bool, bool, bool]:
    """Return process, gate released, canceled before release, spawn failure."""
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
                profile["python"]["path"], "-I", "-S", profile["adapter"]["path"],
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
    profile: dict[str, object], command: str, output: bytes, deadline: float,
    *, output_limit: int = TRANSFORM_OUTPUT_LIMIT,
) -> bytes | None:
    if _valid_go_verbose_success(output) is None or _CANCEL_SIGNAL is not None:
        return None
    try:
        text = output.decode("utf-8", errors="strict")
    except UnicodeDecodeError:
        return None
    request = json.dumps(
        {"command": command, "output": text, "exitCode": 0, "rawBypass": False, "telemetryEnabled": False},
        ensure_ascii=False, separators=(",", ":"),
    ).encode("utf-8")
    pith = profile["pith"]
    if not _identity_matches(pith, executable=True) or _CANCEL_SIGNAL is not None:
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

    proc, released, canceled, spawn_failed = _spawn_gated(
        [pith["path"], "pi", "transform"], profile,
        stdin=child_stdin, stdout=child_stdout, stderr=subprocess.DEVNULL,
    )
    os.close(child_stdin)
    os.close(child_stdout)
    if proc is None or not released or canceled or spawn_failed:
        if proc is not None:
            _stop_reap(proc)
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
            _stop_reap(proc)
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
    if failed or proc.returncode != 0:
        return None
    return bytes(response)


def _stderr_delivery_warning(reason: str, native_status: int | None) -> None:
    status = "unknown" if native_status is None else str(native_status)
    _warn_delivery(f"{reason}; native status={status}; stdout was not retried")


def _exec_original(argv: Sequence[str]) -> int:
    try:
        os.execve(argv[0], list(argv), os.environ)
    except FileNotFoundError:
        return 127
    except OSError:
        return 126
    return 126


def run_native(
    argv: Sequence[str], profile_path: str, profile_sha256: str, *,
    capture_limit: int = CAPTURE_LIMIT, buffer_seconds: float = BUFFER_SECONDS,
    stdout_fd: int = 1, writer: Callable[[int, bytes], bool] = _write_all,
    warning_writer: Callable[[str, int | None], None] = _stderr_delivery_warning,
) -> int:
    """Run one profile-bound command; buffer briefly or relay native bytes."""
    global _CANCEL_SIGNAL
    if not argv or not os.path.isabs(argv[0]):
        return 126
    if _must_exec_original():
        return _exec_original(argv)
    profile = _load_profile(profile_path, profile_sha256)
    if profile is None or _absolute_go_test(shlex.join(list(argv)), profile) != list(argv):
        return _exec_original(argv)
    if not _source_matches(profile):
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
        for signum, handler in previous.items():
            signal.signal(signum, handler)
        return _exec_original(argv)

    deadline = time.monotonic() + buffer_seconds
    proc, released, canceled_before_release, spawn_failed = _spawn_gated(
        argv, profile, stdin=None, stdout=write_fd, stderr=None,
    )
    os.close(write_fd)
    if proc is None or spawn_failed:
        if proc is not None:
            _stop_reap(proc)
        os.close(read_fd)
        poller.unregister(read_fd)
        for signum, handler in previous.items():
            signal.signal(signum, handler)
        if _CANCEL_SIGNAL is not None or _pending_cancel():
            if _CANCEL_SIGNAL is None:
                _CANCEL_SIGNAL = signal.SIGTERM
            _reraise(_CANCEL_SIGNAL)
        return _exec_original(argv)

    captured = bytearray()
    eligible = released and not canceled_before_release
    eof = False
    delivery_ok = True
    read_failure = False
    read_errors = 0
    native_status: int | None = None
    try:
        while not eof or proc.poll() is None:
            remaining = deadline - time.monotonic()
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
        if eligible and native_status == 0 and _CANCEL_SIGNAL is None and delivery_ok:
            transformed = _run_transform(profile, shlex.join(list(argv)), bytes(captured), deadline)
            selected = (
                _selected_output(bytes(captured), transformed)
                if transformed is not None and _CANCEL_SIGNAL is None and time.monotonic() < deadline
                else None
            )
            if selected is not None:
                delivery_ok = writer(stdout_fd, selected)
            elif delivery_ok:
                delivery_ok = writer(stdout_fd, bytes(captured))
        elif eligible and delivery_ok:
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
    return 125 if native_status is None else native_status


def main(argv: Sequence[str] | None = None) -> int:
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--run", action="store_true")
    parser.add_argument("--profile")
    parser.add_argument("--profile-sha256")
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
        if not args.profile or not args.profile_sha256:
            return 126
        return run_native(native, args.profile, args.profile_sha256)
    if not args.profile or not args.profile_sha256:
        result = _allow()
    else:
        try:
            raw_payload = sys.stdin.buffer.read(HOOK_INPUT_LIMIT + 1)
            if len(raw_payload) > HOOK_INPUT_LIMIT:
                raise ValueError("hook input exceeds limit")
            payload = json.loads(raw_payload.decode("utf-8", errors="strict"))
        except (json.JSONDecodeError, UnicodeDecodeError, OSError, ValueError):
            result = _allow()
        else:
            result = rewrite_hook_input(payload, args.profile, args.profile_sha256)
    sys.stdout.write(json.dumps(result, separators=(",", ":")) + "\n")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
