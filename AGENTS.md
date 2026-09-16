# Agent Onboarding - Pith

Pith is a high-performance Go CLI proxy that compresses command output before returning it to an LLM caller.

## Work Tracking and Knowledge

- **Linear is the authoritative issue tracker.** Create, update, and close Pith work in the `GeneralAiDev` team (AIDEV issues).
- **The LLM Wiki is the durable knowledge base.** Recall relevant context at task start; record meaningful decisions, discoveries, and completions when work ends.
- **GitHub pull requests** are the review and merge workflow. All changes must be committed to a feature branch and merged into `main` through a PR; never push directly to `main`.
- Do not use Beads (`bd`) or Dolt for Pith task tracking.

## Landing the Plane

Before ending a code-changing session:

1. Create Linear issues for remaining work and update the completed issue.
2. Run applicable tests, linters, and builds.
3. Commit work to a feature branch and open/update a GitHub PR. Do not commit or push directly to `main`.
4. Push changes: `git pull --rebase`, `git push`, then verify `git status` is up to date with origin.
5. Record the handoff and durable insights in the wiki.

## Versioning & Releases

- Use Semantic Versioning (`MAJOR.MINOR.PATCH`). Every PR that changes shipped code must update the binary version in `main.go` and add a matching entry to `CHANGELOG.md` before merge.
- Use a **patch** bump for backward-compatible fixes, refactors, and performance improvements; a **minor** bump for backward-compatible functionality; a **major** bump for breaking changes to supported behavior or interfaces. Minor bumps reset patch to zero; major bumps reset minor and patch to zero.
- Documentation-only and test-only changes are exempt. Embedded assets, generated integration code, and build/dependency changes that alter the shipped product are not exempt.
- Before merge, reconcile the proposed version with current `main` and existing release tags. Parallel PRs must not land conflicting versions; update the later PR's version and changelog after rebasing. Do not assume a version reserved on an unmerged branch is available.
- Every published release has a new, unique version. Its `vMAJOR.MINOR.PATCH` tag, binary-reported version, and changelog heading must agree. Never move or reuse a release tag, including one whose release workflow failed; use a new version for a corrected release.
- Merging a version bump does not authorize tagging, publishing, or installing. Publication and local installation remain separate, explicitly authorized actions. Publish only from a human-merged commit after applicable validation succeeds.
- Include the versioning decision in PR descriptions. These are contributor/reviewer requirements; do not claim automated enforcement unless corresponding CI checks exist and pass.

## Build & Test

- Binary: `pith.exe`
- Build: `go build -o pith.exe main.go`
- Test: `go test ./...`

## Conventions

- Parsers implement `pkg/parser/interface.go`.
- Telemetry is stored in `~/.pith/pith.db`.
- Pith integrates with LLM CLIs through hook configuration in their `settings.json` files.
