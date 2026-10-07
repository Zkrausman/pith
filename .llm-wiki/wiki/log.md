# Wiki Update Log

## 2026-10-07

- Normal Middle-Out truncation now discloses each omitted source segment, including one-line gaps before or between hot windows, without duplicating or reordering retained lines. Gap counts use the actual retained-window start. A final newline terminates the final line instead of consuming an extra tail slot; actual blank lines still count and retained bytes are preserved. Deterministic tables, exhaustive hot-line placements, and compiled-entrypoint fixtures cover boundaries and raw passthrough. Existing keywords, context width, marker formats, numeric exit status, raw bypass, shell/stream behavior, and telemetry/privacy are unchanged. Compression remains lossy; upstream omission handling and independent stream fidelity are separate work. Patch version: v3.0.3; no release or installation is implied.

- Explicit `pith raw` now uses `Runner.RunRaw` to bypass both parser dispatch and head/tail/hot-zone truncation. `RunWithOptions(skipParsing=true)`, disabled parsers, and normal parser compression retain their previous truncation behavior. The raw contract is complete captured stdout followed by stderr on Pith's stdout, preserving bytes without claiming original stream destinations or interleaving. Existing CLI diagnostics, child exit status, shell behavior, telemetry, and privacy policies remain unchanged. Deterministic runner and compiled-entrypoint fixtures cover above-limit output, UTF-8, trailing newlines, empty output, and misleading failure text. Patch version: v3.0.2; no release or installation is implied.

- Preserve normal numeric child exit statuses at `main.go`'s process boundary using `errors.As`, including wrapped exit errors. Generic CLI failures and Unix signal termination retain status 1; no signal forwarding, parser, stream, telemetry, or startup policy changes. Regression fixtures compile the shipped `main.go` entrypoint and a deterministic helper, exercise normal/raw routes with statuses 0, 1, 2, 7, 42, and 127, and keep configuration and databases synthetic. Linux full tests and Windows focused CLI tests validate the change in CI. Patch version: v3.0.1; no release or installation is implied.

## 2026-10-06

- Linked the public [EmbeddingGemma 2 feasibility proposal](https://github.com/Zkrausman/Squire/blob/3335ba528b9af884c9ff17c6e6af40c43579a46e/docs/proposals/embedding-gemma-integration.md) from the roadmap and knowledge index. Offline-only experiment scope, explicit sample permission and output-fidelity prerequisites remain proposed; no implementation, inference or benchmark was performed.

- Added a source-reviewed [output-fidelity regression roadmap](../../docs/output-fidelity-regressions.md), based on commit `6939184b197085c2e3d2dd484e169123fb7a70c6`. Priorities cover CLI exit codes, child-stream fidelity, late failures, complete inventories/counts, honest omissions, raw passthrough, and unsupported parser output. Existing Jest `total` summaries are recognized; successful empty Git porcelain output has valid command semantics. Proposed cases remain unexecuted and implementation is pending.

## 2026-09-16

- **retro**: {"category":"release","slug":"pith-windows-gcc-emutls-release-failure","title":"Windows GCC emutls release failure"}
- **retro**: {"category":"release","slug":"pith-v241-release-gate","title":"Pith v2.4.1 release gate"}
- **retro**: {"category":"deployment","slug":"pith-local-upgrade-merged-aidev-153","title":"Pith local upgrade to merged AIDEV-153"}
- **retro**: {"category":"testing","slug":"pith-provenance-count-semantics","title":"Pith provenance count semantics"}
- **observe**: {"relevance":"medium","slug":"obs-2026-09-16-aidev-153-recovery-restarted-in-pith-worktree","title":"AIDEV-153 recovery restarted in Pith worktree"}
