---
type: source
title: Pith v2.4.1 release gate
status: insight
category: release
created: 2026-09-16
updated: 2026-09-16
slug: pith-v241-release-gate
---

# Pith v2.4.1 release gate

AIDEV-269 prepares v2.4.1 after AIDEV-153. Version is a constant in main.go. .github/workflows/test.yml publishes on v*.*.* tag push only after tests and native Linux amd64, Windows amd64, macOS arm64 builds; release includes SBOM, checksums and signature. Version bump requires human PR merge before tagging and upgrading from published assets. Local go test ./..., temporary build, version smoke test and diff check passed. Earlier installed source build still reports v2.4.0; see [[pith-local-upgrade-merged-aidev-153]].

*Category: release*

---
*Captured: 2026-09-16*

## Related

_Add links to related pages._
