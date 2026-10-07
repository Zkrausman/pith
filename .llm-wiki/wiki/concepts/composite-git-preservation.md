# Composite Git capture preservation

Composite Git and `git show` validate each commit header with the existing strict
GitLog parser before emitting a summary. A supported record has a full SHA-1 or
SHA-256 hash, default author and valid default date, a blank separator, and one
nonempty subject. A following section must be another commit, a Git diff, or a
human status header. Unsupported records and boundaries return the entire original
capture, including preceding sections, whitespace, and ANSI bytes. This prevents
shortened dates from panicking and incomplete records from gaining invented fields.

Multiline messages, merge/signature metadata, decorated/custom output, CRLF records,
and stat-only suffixes conservatively pass through. Non-commit `git show` output
(including blobs and annotated tags) also passes through. Recognized simple
commit/diff output retains existing compression. GitStatus inventories and general
parser dispatch are unchanged. This does not establish that upstream capture is
complete or change ordinary runner truncation limits.

Pi retains mandatory redaction even when preserving an unsupported capture, and
unchanged GitShow results retain passthrough provenance. Shell-composite Pi commands
remain protected by the existing shell guard.

Synthetic regressions live in `pkg/parser/git_composite_fidelity_test.go`,
`pkg/pi/hook_git_show_test.go`, and the packaged-binary CLI fallback fixtures in
`main_parser_fallback_test.go`. No real source/provider/benchmark runs are needed.
The patch version is v3.0.6; a merge is not a tag, release, or installation.
