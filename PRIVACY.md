# Privacy and local diagnostic logs

Pith stores telemetry in its configured local storage directory. Diagnostic command/output logs are **disabled by default**.

To opt in, set `snag_logging: true` in Pith's `config.json`. Pith redacts common API-key, token, secret, password, and bearer-token values before writing, but redaction is heuristic and cannot guarantee removal of every sensitive value. Logs are owner-only, retain at most 1 MiB before one rotated prior file is kept, and can be disabled again by setting `snag_logging` to `false`.

`pith reset --all` removes telemetry and both diagnostic log files. Use a storage directory with appropriate local-disk protections.

## Bounded execution accounting

Telemetry remains default-enabled on existing CLI entry points. Pi transform
requests can explicitly set `telemetryEnabled: false`; this is honored before
storage creation or migration. Go `OptimizeHook` callers must explicitly enable
telemetry. Existing consent controls and diagnostic-log opt-in are unchanged.

Execution accounting includes one fixed `decision_reason` value: `transformed`,
`protected_passthrough`, `unsupported_parser`, `rejected_non_reduction`, or
`unknown`. It describes an existing processing decision, not personal analytics
or a free-form explanation. Legacy, missing, and invalid reasons are `unknown`;
Pith does not infer reasons from historical commands or output.

This metadata adds no command/output retention and does not expand existing
command metadata. Record retains its existing credential redaction for command
metadata. Record and JSONL import clear `original_content` and
`compressed_content`; both full and incremental exports emit these fields empty,
including for legacy rows. JSONL retains its existing command metadata and
duplicate identity for compatibility. No new raw command/output logs are enabled.

## Network behavior

Update checks use anonymous requests to the public GitHub Releases API and never read `GITHUB_TOKEN` or invoke `gh auth`. `pith audit anomalies --diagnose` sends bounded, redacted diagnostic snippets to Gemini only when you also pass `--allow-external-ai`; without that flag no data is sent. The local dashboard bundles its JavaScript, CSS, and fonts and makes no browser requests to third-party CDNs.
