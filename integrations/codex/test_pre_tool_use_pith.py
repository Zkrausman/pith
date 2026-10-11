import importlib.util
import json
import os
import signal
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock


SCRIPT = Path(__file__).with_name("pre_tool_use_pith.py")
SPEC = importlib.util.spec_from_file_location("pre_tool_use_pith", SCRIPT)
assert SPEC and SPEC.loader
pith_adapter = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(pith_adapter)

VALID = (
    b"=== RUN   TestAlpha\n"
    b"--- PASS: TestAlpha (0.00s)\n"
    b"=== RUN   TestBeta\n"
    b"--- PASS: TestBeta (0.00s)\n"
    b"PASS\n"
    b"ok pith/pkg/parser 0.001s\n"
)
SELECTED = b"PASS\nok pith/pkg/parser 0.001s"


def fake_executable(path: Path, body: str) -> Path:
    path.write_text(f"#!{sys.executable}\n" + body, encoding="utf-8")
    path.chmod(0o700)
    return path


class AdapterTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory(prefix="pith-fake-")
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.go = fake_executable(
            self.root / "go",
            """
import os, signal, sys, time
mode = os.environ.get('FAKE_GO_MODE', 'valid')
if mode == 'valid':
    os.write(1, %r)
elif mode == 'diagnostic':
    os.write(1, %r + b'diagnostic line\\n')
elif mode == 'invalid_utf8':
    os.write(1, b'\\xff')
elif mode == 'overflow':
    os.write(1, b'x' * int(os.environ['FAKE_GO_BYTES']))
elif mode == 'prefix_overflow':
    os.write(1, b'prefix')
    os.write(1, b'x' * int(os.environ['FAKE_GO_BYTES']))
elif mode == 'silent':
    time.sleep(float(os.environ.get('FAKE_GO_SLEEP', '0.12')))
    os.write(1, %r)
elif mode == 'eof_before_exit':
    os.write(1, %r)
    os.close(1)
    time.sleep(float(os.environ.get('FAKE_GO_SLEEP', '0.12')))
elif mode == 'signal':
    os.write(1, %r)
    os.kill(os.getpid(), signal.SIGTERM)
sys.exit(int(os.environ.get('FAKE_GO_STATUS', '0')))
""" % (VALID, VALID, VALID, VALID, VALID),
        )
        self.pith = fake_executable(
            self.root / "pith",
            """
import json, os, signal, sys, time
request = json.load(sys.stdin)
log = os.environ.get('FAKE_PITH_LOG')
if log:
    with open(log, 'w', encoding='utf-8') as f:
        json.dump(request, f)
mode = os.environ.get('FAKE_PITH_MODE', 'success')
if mode == 'sleep':
    time.sleep(5)
elif mode == 'signal':
    os.kill(os.getpid(), signal.SIGTERM)
elif mode == 'malformed':
    sys.stdout.write('{')
elif mode == 'huge_output':
    sys.stdout.write('x' * (2 * 1024 * 1024))
else:
    selected = 'PASS\\nok pith/pkg/parser 0.001s'
    if mode == 'diagnostic':
        selected = 'PASS\\nok pith/pkg/parser 0.001s\\nwarning: hidden'
    elif mode == 'wrong_parser':
        pass
    elif mode == 'same':
        selected = request['output']
    print(json.dumps({'output': selected, 'parser': 'tests' if mode != 'wrong_parser' else 'go', 'passthrough': False}))
""",
        )
        self.log = self.root / "request.json"

    def environ(self, **values):
        env = os.environ.copy()
        env["FAKE_PITH_LOG"] = str(self.log)
        for key, value in values.items():
            env[key] = str(value)
        return env

    def run_fake(self, **kwargs):
        chunks = bytearray()
        calls = []

        def writer(_fd, data):
            calls.append(bytes(data))
            chunks.extend(data)
            return True

        env = os.environ.copy()
        env["FAKE_PITH_LOG"] = str(self.log)
        with mock.patch.dict(os.environ, env, clear=True):
            status = pith_adapter.run_native(
                [str(self.go), "test", "-v", "./pkg/parser"],
                str(self.pith),
                stdout_fd=123,
                writer=writer,
                **kwargs,
            )
        return status, bytes(chunks), calls

    def test_hook_rewrites_only_supported_literal_command(self):
        payload = {"tool_name": "Bash", "tool_input": {"command": f'"{self.go}" test -v ./pkg/parser'}}
        result = pith_adapter.rewrite_hook_input(payload, str(self.pith))
        specific = result["hookSpecificOutput"]
        self.assertEqual(specific["hookEventName"], "PreToolUse")
        self.assertEqual(specific["permissionDecision"], "allow")
        rewritten = specific["updatedInput"]["command"]
        self.assertIn("--run", rewritten)
        self.assertIn(shlex_quote(str(self.go)), rewritten)

    def test_unknown_and_unsupported_hook_inputs_allow_without_rewrite(self):
        for payload in (
            None,
            {},
            {"tool_name": "MCP"},
            {"tool_name": "Bash", "tool_input": {"command": "go test -v ./pkg/parser"}},
            {"tool_name": "Bash", "tool_input": {"command": f"{self.go} test -v ./pkg/parser && id"}},
            {"tool_name": "Bash", "tool_input": {"command": f"X=1 {self.go} test -v ./pkg/parser"}},
            {"tool_name": "Bash", "tool_input": {"command": f"{self.go} test -run TestX -v ./pkg/parser"}},
            {"tool_name": "Bash", "tool_input": {"command": f"{self.go} test -v $(id)"}},
            {"tool_name": "Bash", "tool_input": {"command": f"{self.go} test -v ./..."}},
        ):
            with self.subTest(payload=payload):
                specific = pith_adapter.rewrite_hook_input(payload, str(self.pith))["hookSpecificOutput"]
                self.assertNotIn("updatedInput", specific)

    def test_command_quoting_is_shell_safe_for_path_with_spaces(self):
        spaced = self.root / "with space"
        spaced.mkdir()
        go = fake_executable(spaced / "go", "import os\nos.write(1, b'x')\n")
        command = f"'{go}' test -v ./pkg/parser"
        argv = pith_adapter._absolute_go_test(command)
        self.assertEqual(argv, [str(go), "test", "-v", "./pkg/parser"])

    def test_normal_success_uses_transform_contract_and_native_status(self):
        self.assertEqual(pith_adapter._valid_go_verbose_success(VALID), ("PASS", "ok pith/pkg/parser 0.001s"))
        status, output, _ = self.run_fake()
        self.assertEqual(status, 0)
        self.assertEqual(output, SELECTED)
        request = json.loads(self.log.read_text(encoding="utf-8"))
        self.assertEqual(request["exitCode"], 0)
        self.assertFalse(request["rawBypass"])
        self.assertFalse(request["telemetryEnabled"])
        self.assertNotIn("enabledParsers", request)

    def test_capture_limit_boundary_and_overflow_switch_to_raw_once(self):
        status, output, _ = self.run_fake(capture_limit=len(VALID))
        self.assertEqual((status, output), (0, SELECTED))
        self.log.unlink(missing_ok=True)
        status, output, calls = self.run_fake(capture_limit=len(VALID) - 1)
        self.assertEqual((status, output), (0, VALID))
        self.assertFalse(self.log.exists())
        self.assertEqual(b"".join(calls), VALID)

    def test_default_one_mebibyte_cap_transitions_to_raw(self):
        data = b"x" * (pith_adapter.CAPTURE_LIMIT + 1)
        env = self.environ(FAKE_GO_MODE="overflow", FAKE_GO_BYTES=len(data))
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, _ = self.run_fake()
        self.assertEqual((status, output), (0, data))
        self.assertFalse(self.log.exists())

    def test_overflow_emits_prefix_and_every_later_chunk_exactly_once(self):
        env = self.environ(FAKE_GO_MODE="prefix_overflow", FAKE_GO_BYTES=100)
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, calls = self.run_fake(capture_limit=6)
        self.assertEqual(status, 0)
        self.assertEqual(output, b"prefix" + b"x" * 100)
        self.assertEqual(b"".join(calls), output)
        self.assertFalse(self.log.exists())

    def test_silent_stdout_expiry_uses_monotonic_deadline(self):
        env = self.environ(FAKE_GO_MODE="silent", FAKE_GO_SLEEP="0.12")
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, _ = self.run_fake(buffer_seconds=0.03)
        self.assertEqual((status, output), (0, VALID))
        self.assertFalse(self.log.exists())

    def test_deadline_still_fires_after_stdout_eof_before_process_exit(self):
        env = self.environ(FAKE_GO_MODE="eof_before_exit", FAKE_GO_SLEEP="0.12")
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, _ = self.run_fake(buffer_seconds=0.03)
        self.assertEqual((status, output), (0, VALID))
        self.assertFalse(self.log.exists())

    def test_native_failure_and_native_signal_return_raw_bytes(self):
        env = self.environ(FAKE_GO_STATUS=7)
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, _ = self.run_fake()
        self.assertEqual((status, output), (7, VALID))
        env = self.environ(FAKE_GO_MODE="signal")
        with mock.patch.dict(os.environ, env, clear=True), mock.patch.object(pith_adapter, "_reraise") as reraise:
            status, output, _ = self.run_fake()
        reraise.assert_called_once_with(signal.SIGTERM)
        self.assertEqual((status, output), (-signal.SIGTERM, VALID))

    def test_diagnostics_invalid_utf8_and_bad_transform_fall_back_to_raw(self):
        env = self.environ(FAKE_GO_MODE="diagnostic")
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, _ = self.run_fake()
        self.assertEqual((status, output), (0, VALID + b"diagnostic line\n"))
        self.assertTrue(self.log.exists())
        self.log.unlink(missing_ok=True)
        env = self.environ(FAKE_GO_MODE="invalid_utf8")
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, _ = self.run_fake()
        self.assertEqual((status, output), (0, b"\xff"))
        self.assertFalse(self.log.exists())
        for mode in ("malformed", "huge_output", "wrong_parser", "diagnostic", "same", "signal"):
            self.log.unlink(missing_ok=True)
            env = self.environ(FAKE_PITH_MODE=mode)
            with mock.patch.dict(os.environ, env, clear=True):
                status, output, _ = self.run_fake()
            self.assertEqual((status, output), (0, VALID), mode)

    def test_transform_timeout_falls_back_without_replaying_native(self):
        env = self.environ(FAKE_PITH_MODE="sleep")
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, _ = self.run_fake(buffer_seconds=0.05)
        self.assertEqual((status, output), (0, VALID))
        self.assertTrue(self.log.exists())

    def test_tty_and_descriptor_errors_exec_native_before_spawn(self):
        class ExecCalled(Exception):
            pass

        tty_patcher = mock.patch.object(pith_adapter.os, "isatty", side_effect=[False, True])
        descriptor_error = mock.patch.object(pith_adapter, "_fd_usable", side_effect=OSError("bad fd"))
        for patcher in (tty_patcher, descriptor_error):
            with patcher, mock.patch.object(pith_adapter.os, "execve", side_effect=ExecCalled) as execve:
                with self.assertRaises(ExecCalled):
                    pith_adapter.run_native([str(self.go), "test", "-v", "./pkg/parser"], str(self.pith))
                execve.assert_called_once_with(
                    str(self.go), [str(self.go), "test", "-v", "./pkg/parser"], os.environ
                )
        self.assertFalse(self.log.exists())

    def test_write_all_retries_eintr_and_partial_writes(self):
        result = bytearray()
        calls = 0

        def partial(_fd, view):
            nonlocal calls
            calls += 1
            if calls == 1:
                raise InterruptedError()
            take = min(2, len(view))
            result.extend(view[:take])
            return take

        self.assertTrue(pith_adapter._write_all(1, b"abcdefgh", write=partial))
        self.assertEqual(bytes(result), b"abcdefgh")

    def test_limits_are_fixed_at_reviewed_bounds(self):
        self.assertEqual(pith_adapter.CAPTURE_LIMIT, 1 << 20)
        self.assertEqual(pith_adapter.READ_CHUNK, 32 << 10)
        self.assertEqual(pith_adapter.BUFFER_SECONDS, 2.0)

    def test_hook_input_is_bounded_and_children_do_not_create_groups(self):
        import io

        oversized = b" " * (pith_adapter.HOOK_INPUT_LIMIT + 1)
        old_stdin, old_stdout = pith_adapter.sys.stdin, pith_adapter.sys.stdout
        try:
            pith_adapter.sys.stdin = io.TextIOWrapper(io.BytesIO(oversized))
            pith_adapter.sys.stdout = io.StringIO()
            self.assertEqual(pith_adapter.main(["--pith-binary", str(self.pith)]), 0)
            response = json.loads(pith_adapter.sys.stdout.getvalue())
            self.assertNotIn("updatedInput", response["hookSpecificOutput"])
        finally:
            pith_adapter.sys.stdin, pith_adapter.sys.stdout = old_stdin, old_stdout
        source = SCRIPT.read_text(encoding="utf-8")
        self.assertNotIn("start_new_session", source)
        self.assertNotIn("os.setsid", source)
        self.assertNotIn("os.killpg", source)


def shlex_quote(value):
    import shlex

    return shlex.quote(value)


if __name__ == "__main__":
    unittest.main()
