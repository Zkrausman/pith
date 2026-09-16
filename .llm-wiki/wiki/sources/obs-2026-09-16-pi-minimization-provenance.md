---
type: source
title: "Pi minimization provenance"
tags:
  - pith
  - pi
  - minimization
status: observation
created: 2026-09-16
updated: 2026-09-16
slug: obs-2026-09-16-pi-minimization-provenance
relevance: high
---

# Pi minimization provenance

Pi transform responses distinguish recognized upstream host/tool truncation from Pith minimization. Upstream markers are preserved and reported with `upstreamTruncated`. Pith omission markers use “minimized” with volume. Original/retained counts describe input and final rendered output; parser marker-excluded net reductions are reported separately from exact source omissions. Zero exact-omission fields for parser prose are not proof of losslessness. Redaction is mandatory in the hook, including raw bypass, and is not counted as minimization.

Git porcelain status/worktree and rev-parse results, valid JSON values, failure output, warnings, and final test summaries bypass minimization (subject to redaction). Unknown and disabled-parser results remain passthrough. The generated Pi integration only spawns `pith pi transform`; host details including exit status, truncation metadata, and existing artifact paths survive.

Retrieval guidance: use an existing safe host-provided full-output artifact when supplied. Pith creates no new raw-output retention and cannot restore content already omitted upstream. Behavioral tests cover the generated callback, preservation predicates, UTF-8/count boundaries, and parser marker forms. Full Go tests and an isolated build passed; vet has three pre-existing unreachable-code diagnostics in `pkg/gui/gui.go`.
