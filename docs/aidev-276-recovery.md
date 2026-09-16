# AIDEV-276: bounded, metadata-only local telemetry recovery

## Scope and authority

This is an operator-run, Python standard-library one-off, **not a Pith command or
shipped product change**. `main.go` and `CHANGELOG.md` intentionally remain
unchanged: operational tooling, synthetic tests, and this procedure do not change
the distributed binary or embedded assets (version exemption).

The user authorized the parent operator to back up and import history while
preserving the growing live database. Independent review precedes execution.
Implementation work must not modify any real database, run Pith against real
home, install anything, or commit/push. No Squire changes. Do not call
`NewTelemetry`, `NewTelemetryWithPath`, or application migration APIs: opening
through them can mutate history/storage. The implementation's real-data audit
used only `mode=ro`, `query_only=ON`, PRAGMA/schema metadata and aggregate queries;
it did not retrieve command or output strings. The script itself has only been
run against synthetic fixtures during implementation.

## Exactly these inputs

| Role | Native path | Expected rows and inclusive timestamp bounds |
| --- | --- | --- |
| Source 0 | `E:\pith-worktrees\pith-release-v243-ZHKk44\storage\pith.db` | 27,649; `2026-08-11 06:33:00` through `2026-09-16 15:13:24` |
| Source 1 | `E:\pith-worktrees\pith-release-v244-8bhFdO\storage\pith.db` | 99; `2026-09-16 15:13:39` through `2026-09-16 15:44:07` |
| Live destination | `C:\Users\zkrau\.pith\pith.db` | Growing; earliest row must remain `2026-09-16 15:44:22` |

Import exactly **27,748** records. The destination count is deliberately not fixed:
it had 89 rows at the implementation audit and can continue growing. The `.bak`
that duplicates Source 1 and all smoke-test databases are **excluded**. The CLI
hardcodes this allowlist; it does not glob, scan home, or accept alternate inputs.
Changes to the inputs/bounds require new review, not relaxed assertions.

## Intentional historical-detail limitation

Preserve only timestamp, original/compressed token counts, duration, parser,
passthrough, source, harness, model and recorded rate. Original source IDs are
used for source fingerprinting/record identity, **not** as destination IDs. The
insert omits destination ID, allowing SQLite AUTOINCREMENT to assign fresh IDs
above existing records and any previous sequence high-water mark.

Do not read/copy historical commands or original/compressed output. Both content
fields become `''`. A fully blank command conflicts with the existing unique
index `(timestamp, command, duration_ms)`: the audit found 251 Source 0 and 4
Source 1 collision groups when ignoring command. These are legitimate records,
not duplicates to discard. NULL would bypass that constraint but Go readers scan
commands into strings and may fail. The supervisor therefore approved explicit,
deterministic, non-secret placeholders instead:

```text
[recovered metadata:aidev-276:<full-safe-metadata-SHA256>:<original-source-id>]
```

These are **not actual commands**. Full source fingerprint plus original unique
ID distinguishes collisions within/across sources. Historical command details
are unavailable. Do not derive command-level analytics, discovery conclusions,
or recommendations from recovered placeholders. Token/cost/parser/time totals
remain available, but prior command search/details cannot be recovered by this
procedure. No product UI/report filtering is added. FTS indexes the placeholder;
original historical commands/output are never inserted into destination FTS.
The existing unique index and all application triggers remain unchanged.

## Safety and transaction design

1. Inputs must exist, be distinct files, and be exactly the approved pair. The
   source attachments use URI `mode=ro`. No `immutable=1` shortcut ignores WAL.
2. **Apply:** create a new, exclusively named local SQLite online backup of the
   live database **before** opening the write transaction. Never copy just the
   `.db` file while SQLite may be using WAL. Verify the backup with SQLite
   `integrity_check`; never overwrite a backup. The snapshot may predate live
   commits that occur before import acquires the write lock. Those commits are
   preserved and counted from the locked live database, not the backup.
3. **Dry-run:** online-backup the read-only live database to memory, then rehearse
   the same import/checks there. No disk backup or logical input changes; SQLite
   may still use normal read-side WAL/shared-memory locking artifacts. The temporary
   snapshot necessarily contains live data in process memory; no strings are
   queried for display, logged, or written to recovery artifacts. Closing the
   process discards it; normal OS memory/pagefile policies still apply.
4. `BEGIN IMMEDIATE` has a 3-second busy timeout. Backup and import each have a
   60-second cooperative deadline. Backup copying and its integrity verification
   share that single deadline; callbacks also bound verification SQL on the backup
   connection. These are cooperative checks, not hard wall-clock limits: callbacks
   run between SQLite work units, not continuously. They cannot interrupt stalled OS
   I/O. Contending live writers may wait or receive their own busy timeout;
   schedule a quiet interval if losing application telemetry on busy is a concern.
5. Under the transaction, validate the exact schema/FTS SQL allowlist and explicit
   `table_info`, `index_list`, and `index_xinfo` metadata. Unexpected tables,
   indexes, triggers, missing FTS, or a prior recovery ledger fail closed.
   Assert SQLite integrity, exact source counts/endpoints, canonical non-null
   timestamps, source-to-source separation and source-before-live separation.
   Source snapshots remain pinned through commit. The known live earliest
   timestamp must match; the latest timestamp and row count may grow.
6. Fingerprint each source's ordered original IDs and **safe metadata only** with
   SHA-256. Raw command/output content is never selected or hashed. This is a
   recovery identity, not a forensic checksum of the original database; changes
   only to omitted text intentionally do not affect it.
7. Run destination FTS5 `integrity-check` with `rank=1` before and after inserting:
   this validates external-content consistency, not just FTS internal structure.
   It produces no historical text output. Source FTS schema and SQLite b-tree
   integrity are checked, but its old semantic index is not imported or repaired.
   Source semantic FTS consistency is not required to import safe metadata.
8. Insert every source record with plain `INSERT ... SELECT` (no `OR IGNORE`,
   `REPLACE`, updates, deletes, or metadata deduplication). Known FTS insert
   triggers populate the index. Verify each inserted count, total count, retained
   live-ID count, and final SQLite/FTS integrity before commit.
9. Commit rows and a new `aidev276_recovery` ledger atomically. Its fingerprint
   primary key and unique source slot record count and destination ID bounds.
   **Any existing ledger refuses every rerun**, even if source metadata changed.
   This small persistent table is intentional: it avoids treating legitimate
   metadata collisions as deduplication keys and makes uncertain-commit retries
   fail closed. Pith ignores the extra table. Do not delete it to force a rerun.
   A missing ledger with already imported history also fails the live time-bound
   assertion. Any failure before commit rolls back records, FTS and ledger.

No destructive restore is performed. Existing rows, IDs, commands and content are
not updated. Dry-run verifies the same operations against a consistent clone;
apply revalidates the actual live database after obtaining the write lock.

## Review and run (parent/operator only)

Use Python 3 with SQLite FTS5 support. No external packages or Pith executable are
needed. In PowerShell, from the worktree:

```powershell
Set-Location 'E:\pith-worktrees\aidev-276'
python -B -m unittest discover -s scripts -p 'test_recover_aidev276.py' -v
python -B scripts/recover_aidev276.py --dry-run
```

A successful dry-run prints only aggregate counts. Require `imported_rows=27748`
and `sources=2`; `rows_after` must equal `live_rows_before + 27748`. No row data,
fingerprints, commands, contents or paths are printed. An incompatible SQLite or
failed assertion exits nonzero with a generic refusal, deliberately suppressing
SQLite exception text that might contain stored values. Review locally without
dumping database rows or turning on unrestricted SQL tracebacks.

After independent review and successful dry-run, use a private, **local** directory
outside Git/sync/upload locations. Verify restrictive Windows ACLs before apply
(`0o600` is not sufficient to establish a Windows ACL). A full backup necessarily
retains any pre-existing live secrets/raw details; do not share, attach, or commit
it. The backup name below is unique per run; existing names are refused:

```powershell
$backupDir = 'C:\Users\zkrau\.pith\recovery-backups'
New-Item -ItemType Directory -Force -Path $backupDir | Out-Null
# Inspect/restrict ACLs locally before proceeding; do not upload backups.
$backup = Join-Path $backupDir ('aidev-276-before-' + [guid]::NewGuid().ToString('N') + '.db')
python -B scripts/recover_aidev276.py --apply --backup "$backup"
if ($LASTEXITCODE -ne 0) { throw 'Recovery refused; stop and review locally.' }
```

The success JSON describes counts **at commit**; subsequent live writes can
increase the destination count immediately. Preserve only that aggregate result
and backup location in the private operator record. Keep sources/backups until
recovery verification and retention review; never add database artifacts to Git.

Do not rerun apply after success. After failure/interrupt, stop: a backup can be
partial, and commit outcome can be uncertain if the client was interrupted after
commit. Do not replace the live database with the earlier backup: doing so would
lose newer records. Inspect only ledger/count/integrity metadata read-only and
obtain a reviewed follow-up plan. The script never automatically restores,
repairs FTS, cleans backups, removes a ledger, or retries an import after refusal.

Synthetic tests cover collision multiplicity, redaction/placeholders, exact live
record preservation, source immutability, read-only attachments, rollback of
rows/ledger/sequence/FTS, changed-source reruns, overlaps, count/range/schema/trigger
mismatches (with independent fixtures), refusal of a legal `sqliteXunexpected`
mutating trigger, inconsistent FTS, lock timeout, concurrent writes both before and
during import, WAL-backed online backup, deadlines including backup integrity
verification, backup non-overwrite, absent
inputs, high AUTOINCREMENT sequences and the actual 27,748-row import size.

## Prevent recurrence in future staged updates

A future staged updater must isolate **all three** `HOME`, `USERPROFILE`, and
`PITH_STORAGE` for **every** staged build/test/smoke Pith process, with private
throwaway home/storage and environment restoration afterward. Ensure staged
`config.json` cannot override `storage_path` to a real store: configuration JSON
is loaded after environment-derived defaults. Setting only
`PITH_STORAGE` is unsafe: `MigrateStorage` reads `os.UserHomeDir()/.pith`, copies
real files to the target and renames originals to `.bak`. A version/smoke command
must not be assumed side-effect free. This task does not install that updater.

For a separately authorized actual install using the original store, explicitly
set storage to the **exact native original path** `C:\Users\zkrau\.pith`, matching
Go's `filepath.Join(os.UserHomeDir(), ".pith")`, while leaving the intended real
home identity intact. Verify the effective `storage_path` in configuration agrees
with that exact native path; JSON can override the environment-derived default. `MigrateStorage` currently uses string equality, not
filesystem identity: slash/case variants that name the same directory are not a
safe substitute. Restore all environment variables after that operation. Do not
reuse the staged storage path for real installation, and do not run installation
or Pith smoke commands as part of this recovery.
