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

`pre_tool_use_pith.py` is a not-yet-installed Codex `PreToolUse` adapter. Configure it through the normal trusted hook boundary with the supported Python interpreter, this adapter, and an absolute Pith executable path. It recognizes one literal command shape, `<absolute-path>/go test -v <single-package>`, without evaluating shell syntax; it preserves all tool options and changes only the command. The wrapper executes that already-authorized native argv directly and at most once. Pith only receives completed stdout for transformation; it does not select or authorize native commands. The adapter relies on the normal trusted hook, interpreter, adapter, and Pith installation boundary and does not attest to the repository, Go binary, environment, or a particular package at every invocation. Same-user replacement of those installed files is outside this boundary.

TTY descriptors or descriptor inspection errors execute the original command directly. The adapter inherits cwd, environment, stdin, and native stderr. Eligible stdout is held while the native command completes, up to a two-second selection deadline and a one-MiB cap; timeout or overflow relays the captured prefix and remaining stdout as raw bytes exactly once. Transformation uses only the remaining deadline and rechecks it after preparation and immediately before the gated Pith launch. Preparation itself can overrun the deadline, so this is not a two-second end-to-end completion guarantee. If Pith times out or returns malformed, oversized, non-UTF-8, recursive, diagnostic, or otherwise non-fidelitous output, native stdout is published unchanged. Cleanup kills only the direct Pith child and waits up to 50 ms; if reaping remains unknown, the output falls back to raw and the adapter reports cleanup as unsettled. Native status is returned separately; Pith failures never cause a native rerun.

The adapter remains a small Go-test selection slice, not broad command coverage. It is not installed or trusted by this candidate. The fake-only gates are:

```sh
python3 -B -m unittest -v test_pre_tool_use_pith.py
```

## Focused tests

Run from this directory:

```sh
python3 -B -m unittest -v test_capture_command.py
```

The suite uses mocked process, transform, and publication results. It checks the adapter contract without invoking Pith or a real command; it does not establish engine output behavior.
