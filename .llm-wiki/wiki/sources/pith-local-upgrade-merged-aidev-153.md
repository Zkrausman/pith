---
type: source
title: Pith local upgrade to merged AIDEV-153
status: insight
category: deployment
created: 2026-09-16
updated: 2026-09-16
slug: pith-local-upgrade-merged-aidev-153
---

# Pith local upgrade to merged AIDEV-153

Local installation upgraded from merged commit 09c3df7ede1ae73506467584d3fbf0e00916fe17 using a clean archive build. Both C:/Users/zkrau/.local/bin/pith.exe and C:/Users/zkrau/.pith/bin/pith.exe match SHA-256 107d42698f8b05473202ff474c0404001694672c0a7e9f7c989b6e0e92ab21ef. Installed Pi extension matches merged pkg/install/pi.go exactly. Focused Go tests/build succeeded; installed pi transform preserved Git rev-parse output and returned provenance fields. Backups at E:/pith-worktrees/pith-upgrade-1ta1B6/backup. Version label remains v2.4.0; hash/source commit identify upgrade. Existing Pi sessions need reload/restart for new callback; no hot reload was performed. See [[pith-provenance-count-semantics]].

*Category: deployment*

---
*Captured: 2026-09-16*

## Related

_Add links to related pages._
