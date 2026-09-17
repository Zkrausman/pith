# Preservation-reason accounting

Pith execution accounting carries one bounded `decision_reason` field in SQLite
and JSONL. The shared type and normalization live in
`pkg/telemetry/telemetry.go`.

- `transformed`: accepted parser processing, or actual byte-changing middle-out
  truncation in runner/main. This is not a guarantee of reduction.
- `protected_passthrough`: an existing explicit bypass or preservation guard.
- `unsupported_parser`: no eligible/enabled parser, or Pi's unchanged unsupported
  Git-log result.
- `rejected_non_reduction`: Pi's existing grep byte/token reduction gate rejected
  the parsed representation.
- `unknown`: legacy, absent, invalid, or unclassifiable metadata.

## Architectural constraints

Assign reasons at existing branches in `pkg/pi/hook.go`,
`pkg/runner/runner.go`, and the independent `runHook` in `main.go`. Do not infer
historical decisions from commands, outputs, parser names, or token counts. Do
not change parser selection, preservation guards, or transformation policy to
manufacture a reason. No generic chain compression.

Runner/main middle-out transformation overrides an earlier passthrough reason
only when it changes bytes. Existing `ParserUsed`/`IsPassthrough` provenance stays
unchanged, so a transformed reason can coexist with parser passthrough. Pi has
no new middle-out behavior. Reasons are not added to hook response contracts.

## Storage and privacy boundaries

The additive column is `TEXT NOT NULL DEFAULT 'unknown'`. Initialization checks
column existence before adding it and returns genuine migration errors. Legacy
rows stay unknown. The allowlist normalizes storage writes, imports, execution
reads, and exports; arbitrary explanation text is not retained as a reason.

Record and import always clear both output-content fields. Full and incremental
JSONL exports clear them even for legacy rows written after initialization.
Existing Record command redaction and JSONL command metadata remain compatible;
this is not a new command-retention facility. Import uses `INSERT OR IGNORE` and
the unchanged `(timestamp, command, duration_ms)` unique identity. Reason
changes do not create a new execution identity.

Existing explicit opt-outs must return before constructing telemetry: opening a
store can migrate it even without a record insertion. Pi CLI omission continues
to mean enabled, explicit `telemetryEnabled: false` means no create/migrate/write,
and Go `OptimizeHook` retains its disabled zero-value bool. Do not add a new
consent source or undo staged-update/storage migration hardening.

## Validation

Use synthetic temporary homes/databases only. Regression coverage resides in
`pkg/telemetry/decision_reason_test.go`, `pkg/pi/decision_reason_test.go`,
`pkg/runner/decision_reason_test.go`, `main_decision_reason_test.go`, and existing
consent/preservation tests. Run tests, vet, and build with the repository's
Go 1.26.1 toolchain. Clear attribution variables in validation subprocesses:
`PITH_HARNESS`, `ANTIGRAVITY`, `ANTIGRAVITY_IDE`, `GEMINI_SESSION_ID`, `GEMINI_CLI`,
`GOOGLE_API_KEY`, `CLAUDE_CODE`, and `ANTHROPIC_API_KEY`. Do not weaken production
harness detection or change workflow authentication.

See `pkg/telemetry/README.md` and `PRIVACY.md` for the public contract.
