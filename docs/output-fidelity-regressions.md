# Pith output-fidelity regression specification

Status: proposed implementation roadmap. This documentation does not change shipped behavior.

Source baseline: [Zkrausman/pith, commit 6939184b197085c2e3d2dd484e169123fb7a70c6][baseline], inspected 2026-10-06. Implementation descriptions below refer to that pinned commit. These are source-derived acceptance cases, not executed reproductions or measured performance results.

Implementation progress (2026-10-07): normal numeric CLI exit status is covered by v3.0.1. In v3.0.2, explicit `pith raw` bypasses parsing and truncation and preserves the complete captured stdout-plus-stderr bytes, with runner and compiled-entrypoint regressions. This completes only the raw truncation-bypass portion of addition 4. In v3.0.3, normal Middle-Out truncation reports every omitted segment, including one-line gaps, and counts a final newline as a terminator rather than a separate tail line. Deterministic primitive and compiled-entrypoint regressions cover that accounting; actual blank lines still count, and retained line bytes, order, and existing marker formats are preserved. In v3.0.4, a normal wrapped command whose execution returns an error preserves its complete captured stdout-plus-stderr output without parsing or truncation. This covers authoritative nonzero, signal, and startup failures, with protected-passthrough accounting; successful-command compression is unchanged. Independent stream routing, upstream-loss preservation, exit-zero failure heuristics, and the other proposed changes below remain pending. Normal compression remains lossy. The pinned baseline observations below remain historical.

## Goal and scope

A software-delivery agent must be able to distinguish failure, incomplete evidence, and successful completion after compression. Token reduction is secondary to preserving the facts needed to decide whether to inspect, retry, fix, or finish.

Cover the shipped `pith` and `pith raw` execution paths, plus `pith pi transform` where noted. The direct runner executes a command; the Pi hook transforms an already completed result. A successful transform process does not mean the original command succeeded. Preserve the host's original exit status separately when integrating the transform response.

The five additions below are proposed acceptance contracts. Several require behavior changes; they are not claims that the baseline already satisfies these guarantees. Implement and validate them in separately reviewed engineering PRs.

## Coverage already present

- Pi has preservation guards and tests for nonzero exits, errors, warnings, final test summaries, diffs, valid JSON including scalars, Git inspection, upstream truncation, and raw bypass. Its tests also cover UTF-8 omission counts, trailing newlines, non-expansion, and overlapping retained windows. Preserve the hook's mandatory redaction when extending this coverage. [Pi tests][pi-tests], [hook tests][hook-tests], [preservation boundaries][boundary-tests]
- Parser tests check a multi-line test failure across a blank line; the Vitest fixture checks a failure header, message, file summary, and failed-file count. They do not establish the complete late diagnostic block specified below. [TestParser fidelity test][test-fidelity], [Vitest tests][vitest-test]
- Runner tests exercise command errors, head/tail preservation, a hot error line, and dispatch protection. The error test checks only `err != nil`; it does not establish the shipped process's numeric exit status. [Runner tests][runner-tests], [semantic test][semantic-test], [dispatch tests][dispatch-tests]
- `TestRunnerDecisionReasons` checks concrete output and expects truncation even when parsing is skipped. A new lossless raw acceptance test must deliberately reconcile that expectation. [Decision tests][decision-tests]
- Git status tests verify three retained filenames, while Pi tests protect `--porcelain` output. Neither establishes the direct CLI's complete changed-file inventory beyond the status parser's 20-entry cap. [Git tests][git-test], [hook tests][hook-tests]
- The Jest-style aggregate lines below containing `total` are already recognized by TestParser's case-insensitive `TOTAL` summary match. Exact count assertions should lock in that behavior, rather than report these inputs as a known parser failure. [TestParser][test-source]

## Five prioritized additions

### 1. P0: preserve the executed command's numeric exit status

Suggested test: `TestCLIExitStatusFidelity`, in a new main-package subprocess test file.

Use a synthetic helper executable with a requested exit code and independently controlled stdout/stderr. Run it through both `pith -- <helper>` and `pith raw -- <helper>` for exit codes `0`, `1`, `2`, `7`, `42`, and `127`.

Concrete output variants:

```text
stdout: ""
stderr: ""

stdout: "25 tests passed\n"
stderr: "post-test upload refused\n"
```

Required invariants:

- The Pith process returns exactly the helper's numeric exit code, including silent failure and output that sounds successful.
- Nonzero status cannot be replaced by zero, collapsed to generic 1, or inferred from text. Zero status must remain zero even when stderr is nonempty.
- Assert process status and diagnostic preservation separately. A Cobra error string containing `exit status 42` is insufficient.
- For a Pi request with `exitCode: 42`, transformation may itself exit zero, but output must remain protected and the calling integration must retain 42 as the command result. Do not invent an `exitCode` response field: the baseline `HookResponse` has none.

Why this adds value: the runner returns the underlying error, but `main()` exits 1 for every error. Existing runner tests do not reach that boundary. [Entrypoint][main-entry], [runner execution][runner-source], [hook contract][hook-contract]

### 2. P0: preserve stream meaning and complete late failures

Suggested tests: parameterized `TestRunnerFailureFidelity` with a shipped-CLI stream subtest.

Fixture A, exit 7:

```text
stdout: {"artifact":"dist/app.tgz"}
stderr: warning: upload refused; retry required\n
```

Here stdout has no final newline. Repeat with exit 0 to ensure a warning remains a warning, without inventing failure.

Fixture B: generate exactly 60 lines `routine 001` through `routine 060`, then append:

```text
FAIL checkout.test.ts > rejects expired session
AssertionError: values differ
Expected: "expired"
Received: "valid"

  at checkout.test.ts:42:7
Test Files  1 failed (1)
Tests  1 failed | 24 passed (25)
```

Use command identity `vitest run` and exit 1, then repeat with exit 0 to test failure-marker protection. Set runner truncation limits low enough that the parser's retained result also crosses its second compression stage.

Required invariants:

- Raw execution preserves child stdout and stderr independently, byte for byte. For compressed execution, keep child stderr on stderr; never concatenate an unterminated stdout record with a diagnostic or silently turn diagnostic text into machine data. This is a proposed stricter stream contract.
- Retain every line from the failure header through the exact file/test totals, including expected/received values, the blank separator, and `checkout.test.ts:42:7`.
- Prefer lossless failure output over a heuristic snippet. No success summary may replace the failure or suggest all 25 tests passed.
- Run the same failure text through the Pi hook as a cross-surface control using its mandatory-redaction policy. These fixtures contain no secrets, so exact output equality is appropriate.

Why this adds value: the runner builds `stdout + stderr`, parses it without an exit-status guard, and prints the result to stdout. Vitest retains selected headers after its general 20-line allowance but can drop later expected/received and stack context. Pi's protection already covers these failure fixtures. [Runner execution][runner-source], [Vitest parser][vitest-source], [Pi preservation guard][pi-source]

### 3. P1: preserve authoritative changed-file and test counts

Suggested test: table-driven `TestDeliveryEvidenceCounts`, exercising actual parser dispatch as well as parser units.

Fixture A, command `git status --short`, exit 0: generate ` M src/file01.go\n` through ` M src/file22.go\n`, followed by:

```text
R  src/old.go -> src/new.go
```

This represents 23 change records, including one rename record with two paths. Repeat as `git status --porcelain=v1 -z` using real NUL separators. The rename record is `R  src/new.go\0src/old.go\0`: destination before source, without the arrow. Assert exact bytes for the machine-readable variant. [Git status format][git-manual]

Fixture B, command `npm test`, exit 0:

```text
Test Suites: 2 passed, 2 total
Tests:       2 skipped, 23 passed, 25 total
```

Add a failed variant: `Test Suites: 1 failed, 1 passed, 2 total` and `Tests: 1 failed, 2 skipped, 22 passed, 25 total`, with exit 1.

Also exercise summary-only input without `total`:

```text
Test Suites: 2 passed
Tests:       2 skipped, 23 passed
```

Required invariants:

- All 23 change records remain discoverable; retain the rename and its status. A first/last-ten listing with an unlabeled ellipsis is insufficient for an agent that must enumerate changed files. Conservative passthrough is acceptable.
- Preserve the two status columns and distinguish one rename from two independent changes. Do not derive an original changed-file count from retained rows after truncation.
- Retain the exact suite and test counts, including skipped tests. Never infer that 23 passed means 25 passed.
- Preserve the already-supported aggregate summaries. For a summary format the parser does not recognize, retain the original instead of substituting `Tests finished. (No summary captured)`.
- Verify `npm test` selects the `tests` parser. Use Pi's existing summary/inspection protection as controls; do not equate those controls with direct CLI behavior.

Why this adds value: GitStatusParser drops every line containing `->`, strips leading status-column whitespace, and caps retained output at 20 rows. TestParser retains the supplied `total` aggregates, but the successful summary-only variant lacks its current summary/failure triggers. Its fallback test deliberately expects a completion message for unrecognized text. [Git parser][git-source], [TestParser][test-source], [fallback test][fallback-test], [registry][registry]

Related work: [PR #219][rename-pr], open as of 2026-10-06, preserves human-readable `renamed:` rows. Its proposed condition still filters short-format `R  ... -> ...` rows; its scope does not establish the complete >20-record inventory, two-column fidelity, or NUL-format contract above. Recheck its status and implementation before starting overlapping work.

### 4. P1: make every omission honest, and make raw lossless

Suggested test: `TestRunnerOmissionAccountingAndRaw`, covering the truncation primitive and CLI boundary.

Use `MaxLines=5`, `HeadLines=2`, `TailLines=2` with these eight newline-separated segments and no final newline:

```text
head1
head2
hidden3
context4
ERROR failure5
context6
tail7
tail8
```

Required invariants:

- At the truncation primitive, every missing source line is disclosed with exact omission counts. In particular, if `hidden3` is removed, a marker must report that one omitted line.
- Retained source segments occur once and in order; overlapping hot windows must not duplicate or reorder them. Repeat with adjacent hot lines and a trailing newline to catch boundary/count errors.
- Through `pith raw`, return the entire original output exactly, even above `MaxLines`. This resolves the difference between the README's bit-for-bit promise and baseline skip-parsing behavior.
- For a long result containing `... output truncated by host ...`, preserve that marker and its available safe artifact reference. Do not relabel upstream loss as Pith omission. Reuse existing Pi upstream tests as controls.

Why this adds value: for the eight-line input, source inspection predicts that the hot-zone boundary skips `hidden3` without entering either marker branch. The runner also applies truncation after the parser bypass. These are unexecuted source-derived cases. [Truncation implementation][runner-source], [raw promise][readme], [raw CLI routing][raw-routing], [decision tests][decision-tests]

### 5. P1: unsupported output must not invent evidence

Suggested test: `TestNoInventedSuccess`, for relevant parser units, the runner, and Pi transform.

Implemented in v3.0.5: the affected Git/test/coverage parsers conservatively return captured output when no retained evidence exists, and preserve structured-looking captures rather than extracting partial JSON fields. Git without recognized human-readable context preserves original bytes. Pi reports unchanged fallback output as passthrough, with its mandatory redaction still applied. Deterministic parser-registry and hook fixtures plus `TestCLIExitStatus/parser-fallback-evidence` exercise normal/raw execution and both shipped hook routes; synthetic command aliases avoid invoking actual developer tools. Recognized summary controls remain covered. Independent runner/legacy-hook truncation still applies above configured limits, so this is not a general lossless-output claim.

Concrete table rows:

```text
command: git status
output:  ""
exit:    0, then 2

command: git status --porcelain=v1
output:  ""
exit:    0, then 2

command: go tool cover -func missing.cov
output:  ""
exit:    0, then 1

command: npm test --json
output:  {"stats":{"tests":25,"passes":23,
exit:    0, then 2

command: npm test
output:  collecting 25 cases\nconnection closed\n
exit:    2
```

The exit-zero missing-coverage row is intentionally synthetic. It tests how the parser handles absent evidence, not the expected behavior of a real missing-file invocation.

Required invariants:

- Preserve empty output and the command's actual exit status. Empty text alone must not cause a parser to assert success or 100% coverage.
- Interpret emptiness in the command's context: a successfully completed, untruncated `git status --porcelain=v1` with empty output legitimately indicates no reported changes in its inspected scope. Keep the bytes empty and the status zero. This does not establish ignored files, excluded paths, hidden untracked files, or ignored submodule changes. A failed or incomplete capture provides no such clean-state evidence. [Git status semantics][git-manual]
- Malformed JSON stays available exactly as received for diagnosis, subject to the surface's redaction policy. Do not repair it, extract a partial passing count, erase it, or turn it into a completion sentence. These fixtures contain no secrets.
- Unknown text passes through conservatively. No new `Success`, `100.0%`, or claim of completed tests may appear without support from the captured result and known command semantics.
- A nonzero exit remains nonzero and protects diagnostics. For Pi's empty/unsupported cases, retain passthrough provenance rather than reporting parser transformation that invented evidence.

Why this adds value: GitStatusParser's empty-result fallback unconditionally says success without access to exit status. GoToolCoverParser can emit 100% coverage when it found no coverage evidence, and TestParser substitutes a completion fallback for unrecognized text. Pi protects valid JSON, but malformed JSON fails that guard and can reach a parser on a zero-exit result. The defect is unsupported parser-generated evidence, not legitimate interpretation of a successful command's documented empty output. [Git parser][git-source], [test/coverage parsers][test-source], [hook JSON guard][hook-contract]

## Implementation and acceptance notes

- Use small, deterministic fixtures only. No real repositories, uploads, private source, telemetry histories, provider calls, or external services are needed to exercise these proposed cases.
- Reuse the helper-process approach and shipped-entrypoint test pattern. Isolate `HOME`, `USERPROFILE`, and `PITH_STORAGE`; set `LastUpdateCheck` to the fixture's current time so normal CLI tests do not trigger an update lookup. Capture stdout and stderr separately, not with `CombinedOutput`. Configure synthetic runs to avoid real diagnostic logs and telemetry. [Helper pattern][decision-tests], [entrypoint pattern][startup-test], [startup behavior][main-entry]
- Test parser dispatch through isolated fixture executables or a package-local adapter using the real parser. Invoke a Git fixture by the bare token `git` on an isolated `PATH`: GitStatusParser's matcher requires that exact command token. A generic helper name or absolute path ending in git does not establish that dispatch.
- For numeric status propagation, exercise the compiled Pith entrypoint rather than only `NewRootCmd().Execute()`. Specify how Pith's own wrapper errors are surfaced while keeping child-stream assertions unambiguous.
- Use exact equality for machine output, raw output, and protected failure blocks; use explicit record/count assertions for legitimate summaries. Token savings and substring checks alone are not acceptance criteria.
- Add tests and implementation changes in later engineering PRs. This document alone does not establish that the behavior is safe. Run focused tests, then the repository's `go test -v ./...` CI command and applicable native Windows validation; report failures and never-run stages separately. The documentation PR's existing CI does not execute these proposed fixtures. [CI workflow][ci]
- Documentation-only changes need no binary version bump. Later shipped-behavior changes must follow the repository's version/changelog policy. [Repository instructions][agents]

[baseline]: https://github.com/Zkrausman/pith/commit/6939184b197085c2e3d2dd484e169123fb7a70c6
[pi-tests]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/pkg/pi/pioptimize_test.go
[hook-tests]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/pkg/pi/hook_test.go
[boundary-tests]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/pkg/pi/context_audit_test.go
[test-fidelity]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/pkg/parser/new_parsers_test.go
[vitest-test]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/pkg/parser/vitest_test.go
[runner-tests]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/pkg/runner/runner_test.go
[semantic-test]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/pkg/runner/semantic_test.go
[dispatch-tests]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/pkg/runner/parser_dispatch_test.go
[decision-tests]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/pkg/runner/decision_reason_test.go
[git-test]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/pkg/parser/git_test.go
[main-entry]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/main.go#L217-L265
[runner-source]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/pkg/runner/runner.go#L235-L376
[hook-contract]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/pkg/pi/hook.go
[vitest-source]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/pkg/parser/vitest.go
[pi-source]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/pkg/pi/pioptimize.go
[git-source]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/pkg/parser/git.go#L26-L61
[test-source]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/pkg/parser/infra.go#L111-L229
[fallback-test]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/pkg/parser/coverage95_test.go#L426-L457
[registry]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/pkg/parser/interface.go#L41-L82
[readme]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/README.md#L41-L45
[raw-routing]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/main.go#L749-L764
[startup-test]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/main_pi_startup_test.go#L31-L57
[ci]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/.github/workflows/test.yml
[agents]: https://github.com/Zkrausman/pith/blob/6939184b197085c2e3d2dd484e169123fb7a70c6/AGENTS.md
[rename-pr]: https://github.com/Zkrausman/pith/pull/219
[git-manual]: https://git-scm.com/docs/git-status
