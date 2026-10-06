# Pith Roadmap

## Phase 1: Knowledge Aggregation (Complete)
- [x] Implement High Fidelity Parsing for source and tests.
- [x] Establish "Reasoning Over Brevity" standard.
- [x] Implement Snag log parser (snag list --json).
- [x] Implement Go coverage report parser (go tool cover).
- [x] Improve command extraction for PowerShell & quoted paths.

## Phase 2: Token Economics (Complete)
- [x] Stabilize token tracking and cost analytics.
- [x] Expand parser coverage for common developer tools.

## Phase 3: Advanced Optimization (Complete)
- [x] **Semantic Compression:** Move beyond heuristic-based truncation to LLM-driven summarization for ultra-high-value outputs.
- [x] **Multi-Agent Sync:** Centralize Pith telemetry across multiple machines/users for enterprise-wide token savings.
- [x] **Deep-Dive Search:** Add full-text search across all recorded command executions in the dashboard.

## Phase 4: Predictive Analytics (New)
- [x] **Anomaly Detection**: Use local DuckDB models to bridge Overseer and flag behavioral failures. (pith-lcw): Use local DuckDB models to flag unusual token usage or execution patterns.
- [x] **Context ROI Heatmap**: Visualize which injected files consistently result in "Hits" vs. "Misses" to optimize `whet` injection logic.
- [x] **Predictive Cost Advisor**: Estimate the total session cost based on current roadmap complexity.

## Output Fidelity Roadmap

Source-reviewed priorities and deterministic acceptance fixtures are in the [output-fidelity regression specification](output-fidelity-regressions.md). These are proposed engineering tasks; publishing this roadmap does not mark the behavior implemented or verified.

- [ ] P0: preserve child numeric exit status at the shipped CLI boundary
- [ ] P0: preserve stdout/stderr meaning and complete late failure diagnostics
- [ ] P1: retain complete changed-file inventories and exact test counts
- [ ] P1: disclose exact omissions and make raw output lossless
- [ ] P1: preserve unsupported output without inventing success or coverage

## Embedding research (proposed)

The [EmbeddingGemma 2 feasibility proposal](https://github.com/Zkrausman/Squire/blob/3335ba528b9af884c9ff17c6e6af40c43579a46e/docs/proposals/embedding-gemma-integration.md) recommends only a separately authorized, opt-in offline/shadow discovery experiment using explicit public/synthetic or approved samples. Default synchronous inference is a no-go under the negligible/sub-millisecond overhead requirement. Current telemetry does not retain an output corpus; existing output-fidelity contracts remain prerequisites for any future content-changing mode. This research does not establish shipped embedding support or authorize implementation, model installation/inference, new capture or benchmarks.

## Maintenance & Stabilization
- [x] **Fix cross-platform CI failures and shell execution (pith-btj)**
- [x] **Release v0.14.6 (pith-ksi)**
- [x] **Release Surge v2.0.0 (Pith-surge)**
- [x] **Pith Optimization: Semantic Compression (pith-8e8)**
- [x] **Implement Autonomous Root Cause Forensics (--diagnose) (pith-o7c)**
- [x] **Binary Hygiene: Standardize .gitignore and clean up artifacts (pith-ppz)**
- [x] **Binary Hygiene: Standardize .gitignore and clean up artifacts (pith-zqn)**
- [x] **Binary Hygiene: Standardize .gitignore and clean up artifacts (pith-1l2)**
- [x] **v0.11.0: Fix token tracking feedback loops and update cost basis (pith-bgf)**
- [x] **Implement PowerShell Get-Content parser (pith-bhx)**
- [x] **Increase code coverage to 75% (pith-a4t)**
- [x] **Enforce Thneed-Aware Architectural Standards (pith-wth)**
- [x] **Implement Beads (bd) CLI parser (pith-os4)**
- [x] **Implement Vitest parser (pith-4uf)**
- [x] **Continue parser discovery for high-impact commands (pith-0d3)**
- [x] **Pith vs. Raw Deep Dive (pith-wg4)**
- [x] **Live Activity Stream (pith-lcw)**
- [x] **Bento Grid Layout & Efficiency Gauge (pith-5ju)**
- [x] **Rename project from Diet to Pith (pith-bph)**
- [x] **Reach 95% total code coverage (pith-1mj)**
- [x] **Implement PowerShell Get-ChildItem parser (pith-41w)**

