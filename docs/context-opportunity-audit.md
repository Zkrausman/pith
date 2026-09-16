# AIDEV-272 — bounded context-opportunity audit

Audit snapshot: **2026-09-16 13:59:57 UTC**. Branch `aidev-272-context-audit`; baseline `ffcab2c08d23ff65f427a24057512c9beaddf813`.

The audit below records baseline observations, not a post-change savings claim. The approved follow-up implements only the grep post-redaction non-expansion gate and regression tests. Generic chain compression, Git log correctness, and telemetry consent changes remain out of scope.

## Decision summary

**Do not implement generic chain compression or large-output truncation from this evidence.** Large passthrough and chain outputs dominate observed context occupancy, but telemetry cannot establish which bytes can safely be removed. Correctness takes priority over compression.

**Approved bounded context-reduction change (implemented):** a grep-only, post-redaction non-expansion acceptance gate in `pkg/pi/hook.go:118–130`, inside `OptimizeHook`, before replacing the initial redacted passthrough response. Compare the candidate with that redacted passthrough, not with unredacted source. Accept only a strictly smaller estimated-token representation with no byte growth; otherwise keep the existing passthrough response. Do not relax any preservation guard or enabled-parser setting. This gate is implemented; it is not a correctness certification of GrepParser or a claim of measured deployment savings.

The quantified upside is modest: recent grep rows expanded by **683 gross estimated tokens**, offset by 117 tokens of reductions, for **566 net added tokens**. Avoiding all 683 is only **0.0533%** of recent retained occupancy, and remains a retrospective ceiling, not proven achievable savings: redaction, historical versions, and unknown output formats confound replay. A gate cannot recover missing search semantics; malformed/unsupported grep formats need lossless fallback before broader rollout.

**Correctness prerequisite for broader parser work:** investigate destructive dispatch, particularly GitLogParser handling one-line output and whole mixed-command output. Synthetic characterization confirms that a successful mixed command beginning with a one-line git log can produce an empty result. Nine recent git-log rows have positive input estimates and zero retained estimates; this is a warning signal, not proof of nine real losses because token estimates floor short strings and no historical output was inspected.

## Scope and privacy

- Read `AGENTS.md`; followed this task's explicit no-commit/no-publication/no-tracker-write restrictions.
- Existing database opened only through Python `sqlite3.connect` with a `file:` URI ending in `?mode=ro` and `uri=True`, with `PRAGMA query_only=ON` and a read transaction.
- Never opened the existing database through application migration APIs; did not modify it, copy it, query output-content columns, retain raw outputs, or print historical command strings, arbitrary parser labels, model names, credentials, or per-execution identifiers.
- Commands were inspected transiently in Python solely to emit fixed family/shape categories; no command corpus was retained. Labels not on an explicit allowlist were collapsed.
- Baseline audit added synthetic tests; the approved follow-up adds only the grep acceptance gate and associated tests. Hook tests always supply `t.TempDir()` because the hook currently records telemetry even when `TelemetryEnabled` is false. Those isolated fixture databases are not the user's database.
- No Squire, installation, activation, commit, push, PR, Linear write, or wiki write. No production commands from telemetry were executed.

## Quantitative observations

“Tokens” below mean stored estimates, not model-tokenizer measurements. Current hook estimator is `floor(Unicode rune count / 4)` (`pkg/runner/runner.go:398`); historical runner configurations may differ. “Occupancy” is the sum of recorded retained estimates across calls, **not simultaneous context-window occupancy**. It excludes prompts and other tool output, and does not establish unique context, repeated exposure, dollar savings, output usefulness, or parser correctness.

The read snapshot contained **27,457 rows**, spanning `2026-08-11 06:33:00` through `2026-09-16 13:59:41` (database timestamp values). Maximum row ID was **27458**, used as a reproducibility high-water mark. No null or negative token fields were found.

Recent means the seven days ending at the snapshot's maximum timestamp: inclusive cutoff **2026-09-09 13:59:41**, not seven days relative to the wall clock. First observed recent row was `2026-09-09 14:51:53`. Latest-1,000 means final 1,000 rows ordered by ID; first timestamp `2026-09-10 22:14:38`.

| Window | Calls | Original estimate | Retained estimate | Net reduction | Reduction / original |
|---|---:|---:|---:|---:|---:|
| All | 27,457 | 30,272,995 | 26,966,325 | 3,306,670 | 10.9228% |
| Recent 7 days | 2,107 | 1,468,887 | 1,282,477 | 186,410 | 12.6906% |
| Latest 1,000 | 1,000 | 939,766 | 802,174 | 137,592 | 14.6411% |

The earlier approximately 11% lead is supported for the all-history window. It is not a forecast for a new parser.

### Recent context concentration

| Recorded category | Calls | Original | Retained | Net saved | Share of recent retained |
|---|---:|---:|---:|---:|---:|
| `is_passthrough=true` | 874 | 735,871 | 735,848 | 23 | 57.3771% |
| Large passthrough: original >=2,000 | 149 | 568,340 | 568,318 | 22 | 44.3141% |
| `parser_used=chain` | 616 | 349,579 | 349,546 | 33 | 27.2555% |
| `parser_used=grep` | 42 | 88,753 | 89,319 | -566 | 6.9646% |

Large passthrough is a subset of passthrough; do not add those two rows. Chain/passthrough overlap is zero in this recent snapshot. Large passthrough plus chain therefore occupies **917,864 estimated tokens (71.57%)**, but this is **not a savings estimate**. Only 17 chain calls meet the >=2,000 threshold; they account for 66,390 retained tokens. Much chain occupancy comes from many smaller calls.

Pi harness accounts for 2,066 recent calls, 1,461,954 original and 1,275,585 retained estimates. All 149 large passthrough rows and all 42 grep rows are Pi. Pi-only chain: **609 calls, 346,153 original, 346,120 retained, 33 net saved**. The supplied lead of 610 chain calls cannot be reproduced exactly under this fixed window; the near-identical effect is supported without treating that prior count as ground truth.

### Large passthrough by safe first-executable family

| Family | Recent calls | Retained estimate |
|---|---:|---:|
| Git | 63 | 226,723 |
| Other / unrecognized / wrapped | 55 | 155,439 |
| Search | 10 | 98,035 |
| Python | 14 | 64,486 |
| Node/package tools | 4 | 14,916 |
| Text tools | 1 | 3,887 |
| GitHub CLI | 1 | 2,657 |
| Filesystem | 1 | 2,175 |
| **Total** | **149** | **568,318** |

These are lexical categories, not claims about output semantics or shell execution. Quoted paths with spaces, environment prefixes, wrappers, and setup commands can land in “other” or the wrapper's family. Git-heavy occupancy can include diffs, exact inspection results, errors, JSON, or other protected output. No eligibility percentage can be inferred.

All-history large passthrough: 2,362 calls, 15,704,984 retained estimates. Dominant families: other 7,509,704; Git 4,090,763; Node/package tools 2,066,608; search 718,704. Recent mix differs substantially, so prioritize recent evidence rather than all-history totals alone.

### Chain effectiveness and attribution

- Recent chain: 593 equal-sized, 11 reduced, 12 expanded calls; gross expansion 16, net reduction 33 estimates (**0.0094% of chain input**).
- Latest 1,000: 178 chain calls, 78,802 original, 78,769 retained, also 33 net reduction.
- All-history chain: 8,193 calls, 5,297,902 original, 5,264,867 retained, 33,035 net reduction. This cannot be attributed to today's ChainParser.
- Current `ChainParser.Parse` is identity. Hook redaction can change length; runner middle-out truncation is separate; historical code/configuration can differ. A “chain” parser label is not proof that constituent dispatch or compression succeeded.
- Recent chain strings contain `&&` in 190 calls, `|` in 153, and `;` in 428. Categories overlap and can include quoted literals. These are not proven shell-chain counts.

### Grep expansion

| Window | Grep calls | Expanded calls | Gross expansion | Net saved |
|---|---:|---:|---:|---:|
| All | 893 | 583 | 12,706 | 68,181 |
| Recent | 42 | 39 | 683 | -566 |
| Latest 1,000 | 34 | 32 | 400 | -383 |

All-history grep is net beneficial despite many expansions. Blanket disabling sacrifices genuine repeated-filename grouping benefits. Current `GrepParser.Parse` emits file headers and indentation even when most files have just one hit; a synthetic three-file fixture grows from **18 bytes / 4 estimated tokens** to **31 bytes / 7 estimated tokens**. A repeated-long-filename fixture demonstrates the opposite direction.

## Code inspection and safety constraints

Line references in baseline observations refer to the audited commit; function names identify the stable seams.

### Pi hook

`pkg/pi/hook.go:102` (`OptimizeHook`) transforms completed output and does not execute the supplied command. Before parser selection it preserves raw bypass, nonzero exits, error markers, warnings, final test summaries, upstream-truncation markers, exact Git inspection commands, valid JSON including scalars, and diff markers (`pkg/pi/pioptimize.go:82`, `mustPreserveOutput`). Preservation is lossless **except mandatory redaction**. Broad regex matches can legitimately suppress compression; their false-positive rate cannot be measured without output inspection, which this audit intentionally forbids.

The hook splits command text with `strings.Fields`, not a shell lexer. Registry order picks the first matching parser. At the audit baseline it accepted parser output regardless of growth, emptiness, or output-format recognition. The approved gate now rejects grep growth and non-reduction; other parsers and format recognition are unchanged. Returning the initialized passthrough response on rejection is the narrow no-expansion seam; it also avoids falsely claiming parser minimization. Do not compare to raw unredacted source or accidentally bypass redaction when rejecting.

`OptimizeHook` unconditionally opens telemetry at line 133; `HookRequest.TelemetryEnabled` does not gate that call, unlike `PiOptimizeWithConfig`. This is a source-inspected consent/test-isolation risk, not changed here. `telemetry.Record` clears both content fields and redacts command metadata. Do not add raw-output telemetry to resolve audit uncertainty.

Current parser-net-reduction metadata is separate from exact source omissions. Do not reinterpret a shorter parser representation, a marker, or redaction as exact omitted source bytes/lines.

### Chain dispatch

- `pkg/parser/chain.go:14`: any `;|&` character matches, including inside quoted arguments. `Parse` at line 19 is identity. `SplitSubCommands` at line 23 uses string splitting, not shell semantics.
- `pkg/parser/interface.go:41`: ChainParser is last. Earlier parsers can consume the entire combined output based on the first command.
- `pkg/pi/hook.go:113–130`: no call to `SplitSubCommands` and no output segmentation. A setup-prefixed Git command can fall through to identity chain, while a Git-leading chain can reach GitLogParser directly.
- `pkg/runner/runner.go:245–276`: runner searches subcommands, but sets `p=cp`, not the discovered constituent parser; later `cp.Parse` remains identity. Its middle-out truncation is a different mechanism. Do not conflate this runner path with the hook path.
- `CompositeGitParser.CanParse` (`pkg/parser/git.go:180`) expects `cmd` to contain `git ` plus separators, but hook passes only the first whitespace token; simply reordering that parser is not a sound fix.

Applying the last parser, first parser, or every parser to aggregate output is unsafe: pipelines alter formats; earlier commands may contribute critical stdout/stderr; `&&` and `||` change which commands run; quoted operators, substitutions, heredocs, redirections, and shell wrappers have different semantics. HookRequest has one output and one exit code, with no per-command provenance. General chain compression needs trustworthy output boundaries or a proven narrow command-shape contract; telemetry supplies neither.

### Other parsers and false savings

- `pkg/parser/text.go:15`: GrepParser uses colon splitting, strips whitespace/empty lines, and lacks command-mode context. Windows drive letters, context separators, count/list-only output, headings, and NUL formats need exact synthetic coverage before any semantic rewrite. JSON is already guarded at the hook, but that does not cover every structured format. Smaller output does not imply correct output.
- `pkg/parser/git.go:71`: GitLogParser recognizes multi-line commit blocks, not arbitrary git-log formats. A one-line log fixture returns empty. Recent positive-input/zero-estimate git-log observations therefore merit correctness investigation before celebrating their nominal reductions.
- `pkg/parser/source.go`: comment regexes are not language-aware and can alter string literals; do not generalize source stripping to unknown passthrough.
- `pkg/parser/powershell.go`: shell parser truncates generic output, and GetContentParser matches broad substrings; wrapper normalization can expose unsafe matches rather than simply unlock compression.
- `pkg/parser/fs.go`: listing parsers use whitespace heuristics and some hard limits; generic large-output dispatch to them is not lossless.

This was a targeted inspection of hook/registry/chain/grep and adjacent representative implementations, not a complete certification of every parser.

## Ranked opportunities

Ranking is by safe next implementation value, not dollar savings or optimistic volume.

1. **Bound grep expansion at hook acceptance.** Exact seam: `pkg/pi/hook.go`, `OptimizeHook`, after `parsed := maybeRedact(rawParsed, cfg)` and before assigning parser response metadata. Initially scope to `candidate.Name()=="grep"`; retain redacted original unless candidate strictly reduces the existing estimate and does not increase bytes. Keep all preservation checks intact. Historical ceiling: 683 recent gross expansion estimates, not 89,319 grep occupancy and not guaranteed savings. Acceptance tests must exercise beneficial grouping, expansion fallback, equal-estimate behavior, Unicode, mandatory redaction, disabled parsers, and preserved output. Implemented in the approved follow-up; historical savings remain unproven.
2. **Fix destructive dispatch/unsupported Git log formats before chasing chain volume.** Exact seams: `pkg/pi/hook.go` before first-match parser selection; `pkg/parser/git.go`, `GitLogParser.CanParse/Parse`; `pkg/parser/chain.go` only after command grammar requirements are approved. Fail closed on unsupported/mixed formats rather than discard unknown lines. The fixture demonstrates a real baseline failure mode; telemetry does not identify its historical incidence. Correctness may increase retained context; that is acceptable. Do not sell this as savings.
3. **Investigate large passthrough with bounded reason metadata, not output retention.** Exact source seam: guard decision in `OptimizeHook` and `mustPreserveOutput`. A separately approved change could distinguish raw/failure/diff/JSON/upstream/inspection/no-parser/disabled outcomes using a fixed enum and aggregate counts, without commands or contents. Current 568,318 recent retained estimates are occupancy, not compressible volume. Determine protected versus eligible volume before choosing a specific parser. No schema/API change is authorized by this audit.
4. **Improve chain identity attribution before constituent compression.** Exact seam: `ChainParser.Parse` and hook parser-result acceptance/telemetry attribution. Identity should not be read as successful minimization. This improves evidence, not context size. Chain-only observed occupancy is 349,546 recent estimates; defensible achievable savings remain unknown.

**Approved implementation scope:** item 1 only as a small context non-regression gate, with item 2 deferred for separate tracking as a correctness follow-up/prerequisite to broader dispatch changes. Do not authorize a generic large-passthrough or chain compression feature from aggregate occupancy. Actual achievable savings require approved synthetic, format-specific evaluation; this audit intentionally cannot replay historical outputs.

## Synthetic tests and validation

Added `pkg/pi/context_audit_test.go`:

- `TestContextAuditChainDispatchCharacterization`: setup-prefixed chain identity, quoted operator matching, and Git-leading mixed-output erasure. These assertions document today's behavior, including defects; they are not requirements to preserve those defects after a fix.
- `TestContextAuditGrepExpansionFallback`: single-hit-per-file expansion falls back through the hook; direct parser fixtures still demonstrate expansion and beneficial repeated-long-filename grouping.
- `TestContextAuditHookPreservationBoundaries`: failure, error marker, warning, summary, diff, structured JSON, upstream truncation, raw bypass.

Added `pkg/pi/hook_grep_test.go`: beneficial grouping, expansion fallback, equal-estimate fallback, Unicode estimates, redaction growth/reduction, disabled grep, full response metadata, and fallback telemetry provenance. All new hook calls use temporary storage.

Validation passed:

```text
go test ./pkg/pi -run '^(TestContextAudit|TestOptimizeHookGrep)' -count=1
go test ./... -count=1
go vet ./...
go build -o <temporary-directory>/pith.exe main.go
git diff --check
```

Path resolution was inspected first: `telemetry.NewTelemetry("")` uses `os.UserHomeDir()`; configuration also supports `PITH_STORAGE` and a separate machine-specific fallback. Full tests, vet, and build therefore ran with `HOME` and `USERPROFILE` pointing to the same temporary home, `PITH_STORAGE` to temporary storage, and `APPDATA`/`LOCALAPPDATA` to temporary directories. Existing Go caches were retained explicitly through `GOCACHE`, `GOMODCACHE`, and `GOPATH`. The compiled binary and temporary validation storage were removed afterward. New focused tests independently use `t.TempDir()`. No installation, activation, commit, or push was performed.

## Reproducible aggregate methodology

1. Open the fixed URI read-only, enable query-only, start a read transaction. Inspect schema only to locate metadata columns; never select content fields.
2. Record high-water ID and timestamp bounds; use `id<=27458` to reconstruct this snapshot's cohort. The live database can receive unrelated writes; exact reproduction assumes existing rows were not subsequently edited/deleted. The audit does not export a database snapshot.
3. Select only timestamp, command, token counts, parser label, passthrough flag, and harness. Immediately classify commands to fixed first-executable families; discard command strings after classification. Do not print rows or dynamic labels.
4. Aggregate calls, original/retained estimates, signed net reduction, expansion/reduction/equality counts, and gross positive expansion. Large means original estimate >=2,000 (an analysis threshold, not exact 8,000 bytes). Keep negative net reductions visible.
5. Use recent cutoff and latest-ID window specified above; report all-history separately. Do not sum overlapping populations or equate parser-selected with output-changed.

Pass an absolute telemetry database path as the first argument when running this script (read-only; no content access). The following standalone Python reproduces the main tables without application APIs or content access. It prints only fixed labels and aggregated counts. All values in this report came from equivalent read-only queries; the supplemental queries below reproduce the targeted cross-checks.

```python
import collections
import json
import re
import sqlite3
import sys
from pathlib import Path

URI = Path(sys.argv[1]).resolve().as_uri() + '?mode=ro'
CAP = 27458
CUTOFF = '2026-09-09 13:59:41'
families = {
    'git': 'git', 'rg': 'search', 'grep': 'search', 'ripgrep': 'search',
    'python': 'python', 'python3': 'python', 'py': 'python',
    'pwsh': 'shell', 'powershell': 'shell', 'bash': 'shell',
    'sh': 'shell', 'cmd': 'shell', 'go': 'go',
    'node': 'node', 'npm': 'node', 'npx': 'node',
    'pnpm': 'node', 'yarn': 'node',
    'cat': 'file-read', 'type': 'file-read', 'get-content': 'file-read',
    'sed': 'text-tools', 'awk': 'text-tools',
    'head': 'text-tools', 'tail': 'text-tools',
    'ls': 'filesystem', 'dir': 'filesystem', 'find': 'filesystem',
    'tree': 'filesystem', 'du': 'filesystem', 'gh': 'github',
    'curl': 'web', 'wget': 'web', 'pith': 'pith',
}
# Collapse everything except the fixed labels relevant to these tables.
parsers = {'', 'chain', 'grep', 'git_log'}
harnesses = {'pi', 'claude', 'gemini', 'codex', 'jules', 'unknown'}

def family(command):
    words = (command or '').lstrip().split(None, 1)
    name = (words[0].strip(chr(34) + chr(39))
            .replace(chr(92), '/').rsplit('/', 1)[-1].lower()
            if words else '')
    return families.get(re.sub('[.](exe|cmd|bat|ps1)$', '', name), 'other')

def stats(rows):
    return dict(
        n=len(rows), original=sum(r[2] for r in rows),
        retained=sum(r[3] for r in rows),
        net_saved=sum(r[2]-r[3] for r in rows),
        expanded_n=sum(r[3]>r[2] for r in rows),
        gross_expansion=sum(max(0, r[3]-r[2]) for r in rows),
        reduced_n=sum(r[3]<r[2] for r in rows),
        equal_n=sum(r[3]==r[2] for r in rows))

c = sqlite3.connect(URI, uri=True)
c.execute('PRAGMA query_only=ON')
c.execute('BEGIN')
print('bounds', c.execute('''SELECT COUNT(*), MIN(timestamp),
    MAX(timestamp), MAX(id) FROM executions WHERE id<=?''', (CAP,)).fetchone())
rows = []
for ts, cmd, orig, comp, parser, passthrough, harness in c.execute('''
    SELECT timestamp, command, original_tokens, compressed_tokens,
           parser_used, is_passthrough, harness
    FROM executions WHERE id<=? ORDER BY id''', (CAP,)):
    rows.append((ts, family(cmd), orig or 0, comp or 0,
                 parser if parser in parsers else 'other-parser',
                 bool(passthrough),
                 harness if harness in harnesses else 'other-harness'))
    del cmd
for label, data in [('all', rows), ('latest1000', rows[-1000:]),
                    ('recent7d', [r for r in rows if r[0]>=CUTOFF])]:
    print(label, json.dumps(stats(data)))
    for category in ('chain', 'grep', 'git_log'):
        print(category, json.dumps(stats([r for r in data if r[4]==category])))
    print('pi', json.dumps(stats([r for r in data if r[6]=='pi'])))
    print('passthrough', json.dumps(stats([r for r in data if r[5]])))
    large = [r for r in data if r[5] and r[2]>=2000]
    print('large-passthrough', json.dumps(stats(large)))
    groups = collections.defaultdict(list)
    for r in large:
        groups[r[1]].append(r)
    for name, group in sorted(groups.items()):
        print(name, json.dumps(stats(group)))

checks = {
    'chain-pi': "parser_used='chain' AND harness='pi'",
    'grep-pi': "parser_used='grep' AND harness='pi'",
    'large-pass-pi': "is_passthrough=1 AND original_tokens>=2000 AND harness='pi'",
    'chain-pass-overlap': "parser_used='chain' AND is_passthrough=1",
    'large-chain': "parser_used='chain' AND original_tokens>=2000",
    'git-log-zero-estimate': "parser_used='git_log' AND original_tokens>0 AND compressed_tokens=0",
}
for label, predicate in checks.items():
    print(label, c.execute('''SELECT COUNT(*), SUM(original_tokens),
        SUM(compressed_tokens), SUM(MAX(compressed_tokens-original_tokens,0))
        FROM executions WHERE id<=? AND timestamp>=? AND ''' + predicate,
        (CAP, CUTOFF)).fetchone())
print('chain-shape', c.execute('''SELECT COUNT(*), SUM(compressed_tokens),
    SUM(CASE WHEN command LIKE '%&&%' THEN 1 ELSE 0 END),
    SUM(CASE WHEN command LIKE '%|%' THEN 1 ELSE 0 END),
    SUM(CASE WHEN command LIKE '%;%' THEN 1 ELSE 0 END)
    FROM executions WHERE id<=? AND timestamp>=? AND parser_used='chain' ''',
    (CAP, CUTOFF)).fetchone())
print('invalid-token-fields', c.execute('''SELECT COUNT(*) FROM executions
    WHERE id<=? AND (original_tokens IS NULL OR compressed_tokens IS NULL
    OR original_tokens<0 OR compressed_tokens<0)''', (CAP,)).fetchone()[0])
c.close()
```

## Remaining limits

Telemetry lacks retained output, command-output segmentation, preservation reason, parser version, and actual tokenizer measurements. The audit cannot certify causal savings, classify historical protected output, or derive a defensible compression ratio for chain/passthrough. The existing low apparent savings on some categories is not permission to weaken safety. Parent should decide any code change and publication after reviewing the fixtures and these limitations.
