"""Focused source-contract tests. These are authored but not run in this task."""

from __future__ import annotations

import hashlib
import io
import json
import os
import tempfile
import unittest
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch

import capture_command as adapter


def _vitest_raw() -> str:
    lines = [f"routine diagnostic {number:03d}" for number in range(1, 101)]
    lines[99] = "PITH_RAW_ONLY_SENTINEL must remain only in the capture"
    return "\n".join(lines) + "\n"


def _vitest_selected(raw: str) -> str:
    # Derived from pkg/parser/vitest.go at the pinned v3.0.7 source commit.
    lines = raw.split("\n")
    selected = [line.strip() for line in lines if line.strip()][:20]
    if len(lines) > 50:
        selected.append(f"... (+ {len(lines) - len(selected)} more lines, no summary found)")
    return "\n".join(selected)


def _successful_transform(raw: bytes) -> dict:
    output = "short mocked selection"
    response = {
        "output": output,
        "parser": "vitest",
        "passthrough": False,
        "originalByteCount": len(raw),
        "retainedByteCount": len(output.encode("utf-8")),
        "minimizationStrategy": "parser:vitest",
        "upstreamTruncated": False,
    }
    return {
        "stdout": json.dumps(response, separators=(",", ":")).encode("utf-8"),
        "stderr": b"", "exitCode": 0, "signal": None, "settled": True,
        "complete": True, "timedOut": False, "cancelled": False,
        "outputLimited": False, "streamEof": True, "sameProcessGroupGone": True,
        "processError": None, "attempted": True, "launchAttempted": True,
    }


class _PartialWriter:
    def __init__(self):
        self.data = bytearray()
        self.intended = b""
        self.calls = 0

    def write(self, data):
        self.calls += 1
        if self.calls == 1:
            self.intended = bytes(data)
            accepted = bytes(data[:7])
            self.data.extend(accepted)
            return len(accepted)
        raise OSError("mock partial stdout failure")

    def flush(self):
        return None

    def getvalue(self):
        return bytes(self.data)


class CaptureCommandContractTests(unittest.TestCase):
    def _run_mocked(self, command_result, transform_result=None, *, cancel_after_command=False,
                    cancel_during_transform_persist=False, cancel_during_validation=False,
                    receipt_failure=False, partial_output=False,
                    cancel_during_publication_persist=False):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            native = root / "native-elf"
            native.write_bytes(b"\x7fELF" + b"mock executable bytes")
            native.chmod(0o700)
            pith_binary = root / "pith-elf"
            pith_binary.write_bytes(b"\x7fELF" + b"mock pinned Pith bytes")
            pith_binary.chmod(0o700)
            output = _PartialWriter() if partial_output else io.BytesIO()
            stdout = SimpleNamespace(buffer=output)
            real_open_hashed = adapter._open_hashed
            real_mkdtemp = tempfile.mkdtemp
            real_write_new = adapter._write_new
            real_validate = adapter._validate_transform
            real_receipt = adapter._receipt
            real_publication_status = adapter._write_publication_status
            opened_fds = []
            publication_cancel_sent = False

            def fake_open_hashed(path, limit, *, nofollow=True):
                if Path(path) == pith_binary:
                    fd = os.open(pith_binary, os.O_RDONLY)
                    opened_fds.append(fd)
                    return fd, adapter.PITH_SHA256, os.fstat(fd)
                result = real_open_hashed(path, limit, nofollow=nofollow)
                opened_fds.append(result[0])
                return result

            def temp_in_test_root(*, prefix, dir):
                self.assertEqual(dir, "/tmp")
                return real_mkdtemp(prefix=prefix, dir=root)

            process_results = [command_result]
            if transform_result is not None:
                process_results.append(transform_result)

            def fake_process(*_args, **_kwargs):
                result = dict(process_results.pop(0))
                if cancel_after_command and not process_results:
                    adapter._cancel(adapter.signal.SIGTERM, None)
                return result

            def fake_write_new(path, data):
                result = real_write_new(path, data)
                if cancel_during_transform_persist and Path(path).name == "transform.stdout.raw":
                    adapter._cancel(adapter.signal.SIGINT, None)
                return result

            def fake_validate(raw, status):
                result = real_validate(raw, status)
                if cancel_during_validation:
                    adapter._cancel(adapter.signal.SIGTERM, None)
                return result

            def fake_receipt(*args, **kwargs):
                if receipt_failure:
                    raise OSError("mock receipt persistence failure")
                return real_receipt(*args, **kwargs)

            def fake_publication_status(directory, publication):
                nonlocal publication_cancel_sent
                path = real_publication_status(directory, publication)
                if cancel_during_publication_persist and not publication_cancel_sent:
                    publication_cancel_sent = True
                    adapter._cancel(adapter.signal.SIGINT, None)
                return path

            prior_cancel = adapter._CANCEL_SIGNAL
            adapter._CANCEL_SIGNAL = None
            try:
                with (
                    patch.object(adapter, "_open_hashed", side_effect=fake_open_hashed),
                    patch.object(adapter.tempfile, "mkdtemp", side_effect=temp_in_test_root),
                    patch.object(adapter, "_process", side_effect=fake_process) as process,
                    patch.object(adapter, "_write_new", side_effect=fake_write_new),
                    patch.object(adapter, "_validate_transform", side_effect=fake_validate),
                    patch.object(adapter, "_receipt", side_effect=fake_receipt),
                    patch.object(adapter, "_write_publication_status", side_effect=fake_publication_status),
                    patch.object(adapter.sys, "stdout", stdout),
                ):
                    exit_code = adapter._run(
                        SimpleNamespace(
                            command=[str(native)], cwd=tmp, pith_binary=str(pith_binary),
                            timeout_seconds=5,
                        )
                    )
                intended = output.intended if partial_output else output.getvalue()
                packet = json.loads(intended)
                receipt_path = packet["rawReference"].get("receipt")
                receipt = json.loads(Path(receipt_path).read_text()) if receipt_path else None
                if receipt is not None:
                    self.assertEqual(receipt["delivery"]["selectedEnvelopeBytes"], len(intended))
                    self.assertEqual(
                        receipt["delivery"]["selectedEnvelopeSha256"],
                        hashlib.sha256(intended).hexdigest(),
                    )
                raw = {
                    name: Path(path).read_bytes() if path else None
                    for name, path in packet["rawReference"].items()
                    if name in ("stdout", "stderr")
                }
                transform_raw = {
                    name: Path(packet["transformStatus"][f"{name}RawReference"]).read_bytes()
                    if packet["transformStatus"].get(f"{name}RawReference") else None
                    for name in ("stdout", "stderr")
                }
                publication_path = packet["rawReference"].get("publication")
                publication = (
                    json.loads(Path(publication_path).read_text())
                    if publication_path and Path(publication_path).exists()
                    else None
                )
                self.assertEqual(process.call_count, 1 + int(transform_result is not None and not cancel_after_command))
                for fd in opened_fds:
                    with self.assertRaises(OSError):
                        os.fstat(fd)
                return exit_code, packet, receipt, raw, transform_raw, output.getvalue(), publication
            finally:
                adapter._CANCEL_SIGNAL = prior_cancel

    def test_child_environment_does_not_forward_credentials_or_proxy_settings(self):
        with tempfile.TemporaryDirectory() as tmp, patch.dict(
            adapter.os.environ,
            {"PATH": "/usr/bin", "API_KEY": "secret", "HTTPS_PROXY": "http://proxy"},
            clear=True,
        ):
            env = adapter._command_environment(adapter.Path(tmp))
        self.assertEqual(set(env), {"PATH", "HOME", "TMPDIR", "LANG", "LC_ALL"})
        self.assertNotIn("API_KEY", env)
        self.assertNotIn("HTTPS_PROXY", env)

    def test_transform_request_uses_published_cli_fields_and_disables_telemetry(self):
        request = json.loads(adapter._transform_request("vitest run", b"one\n", 0))
        self.assertEqual(
            set(request),
            {"command", "output", "exitCode", "rawBypass", "telemetryEnabled"},
        )
        self.assertEqual(request["command"], "vitest run")
        self.assertEqual(request["output"], "one\n")
        self.assertEqual(request["exitCode"], 0)
        self.assertFalse(request["rawBypass"])
        self.assertFalse(request["telemetryEnabled"])

    def test_pith_binary_path_must_be_absolute(self):
        with self.assertRaises(adapter.CaptureError) as raised:
            adapter._run(SimpleNamespace(
                command=[], cwd="/tmp", pith_binary="pith", timeout_seconds=5
            ))
        self.assertEqual(raised.exception.args[0], "pith_path_must_be_absolute")

    def test_mocked_v370_vitest_contract_selection_keeps_raw_sentinel_out(self):
        """Validates a mocked response contract; this does not invoke Pith or Vitest."""
        raw = _vitest_raw()
        selected = _vitest_selected(raw)
        response = {
            "output": selected,
            "parser": "vitest",
            "passthrough": False,
            "originalByteCount": len(raw.encode("utf-8")),
            "retainedByteCount": len(selected.encode("utf-8")),
            "minimizationStrategy": "parser:vitest",
            "upstreamTruncated": False,
        }
        status = {
            "complete": True,
            "exitCode": 0,
            "signal": None,
            "stderr": b"",
            "stdout": json.dumps(response, separators=(",", ":")).encode("utf-8"),
        }
        output, parsed = adapter._validate_transform(raw.encode("utf-8"), status)
        self.assertEqual(output, selected)
        self.assertEqual(parsed["parser"], "vitest")
        self.assertFalse(parsed["passthrough"])
        self.assertNotIn("PITH_RAW_ONLY_SENTINEL", output)
        self.assertEqual(parsed["selectedSha256"], hashlib.sha256(output.encode()).hexdigest())

        altered = dict(response)
        altered["output"] = "altered mocked selection"
        status["stdout"] = json.dumps(altered, separators=(",", ":")).encode("utf-8")
        with self.assertRaises(adapter.CaptureError):
            adapter._validate_transform(raw.encode("utf-8"), status)

    def test_transform_validator_rejects_wrong_counts_and_empty_parser_claims(self):
        raw = b"vitest run\n"
        response = {
            "output": "short",
            "parser": "vitest",
            "passthrough": False,
            "originalByteCount": True,
            "retainedByteCount": 5,
            "minimizationStrategy": "parser:vitest",
            "upstreamTruncated": False,
        }
        status = {
            "complete": True,
            "exitCode": 0,
            "signal": None,
            "stderr": b"",
            "stdout": json.dumps(response).encode("utf-8"),
        }
        with self.assertRaises(adapter.CaptureError):
            adapter._validate_transform(raw, status)
        response.update(
            {
                "output": raw.decode(),
                "parser": "",
                "passthrough": True,
                "originalByteCount": len(raw),
                "retainedByteCount": len(raw),
                "minimizationStrategy": "passthrough",
            }
        )
        status["stdout"] = json.dumps(response).encode("utf-8")
        output, parsed = adapter._validate_transform(raw, status)
        self.assertEqual(output, raw.decode())
        self.assertTrue(parsed["passthrough"])

        response.update({
            "parser": "vitest",
            "passthrough": False,
            "output": "short",
            "retainedByteCount": len(b"short"),
            "minimizationStrategy": "parser:vitest",
            "upstreamTruncated": True,
        })
        status["stdout"] = json.dumps(response).encode("utf-8")
        with self.assertRaises(adapter.CaptureError):
            adapter._validate_transform(raw, status)

        response.update({
            "parser": "",
            "passthrough": True,
            "output": raw.decode(),
            "retainedByteCount": len(raw),
            "minimizationStrategy": "passthrough",
            "upstreamTruncated": True,
        })
        status["stdout"] = json.dumps(response).encode("utf-8")
        with self.assertRaises(adapter.CaptureError):
            adapter._validate_transform(raw, status)

    def test_delivery_comparison_uses_complete_packets_with_the_same_status_and_refs(self):
        raw = "line one\n" + "line two\n" * 100
        packet = {
            "selectedOutput": "line one\n... (+ 100 more lines)",
            "commandStatus": {"complete": True, "exitCode": 0, "settled": True},
            "transformStatus": {"complete": True, "parser": "vitest"},
            "error": None,
            "rawReference": {"stdout": "/tmp/private/stdout.raw", "stderr": "/tmp/private/stderr.raw"},
        }
        encoded, comparison = adapter._compare_envelopes(packet, raw, selection_succeeded=True)
        raw_packet = dict(packet)
        raw_packet["selectedOutput"] = raw
        self.assertTrue(encoded.endswith(b"\n"))
        self.assertEqual(comparison["selectedEnvelopeBytes"], len(encoded))
        self.assertEqual(comparison["rawReferenceEnvelopeBytes"], len(adapter._serialize(raw_packet)) + 1)
        self.assertTrue(comparison["smaller"])

    def test_null_selection_never_claims_reduction_and_counts_wire_newline(self):
        packet = {
            "selectedOutput": None,
            "commandStatus": {"attempted": True, "complete": True, "exitCode": 0},
            "transformStatus": {"attempted": True, "complete": False},
            "error": "pith_transform_failed",
            "rawReference": {"stdout": "/tmp/raw", "stderr": "/tmp/err", "receipt": None},
        }
        wire, comparison = adapter._compare_envelopes(packet, "raw output\n", selection_succeeded=False)
        self.assertEqual(comparison["selectedEnvelopeBytes"], len(wire))
        self.assertEqual(comparison["rawReferenceEnvelopeBytes"], len(adapter._serialize({**packet, "selectedOutput": "raw output\n"})) + 1)
        self.assertFalse(comparison["smaller"])

    def test_group_signal_requires_unreaped_session_leader_ownership(self):
        proc = SimpleNamespace(pid=45678, returncode=0)
        with patch.object(adapter.os, "killpg") as kill_group:
            self.assertIsNone(adapter._kill_owned_group(proc))
            kill_group.assert_not_called()
            proc.returncode = None
            self.assertIsNone(adapter._kill_owned_group(proc))
            kill_group.assert_called_once_with(proc.pid, adapter.signal.SIGKILL)

    def test_group_signal_and_probe_oserrors_become_unknown_status(self):
        proc = SimpleNamespace(pid=45679, returncode=None)
        with patch.object(adapter.os, "killpg", side_effect=OSError("mock denied")):
            self.assertEqual(adapter._kill_owned_group(proc), "process_group_signal_failed")
        proc.returncode = 9
        with patch.object(adapter.os, "killpg", side_effect=OSError("mock probe failure")):
            settled, group_gone, error = adapter._settlement(proc, True)
        self.assertFalse(settled)
        self.assertIsNone(group_gone)
        self.assertEqual(error, "process_group_probe_failed")

    def test_cancel_latched_before_process_launch_prevents_popen(self):
        prior_cancel = adapter._CANCEL_SIGNAL
        adapter._CANCEL_SIGNAL = adapter.signal.SIGINT
        try:
            with patch.object(adapter.subprocess, "Popen") as launch:
                result = adapter._process(
                    ["/tmp/mock-native"], cwd=Path("/tmp"), env={}, timeout=1,
                    out_limit=1024, err_limit=1024,
                )
            launch.assert_not_called()
            self.assertFalse(result["launchAttempted"])
            self.assertTrue(result["cancelled"])
            self.assertEqual(result["processError"], "cancelled_before_launch")
        finally:
            adapter._CANCEL_SIGNAL = prior_cancel

    def test_process_cleanup_oserrors_preserve_partial_capture(self):
        class Stream:
            def __init__(self, fd):
                self.fd = fd

            def fileno(self):
                return self.fd

            def close(self):
                return None

        stdout_stream, stderr_stream = Stream(101), Stream(102)

        class FakeProcess:
            pid = 45680
            stdin = None
            stdout = stdout_stream
            stderr = stderr_stream
            returncode = None

            def poll(self):
                return self.returncode

            def wait(self, timeout=None):
                self.returncode = -9
                return self.returncode

        process = FakeProcess()
        keys = [
            SimpleNamespace(data="stdout", fd=stdout_stream.fd, fileobj=stdout_stream),
            SimpleNamespace(data="stderr", fd=stderr_stream.fd, fileobj=stderr_stream),
        ]

        class FakeSelector:
            def __init__(self):
                self.read_index = 0

            def register(self, *_args):
                return None

            def get_map(self):
                return {key.fd: key for key in keys}

            def select(self, _timeout):
                key = keys[min(self.read_index, len(keys) - 1)]
                self.read_index += 1
                return [(key, adapter.selectors.EVENT_READ)]

            def close(self):
                return None

        prior_cancel = adapter._CANCEL_SIGNAL
        adapter._CANCEL_SIGNAL = None
        try:
            with (
                patch.object(adapter.subprocess, "Popen", return_value=process),
                patch.object(adapter.selectors, "DefaultSelector", return_value=FakeSelector()),
                patch.object(adapter.os, "set_blocking"),
                patch.object(
                    adapter.os, "read",
                    side_effect=[b"partial stdout", b"partial stderr", OSError("mock pipe error")],
                ) as pipe_read,
                patch.object(adapter.os, "killpg", side_effect=OSError("mock cleanup error")),
            ):
                result = adapter._process(
                    ["/tmp/mock-native"], cwd=Path("/tmp"), env={}, timeout=5,
                    out_limit=1024, err_limit=1024,
                )
        finally:
            adapter._CANCEL_SIGNAL = prior_cancel
        self.assertEqual(pipe_read.call_count, 3)
        self.assertEqual(result["stdout"], b"partial stdout")
        self.assertEqual(result["stderr"], b"partial stderr")
        self.assertEqual(result["signal"], 9)
        self.assertTrue(result["launchAttempted"])
        self.assertFalse(result["complete"])
        self.assertFalse(result["settled"])
        self.assertIsNone(result["sameProcessGroupGone"])
        self.assertIn("process_group_signal_failed", result["cleanupErrors"])
        self.assertIn("process_group_probe_failed", result["cleanupErrors"])

    def test_native_executable_contract_rejects_relative_paths_and_shebang_files(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            native = root / "native"
            native.write_bytes(b"\x7fELFmock")
            native.chmod(0o700)
            fd = os.open(native, os.O_RDONLY)
            try:
                adapter._require_native_executable(native, fd)
                with self.assertRaises(adapter.CaptureError):
                    adapter._require_native_executable(Path("relative-native"), fd)
            finally:
                os.close(fd)
            script = root / "launcher"
            script.write_bytes(b"#!/bin/sh\necho no\n")
            script.chmod(0o700)
            fd = os.open(script, os.O_RDONLY)
            try:
                with self.assertRaises(adapter.CaptureError):
                    adapter._require_native_executable(script, fd)
            finally:
                os.close(fd)

    def test_command_failure_receipt_keeps_partial_native_capture(self):
        native_result = {
            "stdout": b"partial stdout\n", "stderr": b"partial stderr\n",
            "exitCode": 7, "signal": None, "settled": False, "complete": False,
            "timedOut": True, "cancelled": False, "outputLimited": False,
            "streamEof": False, "sameProcessGroupGone": False,
            "processError": "capture_timeout", "attempted": True,
        }
        exit_code, packet, receipt, raw, _transform_raw, _wire, _publication = self._run_mocked(native_result)
        self.assertEqual(exit_code, 7)
        self.assertEqual(packet["commandStatus"]["exitCode"], 7)
        self.assertEqual(packet["commandStatus"]["processError"], "capture_timeout")
        self.assertFalse(packet["transformStatus"]["attempted"])
        self.assertEqual(raw["stdout"], native_result["stdout"])
        self.assertEqual(raw["stderr"], native_result["stderr"])
        self.assertEqual(receipt["commandOutput"]["stdoutSha256"], hashlib.sha256(native_result["stdout"]).hexdigest())
        self.assertEqual(receipt["commandStatus"]["exitCode"], 7)

    def test_failed_pith_attempt_keeps_phase_evidence_and_never_claims_smaller(self):
        native_result = {
            "stdout": b"complete native output\n", "stderr": b"", "exitCode": 0,
            "signal": None, "settled": True, "complete": True, "timedOut": False,
            "cancelled": False, "outputLimited": False, "streamEof": True,
            "sameProcessGroupGone": True, "processError": None, "attempted": True,
        }
        pith_result = {
            "stdout": b"partial pith response", "stderr": b"transform diagnostic\n",
            "exitCode": 2, "signal": None, "settled": False, "complete": False,
            "timedOut": False, "cancelled": False, "outputLimited": False,
            "streamEof": False, "sameProcessGroupGone": False,
            "processError": None, "attempted": True,
        }
        _exit_code, packet, receipt, raw, transform_raw, _wire, _publication = self._run_mocked(native_result, pith_result)
        self.assertEqual(packet["commandStatus"]["exitCode"], 0)
        self.assertTrue(packet["transformStatus"]["attempted"])
        self.assertEqual(packet["transformStatus"]["exitCode"], 2)
        self.assertIsNone(packet["selectedOutput"])
        self.assertFalse(packet["deliveryComparison"]["smaller"])
        self.assertEqual(raw["stdout"], native_result["stdout"])
        self.assertEqual(transform_raw["stdout"], pith_result["stdout"])
        self.assertEqual(transform_raw["stderr"], pith_result["stderr"])
        self.assertEqual(receipt["delivery"]["selectedEnvelopeBytes"], len(json.dumps(packet, ensure_ascii=False, separators=(",", ":")).encode()) + 1)

    def test_cancellation_latch_between_phases_blocks_transform_launch(self):
        native_result = {
            "stdout": b"completed before cancellation\n", "stderr": b"", "exitCode": 0,
            "signal": None, "settled": True, "complete": True, "timedOut": False,
            "cancelled": False, "outputLimited": False, "streamEof": True,
            "sameProcessGroupGone": True, "processError": None, "attempted": True,
        }
        _exit_code, packet, _receipt, raw, _transform_raw, _wire, _publication = self._run_mocked(native_result, cancel_after_command=True)
        self.assertEqual(packet["adapterCancelSignal"], adapter.signal.SIGTERM)
        self.assertEqual(packet["commandStatus"]["exitCode"], 0)
        self.assertFalse(packet["transformStatus"]["attempted"])
        self.assertIsNone(packet["selectedOutput"])
        self.assertEqual(raw["stdout"], native_result["stdout"])

    def test_cancel_during_transform_evidence_persistence_invalidates_selection(self):
        native_result = {
            "stdout": b"complete native output\n", "stderr": b"", "exitCode": 0,
            "signal": None, "settled": True, "complete": True, "timedOut": False,
            "cancelled": False, "outputLimited": False, "streamEof": True,
            "sameProcessGroupGone": True, "processError": None, "attempted": True,
        }
        pith_result = _successful_transform(native_result["stdout"])
        _code, packet, _receipt, _raw, _pith_raw, _wire, _publication = self._run_mocked(
            native_result, pith_result, cancel_during_transform_persist=True
        )
        self.assertEqual(packet["adapterCancelSignal"], adapter.signal.SIGINT)
        self.assertIsNone(packet["selectedOutput"])
        self.assertFalse(packet["deliveryComparison"]["smaller"])

    def test_cancel_during_selection_validation_invalidates_selection(self):
        native_result = {
            "stdout": b"complete native output\n", "stderr": b"", "exitCode": 0,
            "signal": None, "settled": True, "complete": True, "timedOut": False,
            "cancelled": False, "outputLimited": False, "streamEof": True,
            "sameProcessGroupGone": True, "processError": None, "attempted": True,
        }
        pith_result = _successful_transform(native_result["stdout"])
        _code, packet, _receipt, _raw, _pith_raw, _wire, _publication = self._run_mocked(
            native_result, pith_result, cancel_during_validation=True
        )
        self.assertEqual(packet["adapterCancelSignal"], adapter.signal.SIGTERM)
        self.assertIsNone(packet["selectedOutput"])
        self.assertFalse(packet["deliveryComparison"]["smaller"])

    def test_receipt_failure_suppresses_selected_output_and_reduction(self):
        native_result = {
            "stdout": b"complete native output\n", "stderr": b"", "exitCode": 0,
            "signal": None, "settled": True, "complete": True, "timedOut": False,
            "cancelled": False, "outputLimited": False, "streamEof": True,
            "sameProcessGroupGone": True, "processError": None, "attempted": True,
        }
        pith_result = _successful_transform(native_result["stdout"])
        _code, packet, receipt, _raw, _pith_raw, _wire, _publication = self._run_mocked(
            native_result, pith_result, receipt_failure=True
        )
        self.assertIsNone(receipt)
        self.assertEqual(packet["error"], "receipt_write_failed")
        self.assertIsNone(packet["selectedOutput"])
        self.assertIsNone(packet["rawReference"]["receipt"])
        self.assertFalse(packet["deliveryComparison"]["smaller"])
        self.assertTrue(packet["commandStatus"]["attempted"])
        self.assertTrue(packet["transformStatus"]["attempted"])

    def test_partial_stdout_write_never_appends_fallback_and_status_sidecar_marks_uncertain(self):
        native_result = {
            "stdout": b"complete native output\n", "stderr": b"", "exitCode": 0,
            "signal": None, "settled": True, "complete": True, "timedOut": False,
            "cancelled": False, "outputLimited": False, "streamEof": True,
            "sameProcessGroupGone": True, "processError": None, "attempted": True,
        }
        pith_result = _successful_transform(native_result["stdout"])
        code, packet, receipt, _raw, _pith_raw, published, publication = self._run_mocked(
            native_result, pith_result, partial_output=True
        )
        self.assertEqual(code, 125)
        self.assertEqual(len(published), 7)
        self.assertEqual(receipt["publicationStatus"]["state"], "pending")
        self.assertTrue(receipt["publicationStatus"]["deliveryUncertain"])
        self.assertEqual(publication["state"], "uncertain")
        self.assertTrue(publication["deliveryUncertain"])
        self.assertEqual(publication["bytesWritten"], 7)
        self.assertTrue(packet["commandStatus"]["attempted"])
        self.assertTrue(packet["transformStatus"]["attempted"])

    def test_cancel_during_publication_status_persist_marks_finalization_and_returns_125(self):
        native_result = {
            "stdout": b"complete native output\n", "stderr": b"", "exitCode": 0,
            "signal": None, "settled": True, "complete": True, "timedOut": False,
            "cancelled": False, "outputLimited": False, "streamEof": True,
            "sameProcessGroupGone": True, "processError": None, "attempted": True,
        }
        pith_result = _successful_transform(native_result["stdout"])
        code, packet, _receipt, _raw, _pith_raw, published, publication = self._run_mocked(
            native_result, pith_result, cancel_during_publication_persist=True
        )
        self.assertEqual(code, 125)
        self.assertTrue(publication["cancelledDuringFinalization"])
        self.assertEqual(publication["adapterCancelSignal"], adapter.signal.SIGINT)
        self.assertEqual(publication["state"], "complete")
        self.assertFalse(publication["deliveryUncertain"])
        self.assertTrue(published.endswith(b"\n"))
        self.assertEqual(published.count(b"\n"), 1)
        self.assertIsNotNone(packet["selectedOutput"])

    def test_duplicate_json_keys_are_rejected(self):
        with self.assertRaises(adapter.CaptureError):
            adapter._json(b'{"status":0,"status":1}', "invalid")

    def test_transform_failure_does_not_echo_raw_command_output(self):
        raw_secret = "PRIVATE_RAW_SENTINEL"
        command = {"stdout": raw_secret.encode(), "stderr": b"", "exitCode": 0}
        from pathlib import Path

        packet = adapter._packet(None, command, {"attempted": True, "complete": False},
                                 Path("/tmp/private"), "pith_transform_failed")
        encoded = adapter._encode_packet(packet)
        self.assertNotIn(raw_secret.encode(), encoded)
        self.assertIn(b"stdout.raw", encoded)


if __name__ == "__main__":
    unittest.main()
