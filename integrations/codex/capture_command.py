#!/usr/bin/env python3
"""Run one explicit argv, capture its complete bounded output, then return Pith's selection."""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import selectors
import shlex
import signal
import stat
import subprocess
import sys
import tempfile
import time
from pathlib import Path
from typing import Any


PITH_SHA256 = "ce8b2d3888e533bb237f5469e1815ee1c542121fcb0aed2142cc07f7e7e2ba98"
PITH_SOURCE_COMMIT = "bbe9fc5e05cbd20a9be2e1d98f23894fc46ff186"
CAPTURE_SCHEMA = "pith-completed-command-v1"
DELIVERY_SCHEMA = "pith-completed-delivery-v1"
MAX_ARGS = 64
MAX_ARG_BYTES = 2048
MAX_ARG_TOTAL = 16 * 1024
MAX_STDOUT = 1024 * 1024
MAX_STDERR = 256 * 1024
MAX_TRANSFORM_INPUT = 2 * 1024 * 1024
MAX_TRANSFORM_STDOUT = 512 * 1024
MAX_TRANSFORM_STDERR = 8 * 1024
MAX_SELECTED = 64 * 1024
MAX_DELIVERY = 72 * 1024
MAX_RECEIPT = 64 * 1024
MAX_EXECUTABLE = 256 * 1024 * 1024
CHUNK = 16 * 1024
CLEANUP_SECONDS = 3
TRANSFORM_SECONDS = 10
_CANCEL_SIGNAL: int | None = None


class CaptureError(Exception):
    """Short failure category; never carries command output."""


def _sha(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def _json(data: bytes, category: str) -> dict[str, Any]:
    def unique(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
        value: dict[str, Any] = {}
        for key, item in pairs:
            if key in value:
                raise CaptureError("duplicate_json_key")
            value[key] = item
        return value

    try:
        value = json.loads(
            data.decode("utf-8", errors="strict"),
            object_pairs_hook=unique,
            parse_constant=lambda _value: (_ for _ in ()).throw(CaptureError(category)),
        )
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise CaptureError(category) from exc
    if not isinstance(value, dict):
        raise CaptureError(category)
    return value


def _open_hashed(path: Path, limit: int, *, nofollow: bool = True) -> tuple[int, str, os.stat_result]:
    flags = os.O_RDONLY | (getattr(os, "O_NOFOLLOW", 0) if nofollow else 0)
    try:
        fd = os.open(path, flags)
    except OSError as exc:
        raise CaptureError("pinned_file_open_failed") from exc
    try:
        before = os.fstat(fd)
        if not stat.S_ISREG(before.st_mode) or before.st_size > limit:
            raise CaptureError("pinned_file_invalid")
        digest = hashlib.sha256()
        count = 0
        while True:
            chunk = os.read(fd, CHUNK)
            if not chunk:
                break
            count += len(chunk)
            if count > limit:
                raise CaptureError("pinned_file_too_large")
            digest.update(chunk)
        after = os.fstat(fd)
        if (before.st_dev, before.st_ino, before.st_size, before.st_mtime_ns) != (
            after.st_dev,
            after.st_ino,
            after.st_size,
            after.st_mtime_ns,
        ) or count != after.st_size:
            raise CaptureError("pinned_file_changed")
        os.lseek(fd, 0, os.SEEK_SET)
        return fd, digest.hexdigest(), after
    except BaseException:
        os.close(fd)
        raise


def _private_dir(path: Path) -> None:
    info = path.lstat()
    if (
        not stat.S_ISDIR(info.st_mode)
        or stat.S_ISLNK(info.st_mode)
        or info.st_uid != os.getuid()
        or stat.S_IMODE(info.st_mode) != 0o700
    ):
        raise CaptureError("evidence_directory_invalid")


def _command_environment(directory: Path) -> dict[str, str]:
    home = directory / "command-home"
    home.mkdir(mode=0o700)
    return {
        "PATH": os.environ.get("PATH", "/usr/local/bin:/usr/bin:/bin"),
        "HOME": str(home),
        "TMPDIR": str(directory),
        "LANG": "C",
        "LC_ALL": "C",
    }


def _write_new(path: Path, data: bytes) -> None:
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0)
    try:
        fd = os.open(path, flags, 0o600)
    except OSError as exc:
        raise CaptureError("evidence_create_failed") from exc
    with os.fdopen(fd, "wb", closefd=True) as stream:
        view = memoryview(data)
        while view:
            size = stream.write(view)
            if not size:
                raise CaptureError("evidence_write_failed")
            view = view[size:]
        stream.flush()
        os.fsync(stream.fileno())


def _cancel(signum: int, _frame: Any) -> None:
    global _CANCEL_SIGNAL
    if _CANCEL_SIGNAL is None:
        _CANCEL_SIGNAL = signum


def _kill_owned_group(proc: subprocess.Popen[bytes]) -> str | None:
    """Signal only while the unreaped session leader anchors this process group."""
    if proc.returncode is not None:
        return None
    try:
        os.killpg(proc.pid, signal.SIGKILL)
    except ProcessLookupError:
        pass
    except OSError:
        return "process_group_signal_failed"
    return None


def _group_exists(group: int) -> bool | None:
    try:
        os.killpg(group, 0)
        return True
    except ProcessLookupError:
        return False
    except PermissionError:
        return True
    except OSError:
        return None


def _settlement(proc: subprocess.Popen[bytes], streams_eof: bool) -> tuple[bool, bool | None, str | None]:
    group_exists = _group_exists(proc.pid)
    if group_exists is None:
        return False, None, "process_group_probe_failed"
    group_gone = not group_exists
    settled = proc.returncode is not None and streams_eof and group_gone
    return settled, group_gone, None


def _require_native_executable(path: Path, fd: int) -> None:
    if not path.is_absolute():
        raise CaptureError("command_path_must_be_absolute")
    if os.pread(fd, 4, 0) != b"\x7fELF":
        raise CaptureError("command_must_be_native_elf")
    if not os.access(path, os.X_OK):
        raise CaptureError("command_not_executable")


def _process(
    argv: list[str],
    *,
    cwd: Path,
    env: dict[str, str],
    timeout: int,
    out_limit: int,
    err_limit: int,
    input_bytes: bytes | None = None,
    pass_fds: tuple[int, ...] = (),
    executable_fd: int | None = None,
) -> dict[str, Any]:
    if not argv or len(argv) > MAX_ARGS or sum(len(arg.encode("utf-8", errors="strict")) for arg in argv) > MAX_ARG_TOTAL or any(
        "\0" in arg or len(arg.encode("utf-8", errors="strict")) > MAX_ARG_BYTES for arg in argv
    ):
        raise CaptureError("process_argv_invalid")
    proc: subprocess.Popen[bytes] | None = None
    selector: selectors.BaseSelector | None = None
    output = {"stdout": bytearray(), "stderr": bytearray()}
    eof = {"stdout": False, "stderr": False}
    timed_out = cancelled = limited = False
    process_error: str | None = None
    killed = False
    result: dict[str, Any] | None = None
    launch_attempted = False
    cleanup_errors: list[str] = []

    def signal_owned_group() -> None:
        assert proc is not None
        signal_error = _kill_owned_group(proc)  # type: ignore[arg-type]
        if signal_error is not None and signal_error not in cleanup_errors:
            cleanup_errors.append(signal_error)

    try:
        if _CANCEL_SIGNAL is not None:
            return {
                "stdout": b"", "stderr": b"", "exitCode": None, "signal": None,
                "settled": False, "complete": False, "timedOut": False,
                "cancelled": True, "outputLimited": False, "streamEof": False,
                "sameProcessGroupGone": None, "processError": "cancelled_before_launch",
                "launchAttempted": False, "cleanupErrors": [],
            }
        launch_attempted = True
        proc = subprocess.Popen(
            argv,
            cwd=cwd,
            env=env,
            stdin=subprocess.PIPE if input_bytes is not None else subprocess.DEVNULL,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            close_fds=True,
            pass_fds=tuple(sorted(set(pass_fds) | ({executable_fd} if executable_fd is not None else set()))),
            executable=f"/proc/self/fd/{executable_fd}" if executable_fd is not None else None,
            shell=False,
            start_new_session=True,
            bufsize=0,
        )
        assert proc.stdout is not None and proc.stderr is not None
        selector = selectors.DefaultSelector()
        limits = {"stdout": out_limit, "stderr": err_limit}
        for label, stream in (("stdout", proc.stdout), ("stderr", proc.stderr)):
            os.set_blocking(stream.fileno(), False)
            selector.register(stream, selectors.EVENT_READ, label)
        offset = 0
        if input_bytes is not None:
            assert proc.stdin is not None
            os.set_blocking(proc.stdin.fileno(), False)
            if input_bytes:
                selector.register(proc.stdin, selectors.EVENT_WRITE, "stdin")
            else:
                proc.stdin.close()
        deadline = time.monotonic() + timeout
        cleanup_deadline: float | None = None
        while True:
            now = time.monotonic()
            if _CANCEL_SIGNAL is not None and not killed:
                cancelled = True
                signal_owned_group()
                killed = True
                cleanup_deadline = now + CLEANUP_SECONDS
            if now >= deadline and not killed:
                timed_out = proc.returncode is None or not all(eof.values())
                signal_owned_group()
                killed = True
                cleanup_deadline = now + CLEANUP_SECONDS
            if proc.returncode is None and proc.poll() is not None and not killed and all(eof.values()):
                break
            if proc.returncode is not None and all(eof.values()) and not killed:
                break
            if killed and proc.poll() is not None and all(eof.values()):
                group_state = _group_exists(proc.pid)
                if group_state is False:
                    break
                if group_state is None:
                    if "process_group_probe_failed" not in cleanup_errors:
                        cleanup_errors.append("process_group_probe_failed")
            if killed and cleanup_deadline is not None and now >= cleanup_deadline:
                break
            if not selector.get_map() and proc.poll() is not None:
                break
            wait = min(0.1, max(0.0, deadline - now)) if not killed else 0.05
            for key, _mask in selector.select(wait):
                if key.data == "stdin":
                    assert input_bytes is not None and proc.stdin is not None
                    try:
                        written = os.write(proc.stdin.fileno(), input_bytes[offset : offset + CHUNK])
                        offset += written
                    except BrokenPipeError:
                        offset = len(input_bytes)
                    if offset >= len(input_bytes):
                        selector.unregister(proc.stdin)
                        proc.stdin.close()
                    continue
                label = key.data
                try:
                    data = os.read(key.fd, CHUNK)
                except BlockingIOError:
                    continue
                if not data:
                    selector.unregister(key.fileobj)
                    eof[label] = True
                    continue
                remain = limits[label] - len(output[label])
                if len(data) <= remain:
                    output[label].extend(data)
                else:
                    if remain > 0:
                        output[label].extend(data[:remain])
                    limited = True
                    if not killed:
                        signal_owned_group()
                        killed = True
                        cleanup_deadline = time.monotonic() + CLEANUP_SECONDS
        if proc.returncode is None and proc.poll() is None:
            signal_owned_group()
        try:
            proc.wait(timeout=CLEANUP_SECONDS)
        except subprocess.TimeoutExpired:
            signal_owned_group()
            try:
                proc.wait(timeout=CLEANUP_SECONDS)
            except subprocess.TimeoutExpired:
                process_error = "leader_wait_incomplete"
            except OSError:
                process_error = process_error or "leader_wait_failed"
        except OSError:
            process_error = process_error or "leader_wait_failed"
        settled, group_gone, probe_error = _settlement(proc, all(eof.values()))
        if probe_error is not None and probe_error not in cleanup_errors:
            cleanup_errors.append(probe_error)
        code = proc.returncode
        result = {
            "stdout": bytes(output["stdout"]),
            "stderr": bytes(output["stderr"]),
            "exitCode": code if code is not None and code >= 0 else None,
            "signal": -code if code is not None and code < 0 else None,
            "settled": settled,
            "complete": settled and not timed_out and not cancelled and not limited and process_error is None and not cleanup_errors,
            "timedOut": timed_out,
            "cancelled": cancelled,
            "outputLimited": limited,
            "streamEof": all(eof.values()),
            "sameProcessGroupGone": group_gone,
            "processError": process_error,
            "cleanupErrors": list(cleanup_errors),
            "launchAttempted": launch_attempted,
        }
        if process_error is not None or cleanup_errors:
            result["complete"] = False
            result["settled"] = False
        return result
    except BaseException as exc:
        process_error = "process_launch_failed" if proc is None else "process_capture_failed"
        if proc is not None and proc.returncode is None:
            signal_owned_group()
            try:
                proc.wait(timeout=CLEANUP_SECONDS)
            except subprocess.TimeoutExpired:
                signal_owned_group()
                try:
                    proc.wait(timeout=CLEANUP_SECONDS)
                except subprocess.TimeoutExpired:
                    process_error = "leader_wait_incomplete"
                except OSError:
                    cleanup_errors.append("leader_wait_failed")
            except OSError:
                cleanup_errors.append("leader_wait_failed")
        code = proc.returncode if proc is not None else None
        settled, group_gone, probe_error = (
            _settlement(proc, all(eof.values())) if proc is not None else (False, None, None)
        )
        if probe_error is not None and probe_error not in cleanup_errors:
            cleanup_errors.append(probe_error)
        result = {
            "stdout": bytes(output["stdout"]),
            "stderr": bytes(output["stderr"]),
            "exitCode": code if code is not None and code >= 0 else None,
            "signal": -code if code is not None and code < 0 else None,
            "settled": settled,
            "complete": False,
            "timedOut": timed_out,
            "cancelled": cancelled,
            "outputLimited": limited,
            "streamEof": all(eof.values()),
            "sameProcessGroupGone": group_gone,
            "processError": process_error,
            "cleanupErrors": list(cleanup_errors),
            "errorType": type(exc).__name__,
            "launchAttempted": launch_attempted,
        }
        return result
    finally:
        if selector is not None:
            try:
                selector.close()
            except OSError:
                if result is not None:
                    errors = result.setdefault("cleanupErrors", [])
                    if "selector_cleanup_failed" not in errors:
                        errors.append("selector_cleanup_failed")
        if proc is not None:
            for stream in (proc.stdin, proc.stdout, proc.stderr):
                if stream is not None:
                    try:
                        stream.close()
                    except OSError:
                        if result is not None:
                            errors = result.setdefault("cleanupErrors", [])
                            if "stream_cleanup_failed" not in errors:
                                errors.append("stream_cleanup_failed")
        if result is not None and (result.get("processError") is not None or result.get("cleanupErrors")):
            result["complete"] = False
            result["settled"] = False
            if result.get("cleanupErrors"):
                result["sameProcessGroupGone"] = None


def _validate_transform(raw: bytes, status: dict[str, Any]) -> tuple[str, dict[str, Any]]:
    if not status["complete"] or status["exitCode"] != 0 or status["signal"] is not None or status["stderr"]:
        raise CaptureError("pith_transform_failed_or_unsettled")
    response = _json(status["stdout"], "pith_transform_response_invalid")
    output = response.get("output")
    if not isinstance(output, str):
        raise CaptureError("pith_output_missing")
    encoded = output.encode("utf-8", errors="strict")
    if len(encoded) > MAX_SELECTED:
        raise CaptureError("pith_output_too_large")
    original_count = response.get("originalByteCount")
    retained_count = response.get("retainedByteCount")
    if (
        isinstance(original_count, bool)
        or not isinstance(original_count, int)
        or original_count != len(raw)
        or isinstance(retained_count, bool)
        or not isinstance(retained_count, int)
        or retained_count != len(encoded)
    ):
        raise CaptureError("pith_byte_count_mismatch")
    parser = response.get("parser")
    passthrough = response.get("passthrough")
    if not isinstance(parser, str) or not isinstance(passthrough, bool):
        raise CaptureError("pith_provenance_missing")
    strategy = response.get("minimizationStrategy")
    upstream = response.get("upstreamTruncated")
    if not isinstance(strategy, str) or not isinstance(upstream, bool):
        raise CaptureError("pith_strategy_or_truncation_missing")
    if parser:
        if passthrough or strategy != f"parser:{parser}" or upstream:
            raise CaptureError("pith_parser_provenance_inconsistent")
    else:
        if not passthrough or strategy not in ("passthrough", "upstream-host-truncation"):
            raise CaptureError("pith_passthrough_provenance_inconsistent")
        if upstream != (strategy == "upstream-host-truncation"):
            raise CaptureError("pith_truncation_provenance_inconsistent")
    response.update({"selectedSha256": _sha(encoded)})
    return output, response


def _transform_request(command: str, output: bytes, exit_code: int) -> bytes:
    try:
        text = output.decode("utf-8", errors="strict")
    except UnicodeDecodeError as exc:
        raise CaptureError("command_stdout_not_utf8") from exc
    request = json.dumps(
        {
            "command": command,
            "output": text,
            "exitCode": exit_code,
            "rawBypass": False,
            "telemetryEnabled": False,
        },
        separators=(",", ":"),
        ensure_ascii=False,
        allow_nan=False,
    ).encode("utf-8")
    if len(request) > MAX_TRANSFORM_INPUT:
        raise CaptureError("transform_input_too_large")
    return request


def _receipt(directory: Path, argv: list[str], cwd: Path, executable: dict[str, Any], adapter: dict[str, Any],
             command_env: dict[str, str], command: dict[str, Any], pith: dict[str, Any], transform: dict[str, Any],
             delivery: dict[str, Any], raw_references: dict[str, str | None],
             publication_status: dict[str, Any]) -> Path:
    value = {
        "schema": CAPTURE_SCHEMA,
        "argv": argv,
        "cwd": str(cwd),
        "commandExecutable": executable,
        "captureAdapter": adapter,
        "commandEnvironment": {
            "passedVariableNames": ["PATH", "HOME", "TMPDIR", "LANG", "LC_ALL"],
            "pathSha256": _sha(command_env.get("PATH", "").encode("utf-8")),
        },
        "commandStatus": {k: v for k, v in command.items() if k not in ("stdout", "stderr")},
        "commandOutput": {
            "stdoutBytes": len(command["stdout"]),
            "stderrBytes": len(command["stderr"]),
            "stdoutSha256": _sha(command["stdout"]),
            "stderrSha256": _sha(command["stderr"]),
        },
        "rawReferences": raw_references,
        "pith": pith,
        "transform": transform,
        "delivery": delivery,
        "publicationStatus": publication_status,
        "adapterCancelSignal": _CANCEL_SIGNAL,
    }
    path = directory / "receipt.json"
    data = json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=False, allow_nan=False).encode("utf-8")
    if len(data) > MAX_RECEIPT:
        raise CaptureError("receipt_too_large")
    _write_new(directory / ".receipt.tmp", data)
    os.replace(directory / ".receipt.tmp", path)
    fd = os.open(directory, os.O_RDONLY | getattr(os, "O_DIRECTORY", 0))
    try:
        os.fsync(fd)
    finally:
        os.close(fd)
    return path


def _write_publication_status(directory: Path, publication: dict[str, Any]) -> Path:
    path = directory / "publication.json"
    temporary = directory / ".publication.tmp"
    data = json.dumps(publication, sort_keys=True, separators=(",", ":"), allow_nan=False).encode("utf-8")
    if len(data) > 4096:
        raise CaptureError("publication_status_too_large")
    _write_new(temporary, data)
    os.replace(temporary, path)
    fd = os.open(directory, os.O_RDONLY | getattr(os, "O_DIRECTORY", 0))
    try:
        os.fsync(fd)
    finally:
        os.close(fd)
    return path


def _packet(output: str | None, command: dict[str, Any], transform: dict[str, Any],
            directory: Path, error: str | None, *,
            raw_references: dict[str, str | None] | None = None) -> dict[str, Any]:
    if raw_references is None:
        raw_references = {
            "stdout": str(directory / "stdout.raw"),
            "stderr": str(directory / "stderr.raw"),
            "receipt": str(directory / "receipt.json"),
            "publication": str(directory / "publication.json"),
        }
    return {
        "selectedOutput": output,
        "commandStatus": {k: v for k, v in command.items() if k not in ("stdout", "stderr")},
        "transformStatus": transform,
        "error": error,
        "rawReference": raw_references,
        "adapterCancelSignal": _CANCEL_SIGNAL,
    }


def _encode_packet(packet: dict[str, Any]) -> bytes:
    data = json.dumps(packet, ensure_ascii=False, separators=(",", ":"), allow_nan=False).encode("utf-8")
    if len(data) + 1 > MAX_DELIVERY:
        raise CaptureError("delivery_too_large")
    return data


def _serialize(packet: dict[str, Any]) -> bytes:
    return json.dumps(packet, ensure_ascii=False, separators=(",", ":"), allow_nan=False).encode("utf-8")


def _write_wire(data: bytes, *, cancellation_snapshot: int | None) -> dict[str, Any]:
    publication = {
        "state": "not_started", "attempted": False, "complete": False,
        "deliveryUncertain": True, "bytesWritten": 0, "expectedBytes": len(data),
        "cancelledBeforeStart": False, "cancelledDuringPublication": False,
        "error": None,
    }
    if _CANCEL_SIGNAL != cancellation_snapshot:
        publication["cancelledBeforeStart"] = True
        return publication
    view = memoryview(data)
    try:
        while view:
            publication["attempted"] = True
            publication["state"] = "uncertain"
            written = sys.stdout.buffer.write(view)
            if not isinstance(written, int) or written <= 0:
                raise OSError("stdout write did not accept bytes")
            publication["bytesWritten"] += written
            view = view[written:]
        sys.stdout.buffer.flush()
        publication["complete"] = True
        publication["deliveryUncertain"] = False
        publication["state"] = "complete"
    except Exception:
        publication["error"] = "stdout_write_failed"
        publication["deliveryUncertain"] = True
    if _CANCEL_SIGNAL != cancellation_snapshot:
        publication["cancelledDuringPublication"] = True
        publication["deliveryUncertain"] = True
        publication["state"] = "uncertain"
    return publication


def _compare_envelopes(
    packet: dict[str, Any], raw_text: str | None, *, selection_succeeded: bool
) -> tuple[bytes, dict[str, Any]]:
    comparison: dict[str, Any] = {
        "basis": "complete JSON packet with identical status and evidence references",
        "selectedEnvelopeBytes": 0,
        "rawReferenceEnvelopeBytes": None,
        "smaller": False,
    }
    packet["deliveryComparison"] = comparison
    for _attempt in range(16):
        selected_data = _encode_packet(packet) + b"\n"
        raw_size: int | None = None
        if raw_text is not None:
            raw_packet = dict(packet)
            raw_packet["selectedOutput"] = raw_text
            raw_size = len(_serialize(raw_packet)) + 1
        updated = {
            "basis": comparison["basis"],
            "selectedEnvelopeBytes": len(selected_data),
            "rawReferenceEnvelopeBytes": raw_size,
            "smaller": selection_succeeded and raw_size is not None and len(selected_data) < raw_size,
        }
        if updated == comparison:
            return selected_data, updated
        comparison = updated
        packet["deliveryComparison"] = comparison
    raise CaptureError("delivery_count_did_not_stabilize")


def _empty_process_result() -> dict[str, Any]:
    return {
        "stdout": b"", "stderr": b"", "exitCode": None, "signal": None,
        "settled": False, "complete": False, "timedOut": False, "cancelled": False,
        "outputLimited": False, "streamEof": False, "sameProcessGroupGone": False,
        "processError": None, "attempted": False,
    }


def _run(args: argparse.Namespace) -> int:
    pith_binary = Path(args.pith_binary)
    if not pith_binary.is_absolute():
        raise CaptureError("pith_path_must_be_absolute")
    argv = list(args.command)
    if argv and argv[0] == "--":
        argv = argv[1:]
    if not argv or len(argv) > MAX_ARGS:
        raise CaptureError("command_required")
    if sum(len(arg.encode("utf-8", errors="strict")) for arg in argv) > MAX_ARG_TOTAL:
        raise CaptureError("command_arguments_too_large")
    for arg in argv:
        if "\0" in arg or len(arg.encode("utf-8", errors="strict")) > MAX_ARG_BYTES:
            raise CaptureError("command_argument_invalid")
    cwd_arg = Path(args.cwd)
    if not cwd_arg.is_absolute():
        raise CaptureError("cwd_must_be_absolute")
    cwd = cwd_arg.resolve(strict=True)
    if not stat.S_ISDIR(cwd.lstat().st_mode):
        raise CaptureError("cwd_invalid")
    raw_dir = Path(tempfile.mkdtemp(prefix="pith-completed-", dir="/tmp"))
    os.chmod(raw_dir, 0o700)
    _private_dir(raw_dir)
    raw_references: dict[str, str | None] = {
        "stdout": None, "stderr": None, "receipt": None,
        "publication": str(raw_dir / "publication.json"),
    }
    command = _empty_process_result()
    transform: dict[str, Any] = {
        "attempted": False, "complete": False, "settled": False, "exitCode": None,
        "cancelled": False, "timedOut": False, "binarySha256": None,
        "sourceCommit": PITH_SOURCE_COMMIT,
    }
    command_env: dict[str, str] = {}
    executable: dict[str, Any] = {"requestedArgv0": argv[0], "resolvedPath": None}
    pith_info: dict[str, Any] = {"path": str(pith_binary), "sha256": None,
                                 "sourceCommit": PITH_SOURCE_COMMIT,
                                 "argv": [str(pith_binary), "pi", "transform"]}
    adapter_info: dict[str, Any] = {"path": str(Path(__file__).resolve(strict=False)),
                                   "sha256": None, "bytes": None}
    open_fds: list[int] = []
    selected: str | None = None
    error: str | None = None
    command_attempted = False
    try:
        command_env = _command_environment(raw_dir)
        adapter_path = Path(__file__).resolve(strict=True)
        adapter_fd, adapter_sha, adapter_stat = _open_hashed(adapter_path, MAX_EXECUTABLE)
        open_fds.append(adapter_fd)
        adapter_info = {"path": str(adapter_path), "sha256": adapter_sha, "bytes": adapter_stat.st_size}

        pith_fd, pith_sha, pith_stat = _open_hashed(pith_binary, MAX_EXECUTABLE)
        open_fds.append(pith_fd)
        pith_info.update({"sha256": pith_sha, "bytes": pith_stat.st_size})
        transform["binarySha256"] = pith_sha
        if pith_sha != PITH_SHA256:
            raise CaptureError("pinned_pith_hash_mismatch")
        if os.pread(pith_fd, 4, 0) != b"\x7fELF":
            raise CaptureError("pinned_pith_not_native_elf")

        if not Path(argv[0]).is_absolute():
            raise CaptureError("command_path_must_be_absolute")
        resolved = Path(argv[0]).resolve(strict=True)
        exe_fd, exe_sha, exe_stat = _open_hashed(resolved, MAX_EXECUTABLE, nofollow=False)
        open_fds.append(exe_fd)
        _require_native_executable(resolved, exe_fd)
        executable = {
            "requestedArgv0": argv[0], "resolvedPath": str(resolved), "sha256": exe_sha,
            "bytes": exe_stat.st_size, "device": exe_stat.st_dev, "inode": exe_stat.st_ino,
        }

        if _CANCEL_SIGNAL is not None:
            error = "adapter_cancelled_before_command"
        else:
            command_attempted = True
            command = _process(
                argv, cwd=cwd, env=command_env, timeout=args.timeout_seconds,
                out_limit=MAX_STDOUT, err_limit=MAX_STDERR, executable_fd=exe_fd,
            )
            command_attempted = bool(command.get("launchAttempted", True))
            command["attempted"] = command_attempted
            if command.get("processError"):
                error = command["processError"]

        for stream_name in ("stdout", "stderr"):
            path = raw_dir / f"{stream_name}.raw"
            try:
                _write_new(path, command[stream_name])
                raw_references[stream_name] = str(path)
            except (CaptureError, OSError):
                error = error or f"{stream_name}_evidence_write_failed"

        if error is None and not command["complete"]:
            error = "command_output_incomplete"
        if error is None and command["exitCode"] is not None and _CANCEL_SIGNAL is not None:
            error = "adapter_cancelled_before_transform"
        if error is None and command["complete"] and command["exitCode"] is not None:
            request = _transform_request(shlex.join(argv), command["stdout"], command["exitCode"])
            home, storage = raw_dir / "pith-home", raw_dir / "pith-storage"
            home.mkdir(mode=0o700)
            storage.mkdir(mode=0o700)
            pith_env = {"HOME": str(home), "PITH_STORAGE": str(storage),
                        "PATH": str(pith_binary.parent), "LANG": "C", "LC_ALL": "C"}
            if _CANCEL_SIGNAL is not None:
                error = "adapter_cancelled_before_transform"
            else:
                pith_result = _process(
                    [str(pith_binary), "pi", "transform"], cwd=raw_dir, env=pith_env,
                    timeout=TRANSFORM_SECONDS, out_limit=MAX_TRANSFORM_STDOUT,
                    err_limit=MAX_TRANSFORM_STDERR, input_bytes=request,
                    pass_fds=(pith_fd,), executable_fd=pith_fd,
                )
                transform["attempted"] = bool(pith_result.get("launchAttempted", True))
                transform.update({k: pith_result.get(k) for k in (
                    "exitCode", "signal", "complete", "settled", "timedOut", "cancelled",
                    "outputLimited", "processError", "errorType", "cleanupErrors",
                )})
                transform["stdoutBytes"] = len(pith_result["stdout"])
                transform["stderrBytes"] = len(pith_result["stderr"])
                for stream_name in ("stdout", "stderr"):
                    path = raw_dir / f"transform.{stream_name}.raw"
                    try:
                        _write_new(path, pith_result[stream_name])
                        transform[f"{stream_name}RawReference"] = str(path)
                    except (CaptureError, OSError):
                        error = error or f"transform_{stream_name}_evidence_write_failed"
                if _CANCEL_SIGNAL is not None:
                    error = error or "adapter_cancelled"
                    selected = None
                elif error is None:
                    selected, parsed = _validate_transform(command["stdout"], pith_result)
                    if _CANCEL_SIGNAL is not None:
                        selected = None
                        error = "adapter_cancelled"
                    else:
                        for key in (
                            "parser", "passthrough", "minimizationStrategy", "upstreamTruncated",
                            "originalByteCount", "retainedByteCount", "selectedSha256",
                        ):
                            transform[key] = parsed[key]
                else:
                    selected = None
        elif error is None:
            error = "command_status_unknown"
    except Exception as exc:
        error = exc.args[0] if isinstance(exc, CaptureError) else "capture_or_transform_failed"
    finally:
        for fd in reversed(open_fds):
            try:
                os.close(fd)
            except OSError:
                error = error or "descriptor_cleanup_failed"

    if _CANCEL_SIGNAL is not None:
        error = error or "adapter_cancelled"
        selected = None
    if error is not None:
        selected = None
    receipt_path = raw_dir / "receipt.json"
    raw_references["receipt"] = str(receipt_path)
    try:
        raw_text = command["stdout"].decode("utf-8", errors="strict")
    except UnicodeDecodeError:
        raw_text = None
    selection_succeeded = selected is not None and error is None and command["complete"] and transform["complete"]

    def make_packet_and_delivery(current_error: str | None, include_receipt: bool) -> tuple[dict[str, Any], bytes, dict[str, Any]]:
        refs = dict(raw_references)
        if not include_receipt:
            refs["receipt"] = None
        packet_value = _packet(selected, command, transform, raw_dir, current_error, raw_references=refs)
        try:
            wire, comparison = _compare_envelopes(
                packet_value, raw_text, selection_succeeded=selection_succeeded and current_error is None
            )
        except CaptureError:
            packet_value["selectedOutput"] = None
            packet_value["error"] = "delivery_packet_too_large"
            wire, comparison = _compare_envelopes(
                packet_value, raw_text, selection_succeeded=False
            )
        delivery_value = {
            "schema": DELIVERY_SCHEMA,
            "selectedEnvelopeBytes": len(wire),
            "selectedEnvelopeSha256": _sha(wire),
            "rawReferenceEnvelopeBytes": comparison["rawReferenceEnvelopeBytes"],
            "comparison": comparison,
            "outputItemCount": 1,
            "commandAttempts": int(command_attempted),
            "pithAttempts": int(transform["attempted"]),
        }
        return packet_value, wire, delivery_value

    def persist_receipt(delivery_value: dict[str, Any], publication: dict[str, Any]) -> None:
        _receipt(
            raw_dir, argv, cwd, executable, adapter_info, command_env, command,
            pith_info, transform, delivery_value,
            {"stdout": raw_references["stdout"], "stderr": raw_references["stderr"],
             "publication": raw_references["publication"]},
            publication,
        )

    def invalidate_selection_on_cancel() -> None:
        nonlocal selected, error, selection_succeeded
        if _CANCEL_SIGNAL is not None:
            selected = None
            error = error or "adapter_cancelled"
            selection_succeeded = False

    receipt_written = False
    packet: dict[str, Any] | None = None
    wire = b""
    delivery: dict[str, Any] = {}
    publication_status: dict[str, Any] = {}
    for _attempt in range(2):
        invalidate_selection_on_cancel()
        packet, wire, delivery = make_packet_and_delivery(error, include_receipt=True)
        publication_status = {
            "state": "pending", "deliveryUncertain": True,
            "expectedBytes": len(wire), "bytesWritten": 0,
        }
        if receipt_written:
            try:
                persist_receipt(delivery, publication_status)
            except Exception:
                error = error or "receipt_write_failed"
                selected = None
                selection_succeeded = False
                raw_references["receipt"] = None
                packet, wire, delivery = make_packet_and_delivery(error, include_receipt=False)
                receipt_written = False
                break
        else:
            try:
                persist_receipt(delivery, publication_status)
                receipt_written = True
            except Exception:
                error = error or "receipt_write_failed"
                selected = None
                selection_succeeded = False
                raw_references["receipt"] = None
                packet, wire, delivery = make_packet_and_delivery(error, include_receipt=False)
                break
        if packet.get("adapterCancelSignal") == _CANCEL_SIGNAL:
            break

    invalidate_selection_on_cancel()
    if packet is None or packet.get("adapterCancelSignal") != _CANCEL_SIGNAL:
        packet, wire, delivery = make_packet_and_delivery(error, include_receipt=receipt_written)
        if receipt_written:
            publication_status = {
                "state": "pending", "deliveryUncertain": True,
                "expectedBytes": len(wire), "bytesWritten": 0,
            }
            try:
                persist_receipt(delivery, publication_status)
            except Exception:
                error = error or "receipt_write_failed"
                selected = None
                selection_succeeded = False
                raw_references["receipt"] = None
                receipt_written = False
                packet, wire, delivery = make_packet_and_delivery(error, include_receipt=False)

    publication = _write_wire(wire, cancellation_snapshot=packet["adapterCancelSignal"])
    if publication["cancelledBeforeStart"]:
        invalidate_selection_on_cancel()
        packet, wire, delivery = make_packet_and_delivery(error, include_receipt=receipt_written)
        publication_status = {
            "state": "pending", "deliveryUncertain": True,
            "expectedBytes": len(wire), "bytesWritten": 0,
        }
        if receipt_written:
            try:
                persist_receipt(delivery, publication_status)
            except Exception:
                error = error or "receipt_write_failed"
                selected = None
                selection_succeeded = False
                raw_references["receipt"] = None
                receipt_written = False
                packet, wire, delivery = make_packet_and_delivery(error, include_receipt=False)
        publication = _write_wire(wire, cancellation_snapshot=packet["adapterCancelSignal"])

    publication_persisted = False
    cancellation_at_packet = packet["adapterCancelSignal"]
    cancellation_before_status = _CANCEL_SIGNAL
    try:
        publication["adapterCancelSignal"] = _CANCEL_SIGNAL
        _write_publication_status(raw_dir, publication)
        publication_persisted = True
    except Exception:
        # The pre-publication receipt keeps its pending/uncertain publication state.
        publication["deliveryUncertain"] = True
        publication["statusWriteFailed"] = True
    cancelled_during_status = _CANCEL_SIGNAL != cancellation_before_status
    cancelled_after_packet = _CANCEL_SIGNAL != cancellation_at_packet
    if cancelled_during_status or cancelled_after_packet:
        publication["cancelledDuringFinalization"] = True
        publication["adapterCancelSignal"] = _CANCEL_SIGNAL
        publication_persisted = False
        try:
            _write_publication_status(raw_dir, publication)
            publication_persisted = True
        except Exception:
            publication["statusWriteFailed"] = True
    if (
        publication["complete"]
        and not publication["deliveryUncertain"]
        and publication_persisted
        and command["exitCode"] is not None
        and _CANCEL_SIGNAL is None
    ):
        return min(command["exitCode"], 255)
    return 125


def _parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--cwd", required=True, help="Absolute working directory for the command")
    parser.add_argument(
        "--pith-binary", required=True,
        help="Absolute path to the published v3.0.7 Linux binary with the pinned SHA-256",
    )
    parser.add_argument("--timeout-seconds", type=int, default=120)
    parser.add_argument("command", nargs=argparse.REMAINDER, help="Explicit child argv after --")
    return parser


def main() -> int:
    global _CANCEL_SIGNAL
    _CANCEL_SIGNAL = None
    prior_handlers: dict[int, Any] = {}
    try:
        for signum in (signal.SIGINT, signal.SIGTERM):
            prior_handlers[signum] = signal.signal(signum, _cancel)
        try:
            args = _parser().parse_args()
            if not 1 <= args.timeout_seconds <= 600:
                raise CaptureError("timeout_out_of_range")
            return _run(args)
        except Exception as exc:
            category = exc.args[0] if isinstance(exc, CaptureError) else "capture_failed"
            packet = {
                "selectedOutput": None,
                "commandStatus": {"attempted": False, "complete": False,
                                  "settled": False, "exitCode": None},
                "transformStatus": {"attempted": False, "complete": False, "exitCode": None},
                "error": str(category)[:96],
                "rawReference": None,
                "adapterCancelSignal": _CANCEL_SIGNAL,
            }
            _write_wire(_encode_packet(packet) + b"\n", cancellation_snapshot=_CANCEL_SIGNAL)
            return 125
    finally:
        for signum, handler in prior_handlers.items():
            signal.signal(signum, handler)


if __name__ == "__main__":
    raise SystemExit(main())
