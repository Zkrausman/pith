# Explicit completed-command caller

`capture_command.py` is an opt-in Linux wrapper for one direct command. The caller supplies both the command and the absolute path to a Pith v3.0.7 Linux amd64 binary. The wrapper verifies Pith's pinned SHA-256, captures the command's stdout, stderr, native status, and settlement, then runs Pith's transform on stdout and returns one bounded JSON packet. Raw streams and receipts stay in a private directory under `/tmp` and are referenced by the packet.

From the repository root:

```sh
python3 integrations/codex/capture_command.py \
  --cwd /absolute/path/to/project \
  --pith-binary /absolute/path/to/pith-linux-amd64 \
  --timeout-seconds 120 -- /usr/bin/node /absolute/path/to/vitest.mjs run
```

The Pith executable must match SHA-256 `ce8b2d3888e533bb237f5469e1815ee1c542121fcb0aed2142cc07f7e7e2ba98` and source commit `bbe9fc5e05cbd20a9be2e1d98f23894fc46ff186`. The pinned parser source is [`pkg/parser/vitest.go` at that commit](https://github.com/Zkrausman/pith/blob/bbe9fc5e05cbd20a9be2e1d98f23894fc46ff186/pkg/parser/vitest.go). The caller accepts only an absolute native Linux ELF executable; script launchers and PATH lookup are unsupported. It runs the child argv without a shell, hooks, global interception, or a background service. Commands that detach or create another session are outside its coverage.

This caller does not intercept the desktop app's internal command paths, code mode, hosted tools, or other dot/tool routes. A successful invocation demonstrates only that this selected command was captured and transformed before the wrapper returned its packet. App-origin execution and whole-turn delivery require separate evidence.

## Capture and delivery

The wrapper stores stdout and stderr as separate byte files in a new mode-0700 directory under `/tmp`; files and receipts use mode 0600. The receipt binds argv, canonical working directory, resolved command identity, raw stream sizes and hashes, helper identity, Pith identity, transform status, and delivery envelope.

Capture limits are 1 MiB stdout and 256 KiB stderr. Pith receives stdout, the command name rendered from argv, and the native exit code. It runs once with `rawBypass:false`, `telemetryEnabled:false`, private `HOME` and `PITH_STORAGE`, and no `enabledParsers` field. Timeout, cancellation, overflow, missing EOF, invalid UTF-8, or uncertain process-group cleanup prevents a successful selection. Command stderr is retained separately and is not sent to Pith or repeated in the packet. Proxy, API-key, and other inherited environment values are not passed to the child.

The native exit code is preserved when known; signals, timeout, cancellation, overflow, and unknown status remain explicit. Cancellation is checked before each launch and remains latched through validation, evidence writes, receipt creation, and publication. A cancellation clears the selection and its reduction claim before publication. If receipt persistence fails, the wrapper publishes no selection or reduction claim. Once stdout publication starts, it never appends a second JSON packet. `publication.json` records bytes written and delivery uncertainty; cancellation during final status persistence is recorded there and returns status 125.

`deliveryComparison` compares the complete selected JSON packet, including its terminating newline, with an equivalent packet containing raw UTF-8 stdout and the same status and evidence references. It measures packet bytes, not model tokens or total interaction size. Failed or missing selection cannot claim a reduction; passthrough can be equal to or larger than raw output.

## Codex PreToolUse adapter candidate

`pre_tool_use_pith.py` is a separate, not-yet-installed Codex `PreToolUse` adapter. It rewrites only an absolute Go executable invoked as `go test -v PACKAGE`, with one literal package path. It does not interpret shell syntax. The rewritten adapter preserves the inherited working directory, environment, stdin, stderr, and process group while running the command once.

TTY descriptors or descriptor inspection errors execute the original executable directly. For noninteractive runs, stdout is buffered for at most two seconds and one MiB. Deadline or overflow flushes the captured prefix and switches once to exact raw stdout relay. Completed successful output is sent to `pith pi transform` with telemetry disabled; the adapter selects only a shorter `tests` parser result that retains the complete `PASS` and package `ok` summary. Native failures, diagnostic output, malformed results, timeouts, and other fidelity failures return native stdout and status. This intentional buffering delays eligible stdout while compression is evaluated. The adapter is a narrow command profile, not complete Codex command coverage; detached descendants and current app cancellation behavior still require app-level validation.

The hook configuration will need an absolute trusted Python interpreter, adapter path, and Pith binary path. The separate `pith --hooks` installer and normal Codex trust/adoption flow are not part of this runtime candidate. Its fake-only gates are:

```sh
python3 -B -m unittest -v test_pre_tool_use_pith.py
```

## Focused tests

Run from this directory:

```sh
python3 -B -m unittest -v test_capture_command.py
```

The suite uses mocked process, transform, and publication results. It checks the adapter contract without invoking Pith or a real command; it does not establish engine output behavior.
