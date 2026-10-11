# Codex direct-command parser coverage

The adapter wraps only bounded literal external argv after resolving its
executable. Pith receives the explicit argv and dispatches through the existing
parser registry. The Go regression `TestDirectArgvParserCoverageMatrixMatchesRegistry`
requires one row for every registered parser, checks its representative command
against `CanParse`, and exercises the actual selected parser, strict reduction,
protected raw disposition, or unsupported raw disposition with synthetic
output. The companion priority regression checks JSON minification, `npm test`
parser ordering, and `GetContentParser` command-identity matching.

The table gives one representative disposition per registered parser.
`routed` means the existing parser is eligible for bounded literal argv and
must still return a strict reduction; `protected raw` means a fidelity guard
keeps the captured stdout unchanged; `unsupported` means the representative
output has no safe reduction. Parser output can still be protected by failure,
warning, diff, truncation, structured-output, redaction, or configuration
guards. Shell compounds, expansions, redirections, assignments, shell
builtins, interpreters, recursive wrappers, alias-enabled shells, and commands
with uncertain executable resolution stay with Codex's original shell
command. A parser that overlaps a known lossy command is never allowed to
catch that command after the safety guard rejects it.

| Registry parser | Representative argv | Disposition | Reason |
|---|---|---|---|
| `git_composite` | `git status; git diff` | protected raw | Shell compound stays with Codex. |
| `git_status` | `git status --short` | protected raw | Rename rows with `->` and NUL-delimited rename rows stay raw. |
| `git_log` | `git log` | routed | Complete default-format records may be summarized. |
| `git_show` | `git show HEAD` | unsupported | Opaque object content has no safe reduction. |
| `git_diff` | `git diff` | protected raw | Diff markers bypass minimization. |
| `git_branch` | `git branch` | routed | Branch inventory can be summarized. |
| `ls` | `ls -l` | protected raw | Filename whitespace and newlines are ambiguous. |
| `find` | `find .` | routed | Bounded result summaries may be selected. |
| `tree` | `tree` | routed | Tree glyph compaction may be selected. |
| `du` | `du -h` | routed | Only well-formed records above the row limit are summarized. |
| `grep` | `rg -n needle src` | routed | Only strict reductions are selected. |
| `minify` | `cat data.json` | routed | One JSON file only; minify is the only eligible parser. |
| `source` | `cat source.go` | protected raw | Preserves comments and source text. |
| `env` | `env` | protected raw | Recursive environment wrapper stays with Codex. |
| `docker_ps` | `docker ps` | routed | Container inventory can be summarized. |
| `gh_release` | `gh release list` | routed | Release table can be summarized. |
| `github` | `gh status` | routed | Status output can be summarized. |
| `dependencies` | `npm list` | routed | Dependency tree can be summarized. |
| `tests` | `go test -v ./pkg/parser` | routed | Only the validated complete Go success profile is transformed; other final summaries stay raw. |
| `go_cover` | `go tool cover -func coverage.out` | routed | Recognized coverage profile can be summarized. |
| `web-content` | `curl -s https://example.invalid/data` | routed | Plain text or HTML may be summarized; JSON stays structured. |
| `pith-internal` | `pith raw git status` | protected raw | Prevents recursive Pith execution. |
| `windows-shell` | `powershell -Command Get-Content` | protected raw | Interpreters stay with Codex. |
| `get-content` | `cat source.go` | protected raw | Only the single JSON-file minify case is eligible. |
| `go` | `go version` | unsupported | Short version output has no reduction. |
| `vitest` | `vitest run` | routed | Bounded success summaries may be selected. |
| `bd` | `bd list` | routed | Recognized issue list can be summarized. |
| `promptfoo` | `promptfoo eval` | routed | Recognized evaluation summary can be selected. |
| `snag` | `snag find` | routed | Bounded search results may be selected. |
| `node` | `node app.js` | protected raw | Interpreters stay with Codex. |
| `npm` | `npm run build` | routed | Recognized package manager output may be selected. |
| `chain` | `git status; git diff` | protected raw | Shell compound stays with Codex. |

`cat` and `type` on non-JSON files stay raw so `SourceParser` and
`GetContentParser` cannot strip or reinterpret source content. The JSON
exception is limited to exactly one file argument and does not fall through to
other parsers. Any selected response still needs complete valid UTF-8, a
registered nonempty parser name, exact original and retained byte counts, and
strict byte and rune reductions; otherwise the captured stdout is delivered
unchanged. The private recorder accepts only fixed command-family labels and
the actual registry parser name, and stores aggregate counts without output or
argv content. At the parser layer, `SourceParser` and `LsParser` now return
identity output when they lack language or listing-profile context; the legacy
hook and runner record those results as passthrough, while independent
truncation remains a separate transformation.
