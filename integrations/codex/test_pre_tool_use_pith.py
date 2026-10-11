import hashlib
import importlib.util
import json
import os
import signal
import sys
import tempfile
import time
from pathlib import Path
from unittest import TestCase, mock


SCRIPT = Path(__file__).with_name("pre_tool_use_pith.py").resolve()
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
HEAD = "a" * 40
TREE = "b" * 40


def _sha(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _fake(path: Path, body: str) -> Path:
    path.write_text(f"#!{sys.executable}\n" + body, encoding="utf-8")
    path.chmod(0o700)
    return path.resolve()


class AdapterTests(TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="pith-profile-fakes-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.go_counter = self.root / "go-invocations.txt"
        self.git = _fake(
            self.root / "git",
            """
import os, sys
args = sys.argv[1:]
if 'rev-parse' in args:
    print(os.environ['FAKE_GIT_ROOT'])
    print(os.environ['FAKE_GIT_HEAD'])
    print(os.environ['FAKE_GIT_TREE'])
elif 'status' in args:
    if os.environ.get('FAKE_GIT_DIRTY_BYTES'):
        for _ in range(int(os.environ['FAKE_GIT_DIRTY_BYTES']) // 4096):
            sys.stdout.buffer.write(b'x' * 4096)
    else:
        sys.stdout.write(os.environ.get('FAKE_GIT_DIRTY', ''))
else:
    raise SystemExit(2)
""",
        )
        self.go = _fake(
            self.root / "go",
            """
import json, os, signal, sys, time
counter = os.environ.get('FAKE_GO_COUNTER')
if counter:
    with open(counter, 'a', encoding='ascii') as f:
        f.write('1\\n')
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
elif mode == 'context':
    value = os.read(0, 128)
    record = {'cwd': os.getcwd(), 'context': os.environ.get('FAKE_CONTEXT'), 'stdin': value.decode()}
    with open(os.environ['FAKE_GO_CONTEXT_FILE'], 'w', encoding='utf-8') as f:
        json.dump(record, f)
    os.write(2, b'native stderr fixture\\n')
    os.write(1, %r)
sys.exit(int(os.environ.get('FAKE_GO_STATUS', '0')))
""" % (VALID, VALID, VALID, VALID, VALID, VALID),
        )
        self.pith = _fake(
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
        selected += '\\nwarning: hidden'
    elif mode == 'same':
        selected = request['output']
    parser = 'go' if mode == 'wrong_parser' else 'tests'
    print(json.dumps({'output': selected, 'parser': parser, 'passthrough': False}))
""",
        )
        self.pith_log = self.root / "pith-request.json"
        self.context_file = self.root / "inherited-context.json"
        self.profile_path = self.root / "codex-profile.json"
        self.profile = self._write_profile()

    def _write_profile(self, **overrides):
        cwd = os.path.realpath(os.getcwd())
        profile = {
            "schemaVersion": 1,
            "cwd": cwd,
            "repoRoot": cwd,
            "sourceHead": HEAD,
            "sourceTree": TREE,
            "package": "./pkg/parser",
            "go": {"path": str(self.go), "sha256": _sha(self.go)},
            "pith": {"path": str(self.pith), "sha256": _sha(self.pith)},
            "git": {"path": str(self.git), "sha256": _sha(self.git)},
            "python": {"path": os.path.realpath(sys.executable), "sha256": _sha(Path(sys.executable).resolve())},
            "adapter": {"path": str(SCRIPT), "sha256": _sha(SCRIPT)},
        }
        profile.update(overrides)
        self.profile_path.write_text(json.dumps(profile, sort_keys=True, separators=(",", ":")), encoding="utf-8")
        self.profile_path.chmod(0o600)
        self.profile = profile
        self.profile_sha256 = _sha(self.profile_path)
        return profile

    def _env(self, **values):
        env = os.environ.copy()
        env.update({
            "FAKE_GIT_ROOT": self.profile["repoRoot"],
            "FAKE_GIT_HEAD": HEAD,
            "FAKE_GIT_TREE": TREE,
            "FAKE_GO_COUNTER": str(self.go_counter),
            "FAKE_PITH_LOG": str(self.pith_log),
        })
        env.update({key: str(value) for key, value in values.items()})
        return env

    def _invocations(self):
        return len(self.go_counter.read_text(encoding="ascii").splitlines()) if self.go_counter.exists() else 0

    def _run(self, *, writer_result=True, **kwargs):
        packets = bytearray()
        writes = []
        warnings = []

        def writer(_fd, data):
            writes.append(bytes(data))
            if writer_result:
                packets.extend(data)
            return writer_result

        env = self._env()
        with mock.patch.dict(os.environ, env, clear=True):
            status = pith_adapter.run_native(
                [str(self.go), "test", "-v", "./pkg/parser"],
                str(self.profile_path), self.profile_sha256,
                stdout_fd=123, writer=writer,
                warning_writer=lambda message, native: warnings.append((message, native)),
                **kwargs,
            )
        return status, bytes(packets), writes, warnings

    def test_profile_binds_executable_package_adapter_pith_and_cwd(self):
        payload = {
            "tool_name": "Bash",
            "tool_input": {
                "command": f"'{self.go}' test -v ./pkg/parser",
                "tty": False,
                "yield_time_ms": 1234,
                "timeout_ms": 9000,
                "workdir": "/ignored/by-hook",
            },
        }
        result = pith_adapter.rewrite_hook_input(payload, str(self.profile_path), self.profile_sha256)
        specific = result["hookSpecificOutput"]
        self.assertEqual(specific["hookEventName"], "PreToolUse")
        self.assertEqual(
            set(specific["updatedInput"]),
            {"command", "tty", "yield_time_ms", "timeout_ms", "workdir"},
        )
        self.assertIn("--profile-sha256", specific["updatedInput"]["command"])
        for option in ("tty", "yield_time_ms", "timeout_ms", "workdir"):
            self.assertEqual(specific["updatedInput"][option], payload["tool_input"][option])

        for command in (
            f"/usr/bin/go test -v ./pkg/parser",
            f"{self.go} test -v ./pkg/other",
            f"{self.go} test -v ./...",
            f"{self.go} test -run TestX -v ./pkg/parser",
            f"{self.go} test -v ./pkg/parser && id",
        ):
            with self.subTest(command=command):
                out = pith_adapter.rewrite_hook_input(
                    {"tool_name": "Bash", "tool_input": {"command": command}},
                    str(self.profile_path), self.profile_sha256,
                )
                self.assertNotIn("updatedInput", out["hookSpecificOutput"])

        # Profile identities are independently pinned; changing either target
        # digest restores the original command path without any invocation.
        for key in ("pith", "adapter"):
            with self.subTest(identity=key):
                identity = dict(self.profile[key])
                identity["sha256"] = "0" * 64
                self._write_profile(**{key: identity})
                out = pith_adapter.rewrite_hook_input(
                    {"tool_name": "Bash", "tool_input": {"command": f"{self.go} test -v ./pkg/parser"}},
                    str(self.profile_path), self.profile_sha256,
                )
                self.assertNotIn("updatedInput", out["hookSpecificOutput"])
        self._write_profile()

        os.chmod(self.profile_path, 0o640)
        out = pith_adapter.rewrite_hook_input(
            {"tool_name": "Bash", "tool_input": {"command": f"{self.go} test -v ./pkg/parser"}},
            str(self.profile_path), self.profile_sha256,
        )
        self.assertNotIn("updatedInput", out["hookSpecificOutput"])
        self._write_profile()

        self._write_profile(cwd=str(self.root / "different-cwd"), repoRoot=str(self.root / "different-cwd"))
        class ExecCalled(Exception):
            pass
        with mock.patch.object(pith_adapter, "_exec_original", side_effect=ExecCalled):
            with self.assertRaises(ExecCalled):
                pith_adapter.run_native(
                    [str(self.go), "test", "-v", "./pkg/parser"],
                    str(self.profile_path), self.profile_sha256,
                )
        self.assertEqual(self._invocations(), 0)

    def test_source_head_or_dirty_tree_mismatch_execs_original(self):
        self._write_profile(sourceHead="c" * 40)
        class ExecCalled(Exception):
            pass
        env = self._env()
        with mock.patch.dict(os.environ, env, clear=True), mock.patch.object(pith_adapter, "_exec_original", side_effect=ExecCalled):
            with self.assertRaises(ExecCalled):
                pith_adapter.run_native(
                    [str(self.go), "test", "-v", "./pkg/parser"],
                    str(self.profile_path), self.profile_sha256,
                )
        self.assertEqual(self._invocations(), 0)

        self._write_profile()
        env = self._env(FAKE_GIT_DIRTY=" M pkg/parser/example.go\n")
        with mock.patch.dict(os.environ, env, clear=True), mock.patch.object(pith_adapter, "_exec_original", side_effect=ExecCalled):
            with self.assertRaises(ExecCalled):
                pith_adapter.run_native(
                    [str(self.go), "test", "-v", "./pkg/parser"],
                    str(self.profile_path), self.profile_sha256,
                )
        self.assertEqual(self._invocations(), 0)

        env = self._env(FAKE_GIT_DIRTY_BYTES=10_000_000)
        with mock.patch.dict(os.environ, env, clear=True), mock.patch.object(pith_adapter, "_exec_original", side_effect=ExecCalled):
            with self.assertRaises(ExecCalled):
                pith_adapter.run_native(
                    [str(self.go), "test", "-v", "./pkg/parser"],
                    str(self.profile_path), self.profile_sha256,
                )
        self.assertEqual(self._invocations(), 0)

    def test_success_uses_transform_contract_once(self):
        status, output, _, warnings = self._run()
        self.assertEqual((status, output), (0, SELECTED))
        self.assertEqual(warnings, [])
        self.assertEqual(self._invocations(), 1)
        request = json.loads(self.pith_log.read_text(encoding="utf-8"))
        self.assertEqual(request["command"], f"{self.go} test -v ./pkg/parser")
        self.assertEqual(request["exitCode"], 0)
        self.assertFalse(request["rawBypass"])
        self.assertFalse(request["telemetryEnabled"])
        self.assertNotIn("enabledParsers", request)

    def test_inherited_cwd_environment_stdin_and_native_stderr(self):
        read_fd, write_fd = os.pipe()
        os.write(write_fd, b"stdin fixture\n")
        os.close(write_fd)
        saved_stdin = os.dup(0)
        stderr_read, stderr_write = os.pipe()
        saved_stderr = os.dup(2)
        old_cwd = os.getcwd()
        try:
            os.dup2(read_fd, 0)
            os.close(read_fd)
            os.dup2(stderr_write, 2)
            os.close(stderr_write)
            os.chdir(self.root)
            self.profile["cwd"] = str(self.root)
            self.profile["repoRoot"] = str(self.root)
            self._write_profile()
            env = self._env(FAKE_GO_MODE="context", FAKE_CONTEXT="kept", FAKE_GO_CONTEXT_FILE=self.context_file)
            with mock.patch.dict(os.environ, env, clear=True):
                status, output, _, _ = self._run()
        finally:
            os.chdir(old_cwd)
            os.dup2(saved_stdin, 0)
            os.close(saved_stdin)
            os.dup2(saved_stderr, 2)
            os.close(saved_stderr)
        stderr = os.read(stderr_read, 4096)
        os.close(stderr_read)
        self.assertEqual((status, output), (0, SELECTED))
        self.assertEqual(stderr, b"native stderr fixture\n")
        context = json.loads(self.context_file.read_text(encoding="utf-8"))
        self.assertEqual(context, {"cwd": str(self.root), "context": "kept", "stdin": "stdin fixture\n"})

    def test_tty_and_descriptor_errors_exec_original_before_spawn(self):
        class ExecCalled(Exception):
            pass
        for patcher in (
            mock.patch.object(pith_adapter.os, "isatty", side_effect=[False, True]),
            mock.patch.object(pith_adapter.os, "fstat", side_effect=OSError("bad descriptor")),
        ):
            with patcher, mock.patch.object(pith_adapter, "_exec_original", side_effect=ExecCalled):
                with self.assertRaises(ExecCalled):
                    pith_adapter.run_native(
                        [str(self.go), "test", "-v", "./pkg/parser"],
                        str(self.profile_path), self.profile_sha256,
                    )
        self.assertEqual(self._invocations(), 0)

    def test_cancellation_pending_at_exec_gate_never_starts_go(self):
        real_write_gate = pith_adapter._write_gate
        original_kill = os.kill

        def cancel_both_before_release(fd, child_pid):
            # The adapter and gated child both receive the process-group signal
            # while it is blocked; the helper must check pending signals before exec.
            original_kill(os.getpid(), signal.SIGTERM)
            original_kill(child_pid, signal.SIGTERM)
            real_write_gate(fd, child_pid)

        env = self._env()
        with (
            mock.patch.dict(os.environ, env, clear=True),
            mock.patch.object(pith_adapter, "_write_gate", side_effect=cancel_both_before_release),
            mock.patch.object(pith_adapter, "_reraise") as reraise,
        ):
            status, output, _, _ = self._run()
        reraise.assert_called_once_with(signal.SIGTERM)
        self.assertEqual((status, output), (125, b""))
        self.assertEqual(self._invocations(), 0)

    def test_silent_deadline_and_eof_before_exit_switch_to_raw(self):
        expected_invocations = 0
        for mode in ("silent", "eof_before_exit"):
            self.pith_log.unlink(missing_ok=True)
            env = self._env(FAKE_GO_MODE=mode, FAKE_GO_SLEEP="0.12")
            with mock.patch.dict(os.environ, env, clear=True):
                status, output, _, _ = self._run(buffer_seconds=0.03)
            self.assertEqual((status, output), (0, VALID), mode)
            self.assertFalse(self.pith_log.exists())
            expected_invocations += 1
            self.assertEqual(self._invocations(), expected_invocations)

    def test_pipe_preparation_and_native_read_failure_are_explicit(self):
        class ExecCalled(Exception):
            pass
        env = self._env()
        with (
            mock.patch.dict(os.environ, env, clear=True),
            mock.patch.object(pith_adapter, "_new_pipe_poller", side_effect=OSError("poll setup failed")),
            mock.patch.object(pith_adapter, "_exec_original", side_effect=ExecCalled),
        ):
            with self.assertRaises(ExecCalled):
                pith_adapter.run_native(
                    [str(self.go), "test", "-v", "./pkg/parser"],
                    str(self.profile_path), self.profile_sha256,
                )
        self.assertEqual(self._invocations(), 0)

        real_poller = pith_adapter._new_pipe_poller
        native_read_fds = set()

        def remember_poller(*fds):
            if len(fds) == 1:
                native_read_fds.add(fds[0])
            return real_poller(*fds)

        real_read = os.read
        fail_once = True

        def fail_native_read(fd, size):
            nonlocal fail_once
            if fd in native_read_fds and fail_once:
                fail_once = False
                raise OSError(5, "fixture read failure")
            return real_read(fd, size)

        env = self._env()
        with (
            mock.patch.dict(os.environ, env, clear=True),
            mock.patch.object(pith_adapter, "_new_pipe_poller", side_effect=remember_poller),
            mock.patch.object(pith_adapter.os, "read", side_effect=fail_native_read),
        ):
            status, output, _, warnings = self._run()
        self.assertEqual((status, output), (0, VALID))
        self.assertEqual(warnings, [])
        self.assertEqual(self._invocations(), 1)
        self.assertFalse(self.pith_log.exists())

    def test_capture_limit_boundary_overflow_and_prefix_relay_once(self):
        status, output, writes, _ = self._run(capture_limit=len(VALID))
        self.assertEqual((status, output), (0, SELECTED))
        self.assertEqual(self._invocations(), 1)

        self.pith_log.unlink(missing_ok=True)
        env = self._env(FAKE_GO_MODE="prefix_overflow", FAKE_GO_BYTES=100)
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, writes, _ = self._run(capture_limit=6)
        self.assertEqual((status, output), (0, b"prefix" + b"x" * 100))
        self.assertEqual(b"".join(writes), output)
        self.assertFalse(self.pith_log.exists())
        self.assertEqual(self._invocations(), 2)

        data = b"x" * (pith_adapter.CAPTURE_LIMIT + 1)
        env = self._env(FAKE_GO_MODE="overflow", FAKE_GO_BYTES=len(data))
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, _, _ = self._run()
        self.assertEqual((status, output), (0, data))
        self.assertFalse(self.pith_log.exists())
        self.assertEqual(self._invocations(), 3)

    def test_native_failure_signal_and_transform_timeout_preserve_single_run(self):
        env = self._env(FAKE_GO_STATUS=7)
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, _, _ = self._run()
        self.assertEqual((status, output), (7, VALID))
        self.assertEqual(self._invocations(), 1)

        env = self._env(FAKE_GO_MODE="signal")
        with mock.patch.dict(os.environ, env, clear=True), mock.patch.object(pith_adapter, "_reraise") as reraise:
            status, output, _, _ = self._run()
        reraise.assert_called_once_with(signal.SIGTERM)
        self.assertEqual((status, output), (-signal.SIGTERM, VALID))
        self.assertEqual(self._invocations(), 2)

        env = self._env(FAKE_PITH_MODE="sleep")
        start = time.monotonic()
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, _, _ = self._run(buffer_seconds=0.08)
        elapsed = time.monotonic() - start
        self.assertEqual((status, output), (0, VALID))
        self.assertLess(elapsed, 0.5)
        self.assertEqual(self._invocations(), 3)

    def test_transform_malformed_fidelity_invalid_utf8_and_surrogate_fallback(self):
        expected_invocations = 0
        for mode in ("malformed", "huge_output", "wrong_parser", "diagnostic", "same", "signal"):
            self.pith_log.unlink(missing_ok=True)
            env = self._env(FAKE_PITH_MODE=mode)
            with mock.patch.dict(os.environ, env, clear=True):
                status, output, _, _ = self._run()
            self.assertEqual((status, output), (0, VALID), mode)
            expected_invocations += 1
            self.assertEqual(self._invocations(), expected_invocations)

        self.pith_log.unlink(missing_ok=True)
        env = self._env(FAKE_GO_MODE="diagnostic")
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, _, _ = self._run()
        self.assertEqual((status, output), (0, VALID + b"diagnostic line\n"))
        expected_invocations += 1
        self.assertEqual(self._invocations(), expected_invocations)
        self.assertFalse(self.pith_log.exists())

        env = self._env(FAKE_GO_MODE="invalid_utf8")
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, _, _ = self._run()
        self.assertEqual((status, output), (0, b"\xff"))
        expected_invocations += 1
        self.assertEqual(self._invocations(), expected_invocations)
        self.assertFalse(self.pith_log.exists())

        response = json.dumps({"output": "\ud800", "parser": "tests", "passthrough": False}).encode()
        self.assertIsNone(pith_adapter._selected_output(VALID, response))

    def test_failed_publication_warns_truthfully_without_retry_or_status_change(self):
        status, output, writes, warnings = self._run(writer_result=False)
        self.assertEqual(status, 0)
        self.assertEqual(output, b"")
        self.assertEqual(len(writes), 1)
        self.assertEqual(warnings, [("stdout publication incomplete", 0)])
        self.assertEqual(self._invocations(), 1)

    def test_partial_writes_eintr_limits_and_static_no_group_forwarding(self):
        result = bytearray()
        calls = 0

        def partial(_fd, view):
            nonlocal calls
            calls += 1
            if calls == 1:
                raise InterruptedError()
            amount = min(2, len(view))
            result.extend(view[:amount])
            return amount

        self.assertTrue(pith_adapter._write_all(1, b"abcdefgh", write=partial))
        self.assertEqual(bytes(result), b"abcdefgh")
        self.assertEqual(pith_adapter.CAPTURE_LIMIT, 1 << 20)
        self.assertEqual(pith_adapter.READ_CHUNK, 32 << 10)
        self.assertEqual(pith_adapter.BUFFER_SECONDS, 2.0)
        source = SCRIPT.read_text(encoding="utf-8")
        self.assertNotIn("start_new_session", source)
        self.assertNotIn("os.setsid", source)
        self.assertNotIn("os.killpg", source)


if __name__ == "__main__":
    import unittest

    unittest.main()
