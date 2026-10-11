import importlib.util
import io
import json
import os
import signal
import shlex
import sys
import subprocess
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
def _fake(path: Path, body: str) -> Path:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(f"#!{sys.executable}\n" + body, encoding="utf-8")
    path.chmod(0o700)
    return path.resolve()


class AdapterTests(TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="pith-adapter-fakes-")
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.go_counter = self.root / "go-invocations.txt"
        self.go = _fake(
            self.root / "go suite" / "go",
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
            self.root / "pith transform",
            """
import json, os, signal, sys, time
request = json.load(sys.stdin)
started = os.environ.get('FAKE_PITH_STARTED_FILE')
if started:
    with open(started, 'w', encoding='ascii') as f:
        f.write('started')
log = os.environ.get('FAKE_PITH_LOG')
if log:
    with open(log, 'w', encoding='utf-8') as f:
        json.dump(request, f)
mode = os.environ.get('FAKE_PITH_MODE', 'success')
delay = float(os.environ.get('FAKE_PITH_DELAY', '0'))
if delay:
    time.sleep(delay)
if mode == 'sleep':
    time.sleep(5)
elif mode == 'signal':
    os.kill(os.getpid(), signal.SIGTERM)
elif mode == 'malformed':
    sys.stdout.write('{')
elif mode == 'huge_output':
    sys.stdout.write('x' * (2 * 1024 * 1024))
elif mode == 'deep_response':
    sys.stdout.write('[' * 2000 + '0' + ']' * 2000)
else:
    selected = 'PASS\\nok pith/pkg/parser 0.001s'
    if mode == 'diagnostic':
        selected += '\\nwarning: hidden'
    elif mode == 'same':
        selected = request['output']
    parser = 'go' if mode == 'wrong_parser' else 'tests'
    print(json.dumps({'output': selected, 'parser': parser, 'passthrough': False}))
sys.exit(1 if mode == 'status_failure' else int(os.environ.get('FAKE_PITH_STATUS', '0')))
""",
        )
        self.pith_log = self.root / "pith-request.json"
        self.pith_started = self.root / "pith-started.txt"
        self.context_file = self.root / "inherited-context.json"
        self.pith_binary = str(self.pith)

    def _env(self, **values):
        env = os.environ.copy()
        env.update({
            "FAKE_GO_COUNTER": str(self.go_counter),
            "FAKE_PITH_LOG": str(self.pith_log),
            "FAKE_PITH_STARTED_FILE": str(self.pith_started),
        })
        env.update({key: str(value) for key, value in values.items()})
        return env

    def _invocations(self):
        return len(self.go_counter.read_text(encoding="ascii").splitlines()) if self.go_counter.exists() else 0

    def _run(self, *, writer_result=True, record_outcome=None, record_observation=None, **kwargs):
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
                self.pith_binary,
                stdout_fd=123, writer=writer,
                warning_writer=lambda message, native: warnings.append((message, native)),
                record_outcome=record_outcome or (lambda *_args: None),
                record_observation=record_observation or (lambda *_args: None),
                **kwargs,
            )
        return status, bytes(packets), writes, warnings

    def _call_hook(self, raw: bytes):
        output = io.StringIO()
        fake_stdin = mock.Mock(buffer=io.BytesIO(raw))
        with mock.patch.object(sys, "stdin", fake_stdin), mock.patch.object(sys, "stdout", output):
            status = pith_adapter.main(["--pith-binary", self.pith_binary])
        return status, output.getvalue()

    def test_exact_command_shape_preserves_options_and_paths_with_spaces(self):
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
        result = pith_adapter.rewrite_hook_input(payload, self.pith_binary)
        specific = result["hookSpecificOutput"]
        self.assertEqual(specific["hookEventName"], "PreToolUse")
        self.assertEqual(
            set(specific["updatedInput"]),
            {"command", "tty", "yield_time_ms", "timeout_ms", "workdir"},
        )
        wrapper = shlex.split(specific["updatedInput"]["command"])
        self.assertEqual(wrapper[:4], [os.path.realpath(sys.executable), "-I", "-S", str(SCRIPT)])
        self.assertIn("--pith-binary", wrapper)
        self.assertEqual(wrapper[wrapper.index("--pith-binary") + 1], self.pith_binary)
        self.assertIn(str(self.pith), wrapper)
        self.assertIn(str(self.go), wrapper)
        for option in ("tty", "yield_time_ms", "timeout_ms", "workdir"):
            self.assertEqual(specific["updatedInput"][option], payload["tool_input"][option])

        for command in (
            f"{self.go} test -v ./...",
            f"{self.go} test -run TestX -v ./pkg/parser",
            f"{self.go} test -v ./pkg/parser && id",
            f"go test -v ./pkg/parser",
            f"{self.go} test -v -run TestAlpha ./pkg/parser",
            f"{self.go} test -v ./pkg/parser extra",
            f"{self.go} test -v '-flag with space'",
            f"{self.go} test -v 'unterminated",
        ):
            with self.subTest(command=command):
                out = pith_adapter.rewrite_hook_input(
                    {"tool_name": "Bash", "tool_input": {"command": command}},
                    self.pith_binary,
                )
                self.assertNotIn("updatedInput", out["hookSpecificOutput"])

        for command in (
            "/usr/bin/go test -v ./pkg/parser",
            shlex.join([str(self.go), "test", "-v", "./pkg/other"]),
        ):
            with self.subTest(unpinned_native=command):
                out = pith_adapter.rewrite_hook_input(
                    {"tool_name": "Bash", "tool_input": {"command": command}}, self.pith_binary,
                )
                self.assertIn("updatedInput", out["hookSpecificOutput"])


    def test_malformed_unsupported_oversized_and_deep_hooks_bypass(self):
        deep = b'{"x":' + b'[' * 1500 + b'0' + b']' * 1500 + b'}'
        for raw in (
            b"not json",
            json.dumps({"tool_name": "Other", "tool_input": {"command": "anything"}}).encode(),
            deep,
            b"x" * (pith_adapter.HOOK_INPUT_LIMIT + 1),
        ):
            with self.subTest(size=len(raw)):
                status, text = self._call_hook(raw)
                self.assertEqual(status, 0)
                self.assertEqual(json.loads(text), pith_adapter._allow())

    def test_proposal_families_are_fixed_and_eligible_go_test_is_not_double_counted(self):
        cases = {
            "/private/sentinel/bin/git status --token=secret-sentinel": "git status",
            "go test -run TestX ./pkg/parser": "go test",
            "npm run build": "npm run",
            "python3 -m pytest -q": "pytest",
            "node app.js": "node",
            "git obscure-subcommand": pith_adapter._FAMILY_OTHER,
            "custom-tool --password=sentinel": pith_adapter._FAMILY_OTHER,
            "git status && cat /private/sentinel/file": pith_adapter._FAMILY_OTHER,
            "unterminated ' quote": pith_adapter._FAMILY_OTHER,
        }
        for command, expected in cases.items():
            with self.subTest(command=command):
                actual = pith_adapter._proposal_family(command)
                self.assertEqual(actual, expected)
                self.assertNotIn("sentinel", actual)

        eligible_command = shlex.join([str(self.go), "test", "-v", "./pkg/parser"])
        payload = {"tool_name": "Bash", "tool_input": {"command": eligible_command}}
        proposals = []
        wrapped = pith_adapter.rewrite_hook_input(
            payload, self.pith_binary, record_proposal=lambda binary, family: proposals.append((binary, family)),
        )
        self.assertIn("updatedInput", wrapped["hookSpecificOutput"])
        self.assertEqual(proposals, [], "eligible wrapped Go tests must not add a proposal row")

        unsupported = {"tool_name": "Bash", "tool_input": {"command": "git status --short"}}
        allowed = pith_adapter.rewrite_hook_input(
            unsupported, self.pith_binary, record_proposal=lambda binary, family: proposals.append((binary, family)),
        )
        self.assertEqual(allowed, pith_adapter._allow())
        self.assertEqual(proposals, [(self.pith_binary, "git status")])

    def test_private_observation_requests_contain_only_allowlisted_labels_and_nullable_counts(self):
        submitted = []
        with mock.patch.object(pith_adapter, "_record_private_observation", side_effect=lambda binary, request, **_kwargs: submitted.append((binary, request))):
            pith_adapter._record_codex_proposal(self.pith_binary, "git status")
            pith_adapter._record_codex_proposal(self.pith_binary, "/private/sentinel/command --token=secret")
            pith_adapter._record_capture_observation(
                self.pith_binary, "complete_within_window", 0, "raw", 0, 0,
            )
            pith_adapter._record_capture_observation(
                self.pith_binary, "incomplete", None, "unknown", None, None,
            )
        self.assertEqual(len(submitted), 3)
        self.assertEqual(submitted[0][1], {"kind": "proposal", "family": "git status"})
        self.assertEqual(submitted[1][1], {
            "kind": "capture", "captureOutcome": "complete_within_window", "elapsedMs": 0,
            "selection": "raw", "originalBytes": 0, "deliveredBytes": 0,
        })
        self.assertEqual(submitted[2][1], {
            "kind": "capture", "captureOutcome": "incomplete", "elapsedMs": None,
            "selection": "unknown", "originalBytes": None, "deliveredBytes": None,
        })
        serialized = json.dumps(submitted)
        for sentinel in ("/private/sentinel", "secret", "command", "session", "model", "price", "output"):
            self.assertNotIn(sentinel, serialized)

    def test_private_observation_recorder_has_bounded_payload_and_best_effort_failure(self):
        process = mock.Mock()
        process.communicate.return_value = b""
        request = {"kind": "proposal", "family": "other/compound"}
        with mock.patch.object(pith_adapter.subprocess, "Popen", return_value=process) as popen:
            pith_adapter._record_private_observation(self.pith_binary, request, timeout=0.02)
        popen.assert_called_once()
        args, kwargs = popen.call_args
        self.assertEqual(args[0], [self.pith_binary, "_record-codex-wrapper"])
        self.assertEqual(json.loads(process.communicate.call_args.args[0]), request)
        self.assertIs(kwargs["stdout"], subprocess.DEVNULL)
        self.assertIs(kwargs["stderr"], subprocess.DEVNULL)
        process.kill.assert_not_called()

        with mock.patch.object(pith_adapter.subprocess, "Popen", side_effect=OSError("fixture recorder failure")):
            pith_adapter._record_private_observation(self.pith_binary, request)

    def test_pretool_entry_records_family_only_for_unsupported_proposals(self):
        submitted = []
        def call(command):
            payload = json.dumps({"tool_name": "Bash", "tool_input": {"command": command}}).encode()
            output = io.StringIO()
            fake_stdin = mock.Mock(buffer=io.BytesIO(payload))
            with (
                mock.patch.object(sys, "stdin", fake_stdin),
                mock.patch.object(sys, "stdout", output),
                mock.patch.object(pith_adapter, "_record_private_observation", side_effect=lambda binary, request, **_kwargs: submitted.append(request)),
            ):
                status = pith_adapter.main(["--pith-binary", self.pith_binary])
            return status, json.loads(output.getvalue())

        status, allowed = call("/private/sentinel/git status --token=secret-sentinel")
        self.assertEqual(status, 0)
        self.assertEqual(allowed, pith_adapter._allow())
        self.assertEqual(submitted, [{"kind": "proposal", "family": "git status"}])
        eligible_command = shlex.join([str(self.go), "test", "-v", "./pkg/parser"])
        status, rewritten = call(eligible_command)
        self.assertEqual(status, 0)
        self.assertIn("updatedInput", rewritten["hookSpecificOutput"])
        self.assertEqual(len(submitted), 1, "eligible Go test should not create a proposal observation")
        self.assertNotIn("sentinel", json.dumps(submitted))

    def test_hook_response_encoder_recursion_falls_back_to_allow(self):
        raw = json.dumps({
            "tool_name": "Bash",
            "tool_input": {"command": f"{self.go} test -v ./pkg/parser"},
        }).encode()
        real_dumps = json.dumps
        calls = 0

        def fail_first_dump(*args, **kwargs):
            nonlocal calls
            calls += 1
            if calls == 1:
                raise RecursionError("fixture recursion")
            return real_dumps(*args, **kwargs)

        with mock.patch.object(pith_adapter.json, "dumps", side_effect=fail_first_dump):
            status, text = self._call_hook(raw)
        self.assertEqual(status, 0)
        self.assertEqual(json.loads(text), pith_adapter._allow())
        self.assertEqual(calls, 2)

    def test_success_uses_transform_contract_once(self):
        status, output, _, warnings = self._run()
        self.assertEqual((status, output), (0, SELECTED))
        self.assertEqual(warnings, [])
        self.assertEqual(self._invocations(), 1)
        request = json.loads(self.pith_log.read_text(encoding="utf-8"))
        self.assertEqual(request["command"], shlex.join([str(self.go), "test", "-v", "./pkg/parser"]))
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
                        self.pith_binary,
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
        records = []
        for mode in ("silent", "eof_before_exit"):
            self.pith_log.unlink(missing_ok=True)
            env = self._env(FAKE_GO_MODE=mode, FAKE_GO_SLEEP="0.12")
            with mock.patch.dict(os.environ, env, clear=True):
                status, output, _, _ = self._run(
                    capture_seconds=0.03,
                    record_observation=lambda *args: records.append(args),
                )
            self.assertEqual((status, output), (0, VALID), mode)
            self.assertFalse(self.pith_log.exists())
            self.assertEqual(records[-1][1], "deadline_stream")
            self.assertEqual(records[-1][3:], ("raw", len(VALID), len(VALID)))
            expected_invocations += 1
            self.assertEqual(self._invocations(), expected_invocations)

    def test_capture_deadline_rechecked_when_exit_is_observed_after_eof(self):
        clock = [0.0]
        spawn_calls = []

        class FakeNativeProcess:
            def __init__(self):
                self.poll_calls = 0
                self.returncode = None
                self.killed = False

            def poll(self):
                self.poll_calls += 1
                if self.poll_calls >= 2:
                    clock[0] = 0.04
                    self.returncode = 0
                return self.returncode

            def kill(self):
                self.killed = True

        process = FakeNativeProcess()

        def spawn_gated(argv, *, stdin, stdout, stderr, launch_deadline=None):
            spawn_calls.append(list(argv))
            if len(spawn_calls) == 1:
                os.write(stdout, VALID)
                return process, True, False, False
            return None, False, False, False

        packets = bytearray()
        writes = []

        def writer(_fd, data):
            value = bytes(data)
            writes.append(value)
            packets.extend(value)
            return True

        with (
            mock.patch.object(pith_adapter.time, "monotonic", side_effect=lambda: clock[0]),
            mock.patch.object(pith_adapter, "_spawn_gated", side_effect=spawn_gated),
        ):
            status = pith_adapter.run_native(
                [str(self.go), "test", "-v", "./pkg/parser"], self.pith_binary,
                capture_seconds=0.03, transform_seconds=0.5,
                stdout_fd=123, writer=writer, record_outcome=lambda *_args: None,
                record_observation=lambda *_args: None,
            )

        self.assertEqual(status, 0)
        self.assertEqual(bytes(packets), VALID)
        self.assertEqual(writes, [VALID])
        self.assertEqual(len(spawn_calls), 1, "Pith must not launch after selection expiry")
        self.assertFalse(process.killed, "selection expiry must not kill the native command")

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
                    self.pith_binary,
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
        self.assertFalse(fail_once)
        self.assertEqual(self._invocations(), 1)
        self.assertFalse(self.pith_log.exists())

        real_poller = pith_adapter._new_pipe_poller
        native_read_fds = set()

        def remember_poller_for_persistent(*fds):
            if len(fds) == 1:
                native_read_fds.add(fds[0])
            return real_poller(*fds)

        persistent_errors = 3

        def fail_persistent_reads(fd, size):
            nonlocal persistent_errors
            if fd in native_read_fds and persistent_errors:
                persistent_errors -= 1
                raise OSError(5, "persistent fixture read failure")
            return real_read(fd, size)

        class SilentNativeProcess:
            def poll(self):
                return 0 if persistent_errors == 0 else None

        process = SilentNativeProcess()

        def spawn_silent_native(argv, *, stdin, stdout, stderr, launch_deadline=None):
            return process, True, False, False

        records = []
        with (
            mock.patch.object(pith_adapter, "_new_pipe_poller", side_effect=remember_poller_for_persistent),
            mock.patch.object(pith_adapter.os, "read", side_effect=fail_persistent_reads),
            mock.patch.object(pith_adapter, "_spawn_gated", side_effect=spawn_silent_native),
        ):
            status, output, _, warnings = self._run(record_observation=lambda *args: records.append(args))
        self.assertEqual((status, output), (0, b""))
        self.assertEqual(persistent_errors, 0)
        self.assertEqual(warnings, [("stdout capture read failed", 0)])
        self.assertEqual(records[0][1], "incomplete")
        self.assertEqual(records[0][3:], ("unknown", None, None))
        self.assertEqual(self._invocations(), 1)
        self.assertFalse(self.pith_log.exists())

    def test_preparation_failure_after_latched_cancel_never_execs_native(self):
        def cancel_then_fail(_fd):
            os.kill(os.getpid(), signal.SIGINT)
            raise OSError("fixture selector preparation failure")

        with (
            mock.patch.object(pith_adapter, "_new_pipe_poller", side_effect=cancel_then_fail),
            mock.patch.object(pith_adapter, "_reraise") as reraise,
            mock.patch.object(pith_adapter, "_exec_original", side_effect=AssertionError("native fallback after cancel")),
        ):
            status, output, _, _ = self._run()
        reraise.assert_called_once_with(signal.SIGINT)
        self.assertEqual(status, 128 + signal.SIGINT)
        self.assertEqual(output, b"")
        self.assertEqual(self._invocations(), 0)

    def test_capture_limit_boundary_overflow_and_prefix_relay_once(self):
        status, output, writes, _ = self._run(capture_limit=len(VALID))
        self.assertEqual((status, output), (0, SELECTED))
        self.assertEqual(self._invocations(), 1)

        self.pith_log.unlink(missing_ok=True)
        env = self._env(FAKE_GO_MODE="prefix_overflow", FAKE_GO_BYTES=100)
        records = []
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, writes, _ = self._run(
                capture_limit=6, record_observation=lambda *args: records.append(args),
            )
        self.assertEqual((status, output), (0, b"prefix" + b"x" * 100))
        self.assertEqual(b"".join(writes), output)
        self.assertEqual(records[0][1], "cap_stream")
        self.assertEqual(records[0][3:], ("raw", len(output), len(output)))
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
        self.pith_started.unlink(missing_ok=True)
        start = time.monotonic()
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, _, _ = self._run(capture_seconds=5.0, transform_seconds=0.5)
        elapsed = time.monotonic() - start
        self.assertEqual((status, output), (0, VALID))
        self.assertLess(elapsed, 0.5 + 0.05 + 0.25)
        self.assertEqual(self.pith_started.read_text(encoding="ascii"), "started")
        self.assertEqual(self._invocations(), 3)

    def test_transform_gets_its_own_window_after_native_capture(self):
        clock = [0.0]
        spawn_calls = []

        class FakeNativeProcess:
            def __init__(self):
                self.poll_calls = 0
                self.returncode = None

            def poll(self):
                self.poll_calls += 1
                if self.poll_calls == 2:
                    clock[0] = 3.0
                    self.returncode = 0
                return self.returncode

            def kill(self):
                raise AssertionError("native command must not be killed")

        process = FakeNativeProcess()

        def spawn_native(argv, *, stdin, stdout, stderr, launch_deadline=None):
            spawn_calls.append(list(argv))
            os.write(stdout, VALID)
            return process, True, False, False

        def transform(_pith_binary, _command, _output, deadline, **_kwargs):
            self.assertEqual(clock[0], 3.0, "native capture should finish after the old shared window")
            self.assertEqual(deadline, 3.5, "transform receives a fresh window after native completion")
            clock[0] = 3.4
            return json.dumps({"output": "PASS\nok pith/pkg/parser 0.001s", "parser": "tests", "passthrough": False}).encode()

        packets = bytearray()

        def writer(_fd, data):
            packets.extend(data)
            return True

        with (
            mock.patch.object(pith_adapter.time, "monotonic", side_effect=lambda: clock[0]),
            mock.patch.object(pith_adapter, "_spawn_gated", side_effect=spawn_native),
            mock.patch.object(pith_adapter, "_run_transform", side_effect=transform),
        ):
            status = pith_adapter.run_native(
                [str(self.go), "test", "-v", "./pkg/parser"], self.pith_binary,
                capture_seconds=30.0, transform_seconds=0.5,
                stdout_fd=123, writer=writer, record_outcome=lambda *_args: None,
                record_observation=lambda *_args: None,
            )

        self.assertEqual((status, bytes(packets)), (0, SELECTED))
        self.assertEqual(spawn_calls, [[str(self.go), "test", "-v", "./pkg/parser"]])

    def test_deadline_after_transform_preparation_publishes_raw_without_pith(self):
        real_poller = pith_adapter._new_pipe_poller
        transform_prep_started = []

        def delay_transform_preparation(*fds):
            if len(fds) == 2:
                transform_prep_started.append(time.monotonic())
                time.sleep(0.6)
            return real_poller(*fds)

        self.pith_started.unlink(missing_ok=True)
        env = self._env(FAKE_GO_MODE="silent", FAKE_GO_SLEEP="0.1")
        with (
            mock.patch.dict(os.environ, env, clear=True),
            mock.patch.object(pith_adapter, "_new_pipe_poller", side_effect=delay_transform_preparation),
        ):
            run_started = time.monotonic()
            status, output, _, _ = self._run(capture_seconds=1.0, transform_seconds=0.5)
        self.assertEqual((status, output), (0, VALID))
        self.assertTrue(transform_prep_started)
        self.assertGreater(transform_prep_started[0] - run_started, 0.08)
        self.assertFalse(self.pith_started.exists())
        self.assertFalse(self.pith_log.exists())
        self.assertEqual(self._invocations(), 1)

    def test_deadline_is_rechecked_at_gate_after_helper_spawn(self):
        real_popen = subprocess.Popen
        deadline = time.monotonic() + 0.5

        def start_helper_then_cross_deadline(*args, **kwargs):
            proc = real_popen(*args, **kwargs)
            self.assertLess(time.monotonic(), deadline, "helper must start before the deadline")
            time.sleep(0.55)
            return proc

        self.pith_started.unlink(missing_ok=True)
        env = self._env()
        with (
            mock.patch.dict(os.environ, env, clear=True),
            mock.patch.object(pith_adapter.subprocess, "Popen", side_effect=start_helper_then_cross_deadline),
        ):
            result = pith_adapter._run_transform(
                self.pith_binary, shlex.join([str(self.go), "test", "-v", "./pkg/parser"]),
                VALID, deadline,
            )
        self.assertIsNone(result)
        self.assertFalse(self.pith_started.exists())
        self.assertFalse(self.pith_log.exists())

    def test_unresolved_transform_reap_is_not_reported_as_settled(self):
        real_stop_reap = pith_adapter._stop_reap

        def reap_but_report_unknown(proc, *, grace_seconds=0.05):
            real_stop_reap(proc, grace_seconds=grace_seconds)
            return False

        self.pith_started.unlink(missing_ok=True)
        env = self._env(FAKE_PITH_MODE="sleep")
        with (
            mock.patch.dict(os.environ, env, clear=True),
            mock.patch.object(pith_adapter, "_stop_reap", side_effect=reap_but_report_unknown),
        ):
            status, output, _, warnings = self._run(capture_seconds=5.0, transform_seconds=0.5)
        self.assertEqual((status, output), (0, VALID))
        self.assertEqual(self.pith_started.read_text(encoding="ascii"), "started")
        self.assertEqual(len(warnings), 1)
        self.assertIn("reap unsettled", warnings[0][0])
        self.assertEqual(warnings[0][1], 0)

    def test_transform_malformed_fidelity_invalid_utf8_and_surrogate_fallback(self):
        expected_invocations = 0
        for mode in (
            "malformed", "huge_output", "deep_response", "wrong_parser", "diagnostic", "same", "signal",
            "status_failure",
        ):
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
        with mock.patch.object(
            pith_adapter, "_encode_selected_utf8", wraps=pith_adapter._encode_selected_utf8,
        ) as encode_selected:
            self.assertIsNone(pith_adapter._selected_output(VALID, response))
        encode_selected.assert_called_once_with("\ud800")
        self.assertIsNone(pith_adapter._encode_selected_utf8("\ud800"))

    def test_failed_publication_warns_truthfully_without_retry_or_status_change(self):
        records = []
        status, output, writes, warnings = self._run(
            writer_result=False, record_observation=lambda *args: records.append(args),
        )
        self.assertEqual(status, 0)
        self.assertEqual(output, b"")
        self.assertEqual(len(writes), 1)
        self.assertEqual(warnings, [("stdout publication incomplete", 0)])
        self.assertEqual(records[0][1], "incomplete")
        self.assertEqual(records[0][3:], ("unknown", None, None))
        self.assertEqual(self._invocations(), 1)

    def test_capture_observation_keeps_existing_gain_telemetry(self):
        records = []
        legacy_records = []
        status, output, _, _ = self._run(
            record_outcome=lambda *args: legacy_records.append(args),
            record_observation=lambda *args: records.append(args),
        )
        self.assertEqual((status, output), (0, SELECTED))
        self.assertEqual(len(records), 1)
        self.assertEqual(len(legacy_records), 1, "existing gain telemetry must remain intact")
        self.assertEqual(legacy_records[0][1:3], (VALID, SELECTED))
        self.assertEqual(legacy_records[0][-1], "transformed")
        binary, outcome, duration_ms, selection, original_bytes, delivered_bytes = records[0]
        self.assertEqual(binary, self.pith_binary)
        self.assertEqual((outcome, selection), ("complete_within_window", "reduced"))
        self.assertEqual((original_bytes, delivered_bytes), (len(VALID), len(SELECTED)))
        self.assertGreaterEqual(duration_ms, 0)

        records.clear()
        env = self._env(FAKE_PITH_MODE="wrong_parser")
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, _, _ = self._run(record_observation=lambda *args: records.append(args))
        self.assertEqual((status, output), (0, VALID))
        self.assertEqual(len(records), 1)
        self.assertEqual(records[0][1], "complete_within_window")
        self.assertEqual(records[0][3], "raw")
        self.assertEqual(records[0][4:], (len(VALID), len(VALID)))

        records.clear()
        env = self._env(FAKE_GO_STATUS="7")
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, _, _ = self._run(record_observation=lambda *args: records.append(args))
        self.assertEqual((status, output), (7, VALID))
        self.assertEqual(len(records), 1)
        self.assertEqual(records[0][1], "complete_within_window")
        self.assertEqual(records[0][3], "raw")
        self.assertEqual(records[0][4:], (len(VALID), len(VALID)))

    def test_recorder_failure_does_not_change_published_native_result(self):
        def fail_recorder(*_args):
            raise OSError("fixture telemetry failure")

        status, output, _, _ = self._run(record_observation=fail_recorder)
        self.assertEqual((status, output), (0, SELECTED))

    def test_signal_arriving_during_recorder_is_reraised_after_publication(self):
        def signal_recorder(*_args):
            os.kill(os.getpid(), signal.SIGTERM)

        with mock.patch.object(pith_adapter, "_reraise") as reraise:
            status, output, writes, _ = self._run(record_observation=signal_recorder)
        self.assertEqual((status, output), (0, SELECTED))
        self.assertEqual(writes, [SELECTED])
        reraise.assert_called_once_with(signal.SIGTERM)

        with mock.patch.dict(os.environ, {"FAKE_GO_STATUS": "7"}), mock.patch.object(
            pith_adapter, "_reraise",
        ) as reraise:
            status, output, writes, _ = self._run(record_observation=signal_recorder)
        self.assertEqual((status, output), (7, VALID))
        self.assertEqual(writes, [VALID])
        reraise.assert_called_once_with(signal.SIGTERM)

    def test_complete_invalid_utf8_and_deadline_stream_still_have_bounded_observations(self):
        records = []
        env = self._env(FAKE_GO_MODE="invalid_utf8")
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, _, _ = self._run(record_observation=lambda *args: records.append(args))
        self.assertEqual((status, output), (0, b"\xff"))
        self.assertEqual(len(records), 1)
        self.assertEqual(records[0][1], "complete_within_window")
        self.assertEqual(records[0][3:], ("raw", 1, 1))

        env = self._env(FAKE_GO_MODE="silent")
        with mock.patch.dict(os.environ, env, clear=True):
            status, output, _, _ = self._run(
                capture_seconds=0.03, record_observation=lambda *args: records.append(args),
            )
        self.assertEqual((status, output), (0, VALID))
        self.assertEqual(records[-1][1], "deadline_stream")
        self.assertEqual(records[-1][3:], ("raw", len(VALID), len(VALID)))

    def test_record_helper_bounds_payload_and_reaps_timed_out_attempt_once(self):
        process = mock.Mock()
        process.communicate.side_effect = [subprocess.TimeoutExpired("pith", 0.01), b""]
        process.returncode = -9
        with mock.patch.object(pith_adapter.subprocess, "Popen", return_value=process) as popen:
            pith_adapter._record_wrapper_outcome(
                self.pith_binary, "🙂abc".encode(), "🙂".encode(), 15, "transformed", timeout=0.01,
            )
        popen.assert_called_once()
        args, kwargs = popen.call_args
        self.assertEqual(args[0], [self.pith_binary, "_record-codex-wrapper"])
        self.assertIs(kwargs["stdout"], subprocess.DEVNULL)
        self.assertIs(kwargs["stderr"], subprocess.DEVNULL)
        self.assertEqual(json.loads(process.communicate.call_args_list[0].args[0]), {
            "originalRunes": 4, "deliveredRunes": 1, "durationMs": 15, "outcome": "transformed",
        })
        process.kill.assert_called_once()

        with mock.patch.object(pith_adapter.subprocess, "Popen") as popen:
            pith_adapter._record_wrapper_outcome(
                self.pith_binary, b"x" * (pith_adapter.CAPTURE_LIMIT + 1), b"", 1, "raw_fallback",
            )
        popen.assert_not_called()

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
        self.assertEqual(pith_adapter.CAPTURE_SECONDS, 30.0)
        self.assertEqual(pith_adapter.TRANSFORM_SECONDS, 2.0)
        source = SCRIPT.read_text(encoding="utf-8")
        self.assertNotIn("start_new_session", source)
        self.assertNotIn("os.setsid", source)
        self.assertNotIn("os.killpg", source)
        self.assertNotIn("_source_matches", source)
        self.assertNotIn("_git_status_clean", source)
        self.assertNotIn("_identity_matches", source)
        self.assertNotIn("sourceHead", source)
        self.assertNotIn("sourceTree", source)
        self.assertNotIn("sha256", source)


if __name__ == "__main__":
    import unittest

    unittest.main()
