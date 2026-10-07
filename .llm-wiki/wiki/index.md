---
okf_version: "0.2"
---

# Pith

## Concepts

- [Composite Git capture preservation](/concepts/composite-git-preservation.md)

- [Preservation-reason accounting](/concepts/preservation-reason-accounting.md)
- [Verified release publication](/concepts/verified-release-publication.md)

## Roadmap notes

- [Output-fidelity regression specification](../../docs/output-fidelity-regressions.md): pinned source observations and proposed CLI/Pi acceptance cases. Normal numeric CLI exit statuses, explicit raw truncation bypass, and honest Middle-Out omission counts are covered by compiled-entrypoint tests. Middle-Out counts a final newline as a terminator and retains actual blank lines. Raw retains the existing stdout-plus-stderr combination; independent stream fidelity, upstream-loss preservation, and the other proposed fixes remain pending.

- [EmbeddingGemma 2 feasibility](https://github.com/Zkrausman/Squire/blob/3335ba528b9af884c9ff17c6e6af40c43579a46e/docs/proposals/embedding-gemma-integration.md): proposed opt-in offline discovery; default synchronous inference remains a no-go under the negligible-latency requirement.

## Directories

- [sources/](sources/index.md)
