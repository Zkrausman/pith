# AIDEV-265 — current context-savings report after recovery

**Snapshot: 2026-09-16 17:07:56 UTC; latest included execution: 17:07:12 UTC.**

## Bottom line

Recorded net reduction is **3,312,476 estimated tokens (10.84%) overall**, **186,884 (11.25%) over the rolling seven days**, and **121,505 (10.98%) today**. These are observed accounting differences, **not tokenizer-measured, correctness-certified, or causally attributable savings**.

The post-v2.4.4 time cohort contains only **105 calls over about 83 minutes**, with **zero recorded reduction or expansion**: 104 passthrough calls and one unchanged `node` call. There are **no selected grep, Git-log, or chain calls** in that cohort. This does not establish a regression, prove the fixes effective in practice, or justify new compression. **More representative, consented post-fix exposure is needed.**

## Scope, recovery, and time semantics

- Database accessed exclusively with Python SQLite URI `file:C:/Users/zkrau/.pith/pith.db?mode=ro`, `PRAGMA query_only=ON`, and explicit read transactions. Only allowed metadata and aggregate results were queried. No command/content columns, application telemetry/migration APIs, Pith executable, or database copy were used. No arbitrary raw labels were emitted: labels are collapsed and models use snapshot-local aliases.
- **27,853 rows**, timestamp range **2026-08-11 06:33:00–2026-09-16 17:07:12**. The **27,748 rows before the supplied deployment boundary** plus 105 at/after it reconcile with the supplied recovery count. Recovery with command placeholders is supplied operational context, not independently verified by reading commands. **Historical command-level analysis is unavailable.** Do not reproduce the earlier audit's command-family/chain-shape queries against this recovered store.
- Actual schema reports `timestamp DATETIME DEFAULT CURRENT_TIMESTAMP`. Source inspection of `pkg/telemetry/telemetry.go` shows `Record` omits timestamp on insertion; SQLite `CURRENT_TIMESTAMP` is UTC. All 27,853 timestamp values are canonical 19-character, parseable date-time text without timezone suffix. Thus UTC comparison is appropriate for ordinary recorded rows. Recovery/import fidelity of historical timestamps cannot be independently certified from metadata alone; do not apply the local timezone again.
- Rolling seven days starts **2026-09-09 17:07:56 UTC**, relative to the audit clock, not the latest row. Today starts **2026-09-16 00:00:00 UTC**. Post-v2.4.4 starts **2026-09-16 15:44:22 UTC inclusive**, as supplied. Windows overlap and must not be added. “Post-v2.4.4” is a time proxy: rows do not establish the executing binary version.
- All token fields were non-null, nonnegative integers; no invalid/null passthrough flags or timestamps. A supplementary model query encountered a concurrent new row; it was rerun capped at the original maximum timestamp, restoring the same **27,853-row** cohort. Reproduction assumes no subsequent backdated inserts, edits, or deletions within that cap.

## Observed accounting

| Window | Calls | Original estimate | Retained estimate | Net reduction | Net / original |
|---|---:|---:|---:|---:|---:|
| Overall | 27,853 | 30,555,061 | 27,242,585 | 3,312,476 | 10.8410% |
| Rolling 7d | 2,320 | 1,660,854 | 1,473,970 | 186,884 | 11.2523% |
| Today UTC | 1,150 | 1,106,218 | 984,713 | 121,505 | 10.9838% |
| Post-v2.4.4 time cohort | 105 | 68,999 | 68,999 | 0 | 0% |

| Window | Reduced calls | Equal-estimate calls | Expanded calls | Gross reduction | Gross expansion |
|---|---:|---:|---:|---:|---:|
| Overall | 6,163 | 20,808 | 882 | 3,325,989 | 13,513 |
| Rolling 7d | 345 | 1,890 | 85 | 187,672 | 788 |
| Today | 151 | 934 | 65 | 121,978 | 473 |
| Post-v2.4.4 | 0 | 105 | 0 | 0 | 0 |

Net reduction = gross reduction − gross expansion. Equal estimates do not prove byte-for-byte identity. Current Pi hook uses **floor(Unicode rune count / 4)** (`pkg/pi/hook.go`, `runner.EstimateTokensWithHeuristic`); historical runner settings can differ. This is not an actual model tokenizer. Short nonempty output can estimate to zero. Totals sum retained estimates across calls: they are **cumulative retained-output occupancy**, not simultaneous context-window usage, unique context, repeated model exposure, money saved, or achievable future savings.

## Where retained output and recorded reductions concentrate

| Category | Overall calls / retained | Rolling-7d calls / retained | Today calls / retained | Post-fix calls / retained |
|---|---:|---:|---:|---:|
| Passthrough | 10,056 / 18,654,128 | 1,161 / 977,056 | 696 / 785,512 | 104 / 68,994 |
| Large passthrough (original ≥2,000) | 2,397 / 15,855,184 | 184 / 718,518 | 160 / 611,749 | 10 / 44,028 |
| `chain` | 8,229 / 5,272,698 | 528 / 284,082 | 176 / 67,602 | 0 / 0 |
| `grep` | 900 / 1,335,137 | 46 / 97,367 | 36 / 67,767 | 0 / 0 |

Large passthrough is a subset of passthrough. Chain/passthrough overlap is **zero in each window**. Rolling-7d passthrough accounts for **66.29%** of retained estimates and chain **19.27%**; **large passthrough plus chain = 1,002,600 (68.02%)**. These are investigation targets, **not a 68% savings opportunity**. Post-fix large passthrough is 44,028 estimates (63.81% of retained output), but metadata cannot tell which output is protected or safely reducible.

- Overall leading recorded net reductions: `get-content` **842,067 / 3,072 calls**, `env` **806,817 / 2,062**, `git_status` **498,490 / 906**; together **64.83%** of overall net reduction. Parser labels are accounting attribution, not correctness certification.
- Rolling-7d leading reductions: `git_status` **82,950 / 84 calls**, `get-content` **48,554 / 172**, `env` **24,162 / 88**, `windows-shell` **12,607 / 13**. The first two contribute **70.37%** of net reduction. This differs from the historical mix.
- Rolling-7d chain: **11 reduced, 507 equal-estimate, 10 expanded**; gross reduction **49**, expansion **13**, net **36**. Identity chain behavior, redaction, separate truncation, and historical versions confound attribution; do not infer successful constituent parsing.
- Grep: overall **590 expanded calls / 12,793 gross expansion / 68,094 net reduction**; rolling 7d **44 / 738 / −721**; today **34 / 436 / −419**. All rolling-7d expansion across parsers is only **788 estimates (0.0535% of retained output)**. Even eliminating it all would be a small retrospective ceiling, not demonstrated safe savings. The already-shipped grep gate is not a new opportunity.
- Positive-input/zero-retained estimates: **1,749 overall**, **60 rolling 7d**, **12 today**, **0 post-fix**. Git-log subset: **127 / 66,377 input estimates overall**, **9 / 6,087 rolling 7d**, **4 / 4,889 today**, **0 post-fix**. Prior unsupported-log correctness problems make historical nominal savings suspect, but flooring prevents treating these counts as proven content-loss incidents. No historical output was inspected.

## Source, harness, and model bias

Source and harness category aggregates happen to match exactly; this does not prove the raw fields match row-by-row. Other non-unknown values are suppressed, not printed.

| Window | Source/harness category | Calls | Original | Retained | Net reduction |
|---|---|---:|---:|---:|---:|
| Overall | Pi | 26,575 | 29,903,746 | 26,814,604 | 3,089,142 |
| Overall | Other | 1,226 | 603,705 | 380,399 | 223,306 |
| Overall | Unknown | 52 | 47,610 | 47,582 | 28 |
| Rolling 7d | Pi | 2,279 | 1,653,921 | 1,467,078 | 186,843 |
| Rolling 7d | Other | 41 | 6,933 | 6,892 | 41 |
| Today | Pi only | 1,150 | 1,106,218 | 984,713 | 121,505 |
| Post-fix | Pi only | 105 | 68,999 | 68,999 | 0 |

Pi supplies **99.53% of rolling-7d retained estimates**. Results are not representative of every harness. Explicit telemetry opt-outs now disappear from observation, so pre/post sampling is not necessarily comparable. Recovery, historical test/audit activity, configuration changes, repeated calls, and unknown selection mechanisms are additional biases; metadata cannot separate them.

Models are anonymized **M1–M8**, stable only within this capped cohort. Stored model attribution is not tokenizer validation or a controlled model comparison.

| Model alias | Overall calls / retained / net | Rolling-7d calls / retained / net | Today calls / retained / net | Post-fix calls / retained / net |
|---|---:|---:|---:|---:|
| M1 | 168 / 27,919 / 84,947 | — | — | — |
| M2 | 56 / 77,739 / 52,416 | — | — | — |
| M3 | 49 / 8,028 / 3,147 | — | — | — |
| M4 | 953 / 266,713 / 82,796 | 41 / 6,892 / 41 | — | — |
| M5 | 8,467 / 7,488,514 / 1,053,401 | 618 / 534,238 / 55,957 | 603 / 518,084 / 44,125 | 70 / 57,576 / 0 |
| M6 | 9,633 / 5,725,936 / 780,720 | 1,159 / 506,757 / 55,007 | 45 / 40,546 / 1,501 | — |
| M7 | 7,886 / 12,988,236 / 1,101,602 | — | — | — |
| M8 | 371 / 218,961 / 5,398 | 369 / 218,818 / 5,398 | 369 / 218,818 / 5,398 | 35 / 11,423 / 0 |
| Unknown | 270 / 440,539 / 148,049 | 133 / 207,265 / 70,481 | 133 / 207,265 / 70,481 | — |

Eight supplied model labels occur overall, four over seven days, three today, and two post-fix. Unknown-model calls contribute **70,481 of 186,884** rolling-7d net reduction, limiting model-specific interpretation.

## Actionable conclusion

1. **Observe rather than change compression now.** Repeat this metadata-only report after representative consented work reaches the relevant parser paths; separately report post-fix selected-parser and passthrough exposure. The current 105 calls provide no grep/log/chain acceptance evidence. Do not force telemetry opt-in to improve sample size.
2. **If separately approved, add bounded preservation-reason attribution**, not command/output retention: fixed enums for raw, failure, diff, JSON, upstream truncation, inspection, mixed syntax, no parser, disabled parser, unsupported format, and rejected non-reduction. This would distinguish protected versus potentially eligible passthrough. Present telemetry cannot quantify that split or a defensible achievable-savings target.
3. **Only then evaluate a narrow, synthetic, format-specific parser opportunity.** Preserve exact/ambiguous output and test semantics, not just shorter estimates. No generic chain compression, broad truncation, blanket grep disabling, or weakening preservation guards is supported here.

Reused `E:/pith-worktrees/aidev-274/docs/context-opportunity-audit.md`; its earlier command-family classifications are historical evidence only. `CHANGELOG.md` confirms grep non-expansion and Git-log/mixed-command preservation shipped in **2.4.3**, and explicit Pi telemetry opt-out in **2.4.4**. Source inspection confirms the grep gate and telemetry early return in `OptimizeHook`. Do not reopen these already-fixed defects as new opportunities. The earlier audit's recent cutoff was 13:59:41 and ended before this report; its 12.69% recent figure and 683 grep-expansion estimates are not directly comparable to this later rolling window.

## Exact aggregate queries and reproducibility

The following is the consolidated executable form of the aggregate queries used, with the original maximum timestamp substituted as the cap. Initial main queries used the snapshot clock upper bound `2026-09-16 17:07:56`; there were no rows between the maximum included timestamp and that clock in the original read snapshot. The model supplement was explicitly rerun with the maximum-timestamp cap. No query below reads commands or content; labels are filtered before emission.

```python
import sqlite3
c = sqlite3.connect('file:C:/Users/zkrau/.pith/pith.db?mode=ro', uri=True)
c.execute('PRAGMA query_only=ON')
c.execute('BEGIN')
end = '2026-09-16 17:07:12'
windows = [('overall', '0000-01-01 00:00:00'),
           ('rolling7d', '2026-09-09 17:07:56'),
           ('today', '2026-09-16 00:00:00'),
           ('post244', '2026-09-16 15:44:22')]
# Initial clock query: SELECT strftime('%Y-%m-%d %H:%M:%S','now')
# Rolling cutoff query: SELECT datetime(?,'-7 days')
# with parameter '2026-09-16 17:07:56'.
print(c.execute('''SELECT COUNT(*), MIN(timestamp), MAX(timestamp)
 FROM executions WHERE timestamp<=?''', (end,)).fetchone())
print(c.execute('''SELECT COUNT(*), SUM(timestamp IS NULL),
 SUM(datetime(timestamp) IS NULL),
 SUM(typeof(timestamp)='text' AND length(timestamp)=19
     AND timestamp=datetime(timestamp)),
 SUM(original_tokens IS NULL OR compressed_tokens IS NULL
     OR original_tokens<0 OR compressed_tokens<0),
 SUM(is_passthrough IS NULL OR is_passthrough NOT IN (0,1)),
 SUM(typeof(original_tokens)!='integer' OR typeof(compressed_tokens)!='integer')
 FROM executions WHERE timestamp<=? OR timestamp IS NULL''', (end,)).fetchone())
# Schema metadata was inspected with PRAGMA table_info(executions), emitting
# only timestamp/token/parser/source/harness/model/passthrough definitions.
stats = '''COUNT(*) AS n,
 COALESCE(SUM(original_tokens),0) AS original,
 COALESCE(SUM(compressed_tokens),0) AS retained,
 COALESCE(SUM(original_tokens-compressed_tokens),0) AS net,
 COALESCE(SUM(original_tokens>compressed_tokens),0) AS reduced_n,
 COALESCE(SUM(original_tokens=compressed_tokens),0) AS equal_n,
 COALESCE(SUM(original_tokens<compressed_tokens),0) AS expanded_n,
 COALESCE(SUM(MAX(original_tokens-compressed_tokens,0)),0) AS gross_reduction,
 COALESCE(SUM(MAX(compressed_tokens-original_tokens,0)),0) AS gross_expansion'''
parsers = """'bd','chain','ls','find','tree','du','git_status','git_log',
 'git_diff','git_branch','git_composite','git_show','gh_release','go','env',
 'docker_ps','dependencies','tests','go_cover','github','node','npm',
 'pith-internal','windows-shell','get-content','promptfoo','snag','source',
 'grep','minify','thneed','vitest','web-content','passthrough'"""
groups = {'parser': f'''CASE WHEN parser_used IN ({parsers}) THEN parser_used
 WHEN parser_used IS NULL OR parser_used='' THEN 'unset'
 ELSE 'other-parser' END'''}
for field in ('source', 'harness'):
    groups[field] = f'''CASE WHEN {field} IN
    ('pi','claude','gemini','codex','jules') THEN {field}
    WHEN {field} IS NULL OR {field}='' OR {field}='unknown' THEN 'unknown'
    ELSE 'other-{field}' END'''
groups['model'] = '''CASE WHEN model IS NULL OR TRIM(model)=''
 OR model='unknown' THEN 'unknown' ELSE 'supplied-model' END'''
checks = {
 'passthrough': 'is_passthrough=1',
 'large-passthrough': 'is_passthrough=1 AND original_tokens>=2000',
 'chain-pass-overlap': "parser_used='chain' AND is_passthrough=1",
 'chain-or-large-pass': "parser_used='chain' OR (is_passthrough=1 AND original_tokens>=2000)",
 'positive-zero': 'original_tokens>0 AND compressed_tokens=0',
 'git-log-positive-zero': "parser_used='git_log' AND original_tokens>0 AND compressed_tokens=0"
}
for w, lo in windows:
    where = 'timestamp>=? AND timestamp<=?'
    args = (lo, end)
    print(w, 'total', c.execute('SELECT '+stats+' FROM executions WHERE '+where, args).fetchone())
    for dim, expr in groups.items():
        print(w, dim, c.execute('SELECT '+expr+' AS category, '+stats+
          ' FROM executions WHERE '+where+
          ' GROUP BY category ORDER BY retained DESC', args).fetchall())
    for label, pred in checks.items():
        print(w, label, c.execute('SELECT '+stats+' FROM executions WHERE '+
          where+' AND ('+pred+')', args).fetchone())
    print(w, 'coverage', c.execute('''SELECT COUNT(DISTINCT CASE
      WHEN model IS NOT NULL AND TRIM(model)!='' AND model!='unknown'
      THEN model END), COUNT(DISTINCT CASE WHEN source IS NOT NULL
      AND source!='' AND source!='unknown' THEN source END),
      MIN(timestamp), MAX(timestamp) FROM executions WHERE '''+where, args).fetchone())
q = '''WITH base AS (
 SELECT timestamp, original_tokens, compressed_tokens,
 CASE WHEN model IS NULL OR TRIM(model)='' OR model='unknown'
      THEN 'unknown' ELSE model END AS m
 FROM executions WHERE timestamp<=:end
), labeled AS (
 SELECT *, DENSE_RANK() OVER (ORDER BY m) AS model_group FROM base
), windows(w,lo) AS (VALUES ('overall','0000-01-01 00:00:00'),
 ('rolling7d','2026-09-09 17:07:56'),('today','2026-09-16 00:00:00'),
 ('post244','2026-09-16 15:44:22'))
SELECT w, CASE WHEN m='unknown' THEN 'unknown'
              ELSE 'M'||model_group END AS model_alias,
 COUNT(*), SUM(original_tokens), SUM(compressed_tokens),
 SUM(original_tokens-compressed_tokens)
FROM labeled JOIN windows ON timestamp>=lo
GROUP BY w, model_alias ORDER BY w, COUNT(*) DESC'''
print(c.execute(q, {'end': end}).fetchall())
c.rollback()
c.close()
```

**Validation/scope:** aggregate partitions reconcile, net equals gross reduction minus gross expansion, and model-group totals reconcile with the capped snapshot. Read-only report only; no tests, installs, Squire, code changes, commits, tracker/wiki writes, or database mutations performed. Parent owns issue/wiki follow-up.
