---
type: source
title: Pith provenance count semantics
status: insight
category: testing
created: 2026-09-16
updated: 2026-09-16
slug: pith-provenance-count-semantics
---

# Pith provenance count semantics

AIDEV-153 distinguishes rendered original/retained counts, exact source omission, and parser marker-excluded net reduction. Redaction must not be counted as omission; arbitrary parser prose does not provide source correspondence. Hook preservation must run before parser dispatch, not only in standalone PiOptimize. Executable generated-extension tests mock spawn and exercise the callback to verify transform-only execution and host-detail preservation. Full tests and temporary build passed; independent targeted final review returned OK. Baseline GUI vet errors are tracked as AIDEV-268. See [[obs-2026-09-16-pi-minimization-provenance]].

*Category: testing*

---
*Captured: 2026-09-16*

## Related

_Add links to related pages._
