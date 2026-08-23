# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [2.4.0] - 2026-08-23
### Added
- **GitHub CLI (`gh`) Parser**: Compresses `gh pr list`, `gh issue search`, and other subcommands into lightweight markdown tables, drastically reducing JSON bloat from GitHub's raw output.
- **Node & NPM Parsers**: Safely truncates massive `npm install` and Node stack traces, preserving the critical head (error summary) and tail (stack execution) while stripping thousands of lines of intermediate download noise.
- **Git Show Integration**: `GitShowParser` added to explicitly compress `git show` outputs using the same algorithms as `git diff` and `git log`.

### Fixed
- **Git Parser Integrity**: Completely overhauled the `CompositeGitParser` state machine. It now flawlessly handles SSH signatures, GPG blocks, and `git log --stat` summaries without destroying commit context.
- **Git Diff Passthrough**: Abandoned naive diff whitelists in favor of blacklist passthrough. `Pith` now explicitly preserves `\ No newline at end of file`, file mode changes, renames, and all other extended git metadata while stripping ANSI color codes safely.
- **GitHub Parser Flag Tolerance**: Hardened `gh` parser positional logic to safely bypass global flags and values (e.g., `-R`, `--jq`), preventing false negatives on complex GitHub CLI queries.

## [2.3.2] - 2026-08-18
### Added
- **Antigravity Model Attribution:** `hook-pretool` now extracts `modelName` from the Antigravity hook payload and forwards `--model` to the Pith runner.
- **Model Pricing Resolution:** Added default token cost lookup for Gemini, Claude, and OpenAI model families with `--model-cost` override support.

## [2.3.1] - 2026-08-18
### Fixed
- **Telemetry Harness Normalization:** Added `antigravity` canonical harness mapping to `normalizeHarness` so stats and gains are properly categorized under the `antigravity` harness breakdown.

## [2.3.0] - 2026-08-18
### Added
- **Antigravity 2.0 & Antigravity CLI Integration:** Added `hook-pretool` subcommand implementing Antigravity's `PreToolUse` lifecycle hook specification to intercept and compress `run_command` output while preserving daemon and self-invocation guards.
- **Automated Antigravity Hook Installation:** Added `pith install --antigravity` to configure `~/.gemini/config/hooks.json` or `.agents/hooks.json` non-destructively.
- **PowerShell Runner Resilience:** Enhanced Windows runner execution to support PowerShell/pwsh statement chaining (`;`), cmdlets, and environment variable expansion.
- **Source Attribution:** Telemetry and gain reporting now tracks executions and token savings under the `antigravity` source.
- **Compiler Compatibility:** Added Windows GCC 16 CGo emutls linker compatibility for DuckDB static bindings.

## [2.2.5] - 2026-08-15
### Changed
- Use `Zkrausman/Pith` as the canonical public repository for updates, documentation, and support links.

## [2.2.4] - 2026-08-14
### Security
- Verify a Cosign-signed release checksum manifest before installing updates.

## [2.2.3] - 2026-08-14
### Security
- Verify the SHA-256 checksum from the release manifest before replacing the executable; refuse releases without a platform checksum.
- Require release assets to use the configured GitHub API host, limit update downloads, and avoid forwarding authentication tokens across redirects.

## [2.2.2] - 2026-08-11
### Fixed
- Ignore older or invalid release tags during update checks.
- Refuse to install from Go test executables.

## [2.2.1] - 2026-08-11
### Changed
- Model savings output now displays the recorded input cost per 1M tokens.

## [2.2.0] - 2026-08-11
### Added
- Model-aware savings telemetry records the active model and execution-time input-token rate.
- `pith stats` alias for model-level `pith gain` reporting.

### Changed
- Savings reports distinguish recorded pricing from clearly labeled fallback estimates.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [0.14.8] - 2026-05-01
### Fixed
- **CI Stability**: Resolved remaining "exec: 'cmd': executable file not found" errors on Linux runners by refactoring `pkg/runner/runner_extra_test.go` to be platform-aware.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [0.14.7] - 2026-05-01
### Fixed
- **Cross-Platform Execution**: Fixed a critical bug in the runner that hardcoded `cmd /c` for shell execution, which caused failures on Linux and macOS. Now correctly uses `sh -c` on non-Windows systems.
- **Test Stability**: Refactored the test suite (`main_test.go`, `runner_test.go`, etc.) to be cross-platform, resolving CI failures on Ubuntu runners.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [0.14.6] - 2026-05-01
### Added
- **Roadmap Enhancement**: Updated Phase 4 objectives in `docs/ROADMAP.md` to prioritize estate-wide synchronization.

### Fixed
- **Changelog Repair**: Fixed a corruption issue where the `v0.14.2` entry was duplicated dozens of times throughout the file.

### Changed
- **Estate-Wide Hygiene**: Synchronized versioning and coverage metrics to align with global standards.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [0.14.5] - 2026-04-30
### Added
- **Modernized Configuration TUI:** Completely rebuilt the `pith config` interface using **Bubble Tea**, **Bubbles**, and **Lip Gloss**.
    - Interactive parser toggling with real-time visual feedback.
    - Validated text inputs for numeric settings (MaxLines, HeadLines, TailLines, etc.).
    - Styled, bordered container with Pith's signature aesthetics.
    - Enhanced navigation with arrow keys and Vim-style (`j`/`k`) bindings.
- **Improved Installation Hygiene:** Automated hook synchronization for Gemini, Claude, and Codex during local installation.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [0.14.2] - 2026-04-27
### Changed
Maintenance & Compliance Release:
- Standardized binary hygiene across estate.
- Achieved 100% documentation compliance (Package READMEs + Mermaid diagrams).
- Infrastructure coverage sprint (50%+ target reached).
- Implemented Estate-Wide Compliance Auditor.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [0.14.1] - 2026-04-26
### Changed
- Estate-Wide Release Sweep: Synchronized versioning and updated dependencies.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.14.0] - 2026-04-23
### Added
- **Forensic Dashboard Search**: 
    - Introduced a full-text search capability in the Pith Dashboard.
    - Allows searching through months of history for specific commands, raw content, or optimized summaries.
    - Enables high-fidelity auditing of what context was actually provided to the agent.
- **Search API**: New `/api/search` endpoint in the telemetry engine.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.13.1] - 2026-04-23
### Added
- **Real-time Token Reporting**: The informational message displayed after optimization now includes the exact number of **tokens saved**.
    - Example: `Output optimized by Pith (parser: tests, tokens saved: 450, truncated: false)`

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.13.0] - 2026-04-23
### Added
- **Expanded Parser Coverage**:
    - **Snag Parser**: Specialized optimizer for `snag list --json`, stripping massive context snippets while preserving IDs and advice.
    - **Go Coverage Parser**: New optimizer for `go tool cover -func` that highlights low-coverage areas and strips 100% covered functions.
- **Financial Analytics & Precision**:
    - **USD Saved Tracking**: Pith now reports estimated financial savings in USD based on configurable token rates.
    - **Dynamic Dashboards**: The web dashboard now uses your specific model's cost rate for all calculations.
    - **Configurable Heuristics**: Added `USDPerMillionTokens` and `TokenHeuristic` settings for better multi-model accuracy.
- **Improved Extraction**: Robust command matching for PowerShell wrappers (`&`) and leading quoted paths.

### Changed
- **Token Logic**: Moved token estimation into a configurable `Runner` method, improving tracking precision for different LLMs.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.12.0] - 2026-04-23
### Added
- **High Fidelity Parsing**: Upgraded `SourceParser` and `TestParser` to be context-aware. 
    - `SourceParser` now preserves "high-signal" comments (BUG, TODO, FIXME, etc.) and avoids stripping critical developer intent.
    - `TestParser` is now robust against multi-line error outputs and stack traces, ensuring no critical debugging info is lost.
- **Reasoning Over Brevity Standard**: Updated `GEMINI.md` with a new engineering standard that prioritizes comprehensive technical reasoning over brevity, overriding global agent output constraints to prevent "intelligence loss."

### Fixed
- **Build-Only Interception**: `TestParser` no longer intercepts `go test -c` or build-only flags, ensuring compilation errors are visible and not optimized away.
- **Test Summary Detection**: Improved regex for aggregate test summaries to avoid capturing individual passing tests as false positives.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.11.3] - 2026-04-19
### Changed
- **Estate Modernization**: Updated repository status to align with the new **Squire** engineering assistant.
- **Squire Integration**: Optimized project metadata for high-signal discovery by Squire.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.11.2] - 2026-03-28

### Added
- **Thneed-Aware Architecture**: Added Mermaid diagrams to all READMEs for high-signal conceptual indexing.
- **Beads Closure Mandate**: Integrated `BEADS_STANDARD.md` and `check-beads.ps1` for task documentation enforcement.

### Improved
- **SCS-1 Compliance**: Enhanced `synapse-sync` to report "already up to date" status via exit code 2 for better orchestration feedback.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.11.1] - 2026-03-28

### Added
- **Synapse Compliance Standard (SCS-1)**: Implemented the hidden synapse-sync command to allow for automated orchestration via Synapse.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.11.0] - 2026-03-25

### Fixed
- **Token Tracking Feedback Loop:** Resolved a critical issue where `pith gain` would record its own output, leading to an exponential explosion in token counts and database size.
- **Improved Token Estimation:** Switched to `unicode/utf8.RuneCountInString` for token estimation to ensure accuracy in multi-byte (UTF-8) environments.
- **Universal Truncation:** Enforced middle-out truncation for ALL intercepted outputs, including unparsed commands caught by hooks, preventing massive unoptimized outputs from consuming LLM context.
- **Runner Test Suite:** Fixed a compilation error in `pkg/runner/runner_test.go` caused by recent function renames.

### Added
- **Intelligent Agent Detection:** The Pith runner now automatically detects the calling agent (Gemini CLI or Claude Code) via environment variables, ensuring accurate source tracking even when not explicitly specified by the hook.
- **Tech Planning Infrastructure:** Initialized the `techPlanning` folder to centralize architectural roadmaps and implementation designs.

### Changed
- **Cost Basis Update:** Updated the dashboard's cost-saving calculation to $0.50/M tokens to align with modern frontier model pricing (e.g., Gemini 1.5 Flash / Claude 3 Haiku).
- **Gain Command Optimization:** Limited `pith gain` to display only the top 20 commands and top 10 discovery opportunities, ensuring a concise summary that doesn't bloat logs.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.10.0] - 2026-03-24

### Added
- **Configurable Storage:** The telemetry and configuration storage path is now fully configurable. Pith defaults to `E:\TheBrain\PithBackup` on Windows to centralize cross-agent memories.
- **Automatic Migration:** Existing `pith.db` and `config.json` files are automatically migrated to the new storage location upon the first run of the new version.
- **Thneed Parser:** Introduced a specialized parser for `thneed.exe query` output. It elegantly compresses large JSON response bodies while preserving critical node metadata and content snippets.
- **NPM Parser:** New optimizer for `npm install` and `npm run` commands that filters out verbose progress bars and audit boilerplate.
- **Thneed-Aware Architectural Standard:** Added Mermaid diagrams to all package-level `README.md` files to optimize GraphRAG indexing and agent navigation.

### Improved
- **Git Parser Extension:** The `git_status` parser now also handles `git add`, `git commit`, and `git push`, ensuring a clean, token-efficient developer experience across the entire branch workflow.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.9.4] - 2026-03-22

### Fixed
- **Removed non-functional Antigravity support.** The VSCode-based Antigravity agent does not honor `.gemini/settings.json` hooks, so the `run_command` / `send_command_input` matchers and `CommandLine` schema support have been removed.

### Added
- **Agent Source Tracking:** Pith now records which agent (Gemini CLI, Claude Code, Codex) triggered each command, enabling per-agent cost savings analysis.
- **Dashboard Agent Filter:** The web dashboard (`pith dashboard`) includes a dropdown to filter all metrics by agent source.
- **`ls` Parser Optimization:** Removed file permissions from `ls -l` output to further reduce token usage.
- **Installer Hook Overwrite:** Re-running `pith install` now correctly updates existing hooks instead of silently skipping them.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.9.3] - 2026-03-22 [YANKED]

### Added
- **Agent Source Tracking:** Pith now records which agent (Gemini CLI, Claude Code, Codex) triggered each command, enabling per-agent cost savings analysis.
- **Dashboard Agent Filter:** The web dashboard (`pith dashboard`) now includes a dropdown to filter all metrics by agent source.
- **`ls` Parser Optimization:** Removed file permissions from `ls -l` output to further reduce token usage.

### Fixed
- **Installer Duplication:** Fixed a bug where the `pith install` command would print duplicate success messages when installing multiple hooks for the same CLI.
- **Installer Hook Overwrite:** Re-running `pith install` now correctly updates existing hooks instead of silently skipping them.
- **Repository Hygiene:** Removed the `.gemini` folder from version control that was accidentally committed, ensuring user configurations remain purely local.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.9.2] - 2026-03-22

### Added
- **Parser Generator Skill:** Added a new `pith-parser-generator` AI agent skill for creating new Pith parsers.

### Changed
- **Repository Cleanup:** Cleaned up version control by ignoring and removing compiled binaries (`pith.exe`, `pith.exe.old`) from tracked files.

### Fixed
- **Testing:** Fixed a syntax error involving duplicate package declarations and imports in `pkg/parser/fs_test.go` that prevented tests from running.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.9.1] - 2026-03-22

### Fixed
- **Cross-Platform Command Matching:** Fixed a bug where Windows backslashes in paths were not correctly handled on Linux CI runners, ensuring `MatchCommand` works reliably in all environments.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.9.0] - 2026-03-22

### Added
- **PowerShell Get-Content Parser:** Dedicated optimizer for `Get-Content` output, specifically targeting large JSON files (like session logs) with minification and truncation of massive fields.
- **Cross-Platform Command Matching:** Introduced a robust `MatchCommand` helper that automatically recognizes commands regardless of Windows extensions (`.exe`, `.cmd`, `.bat`, `.ps1`) or full file paths.
- **Full Test Coverage:** Achieved 100% test coverage across all 20+ specialized parsers with new comprehensive test suites for `MatchCommand` and `GetContentParser`.

### Improved
- Updated `VitestParser`, `BDParser`, `PowerShellParser`, and `PromptfooParser` to leverage the new robust command matching logic, ensuring higher hit rates on Windows.
- Standardized all `CanParse` implementations to use slice-based argument matching for better accuracy.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.8.0] - 2026-03-20

### Added
- **Dedicated Tool Parsers:**
    - `VitestParser`: Strips decorative lines and focuses on failed tests and summary statistics for Vitest.
    - `BDParser`: Compresses massive help output and long issue lists from the Beads (bd) issue tracker.
    - `PromptfooParser`: Minifies evaluation tables and progress bars from promptfoo.
- **Enhanced PowerShell Parsing:** Improved `Get-ChildItem` (ls/dir) optimization by removing redundant mode/date columns and preserving only essential file/size info.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.7.0] - 2026-03-19

### Added
- **Dashboard v2.0:**
    - **Bento Grid Layout:** Completely overhauled the analytics interface with a modern, responsive grid of cards.
    - **Efficiency Gauge:** Interactive SVG ring showing the overall compression ratio at a glance.
    - **Live Activity Stream:** Real-time ticker showing recently executed commands, including parser status (Hit/Miss) and savings per command.
    - **Pith vs. Raw Deep Dive:** Side-by-side comparison modal that allows users to verify Pith's optimization by viewing the raw output alongside the compressed version.
- **Telemetry Improvements:**
    - Added support for recording full original and compressed output content (required for the Deep Dive feature).
    - Database migration to add `original_content` and `compressed_content` columns to the `executions` table.
- **GUI Endpoints:**
    - New `/api/recent` and `/api/execution` API endpoints to power the live dashboard and detailed views.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.6.0] - 2026-03-19

### Added
- **New Parsers:**
    - `WebParser`: Optimizes `curl`, `wget`, and `Invoke-WebRequest` by minifying JSON and extracting HTML titles.
    - `PithParser`: Internal optimizer for Pith's own help and log outputs.
    - `PowerShellParser`: Cleans up verbose Windows PowerShell and CMD headers while preserving file details.
    - `GoParser`: Targeted optimization for `go version`, `go build`, and module listings.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.5.9] - 2026-03-18

### Changed
- **Rebrand:** Renamed the project from **Diet** to **Pith** (skip the peel, get to the pith).
- **Module Update:** Updated Go module name to `pith` and updated all internal imports.
- **Paths:** Changed default configuration and telemetry directory from `~/.diet` to `~/.pith`.
- **Log Files:** Renamed `diet.log` to `pith.log` and `diet.db` to `pith.db`.
- **Hooks:** Updated CLI hooks to use the new `pith` command and `pith-optimizer` name.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.5.8] - 2026-03-17

### Improved
- **Intelligent Hook Installation:** The `pith install` command now defaults to **global** installation for all supported CLIs (Gemini, Claude, and Codex) if no flags are provided.
- **JSON Configuration Merging:** Installer now safely merges Pith hooks into existing `settings.json` files instead of skipping them. It correctly preserves existing hooks (like `bd prime`) and custom settings (like `security` or `ui`).
- **Safety Backups:** Automatically creates a `settings.json.bak` backup before any modification.
- **Duplicate Prevention:** Improved logic to detect and avoid duplicate hook entries during installation.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.5.7] - 2026-03-17

### Added
- **Hook Support:**
    - Support for **Claude Code** CLI via `.claude/settings.json` (using `PostToolUse` event).
    - Support for **Codex** (and compatible agents) via `.codex/settings.json` (using `AfterTool` event).
    - New `--claude` and `--codex` flags for `pith install`.
    - Generic `setupHook` internal helper for easier future hook additions.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.5.6] - 2026-03-16

### Added
- **New Parsers:**
    - `SourceParser`: Optimizes `cat` and `type` output by stripping comments and excessive whitespace while preserving code structure.
    - `GitHubReleaseParser`: Optimizes `gh release view/list` by extracting key metadata (Tag, Title, Assets) and removing redundant SHAs.
    - `ChainParser`: Enables optimization of shell-chained commands (e.g., `cmd1; cmd2 && cmd3`) by recursively matching individual parts.
- New test cases and `promptfoo` evaluations for the new parsers.

### Fixed
- Fixed `dashboard` command being incorrectly proxied as a target command.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.5.5] - 2026-03-16

### Fixed
- Fixed `pith update` to correctly identify and download platform-specific binaries (e.g., `pith-windows-amd64.exe`) from GitHub releases.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.5.4] - 2026-03-16

### Added
- **Interactive Dashboard:** Introduced `pith dashboard`, a web-based GUI that provides a visual overview of token savings, command breakdowns, and opportunity discovery using Chart.js.
- New `GetStatsByDay` method in telemetry to support historical analysis in the dashboard.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.5.3] - 2026-03-16

### Added
- Added a GitHub Actions workflow to automate Go testing on all pushes and pull requests.
- Integrated automated cross-platform releases; pushing a version tag now triggers a build for Windows, Linux, and macOS (amd64 and arm64) and creates a GitHub Release with the binaries attached.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.5.2] - 2026-03-16

### Improved
- Improved the `gain` and `discover` commands to display helpful empty-state messages when no telemetry data has been recorded yet, rather than printing empty tables.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.5.1] - 2026-03-16

### Fixed
- Fixed an issue where commands intercepted by the Gemini CLI hook were not being logged to the Snag log. Pith now correctly parses exit codes from the hook payload to ensure accurate behavioral tracking.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.5.0] - 2026-03-16

### Added
- **Snag Integration:** Pith now actively exports a plain text log file specifically formatted for the [Snag](https://github.com/zkrau/snag) behavioral learning tool. Every intercepted command, its output (truncated to the last 50 lines to prevent bloat), and its exit code are appended to `~/.pith/pith.log` using the `[CMD]` and `[EXIT]` syntax required by Snag's collector.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.4.9] - 2026-03-16

### Improved
- Overhauled `install` command to copy Pith binary to a permanent location (`~/.pith/bin`) and add that to the system `PATH`.
- Updated CLI hooks to use the permanent binary location, making them resilient to source directory changes.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.4.8] - 2026-03-16

### Fixed
- Fixed Gemini CLI hook to correctly handle `Output: ` and `Error: ` prefixes in tool responses, ensuring parsers like `git_status` work correctly when called via hooks.

### Changed
- Updated `promptfooconfig.yaml` with more comprehensive evaluation cases for all major parsers.
- Updated `tests/prompts.txt` to be more flexible for different evaluation tasks.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.4.7] - 2026-03-16

### Fixed
- Improved `GitHubParser` to correctly handle `gh repo list` and other list variants.
- Fixed `Runner` to correctly split command arguments when proxied.
- Updated `Runner` to capture combined stdout/stderr for more reliable parsing.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.4.6] - 2026-03-16

### Added
- New `GitHubParser` to optimize `gh issue list`, `gh pr list`, and `gh release list` outputs.

### Fixed
- Improved `LsParser` reliability across different `ls -l` and `dir` variants.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.4.5] - 2026-03-16

### Improved
- Balanced `LsParser` and `MinifyParser` to preserve critical context for LLMs while still saving tokens, specifically addressing issues seen in other CLI optimizers (RTK-AI/rtk#582).
- `LsParser` now keeps file mode and size when available.
- `MinifyParser` now strips comments but preserves line structure.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.4.4] - 2026-03-16

### Added
- New `CompositeGitParser` to optimize shell-joined Git commands (e.g., `git status & git log`).
- Support for shell execution in the runner, enabling optimization of complex command strings.

### Improved
- Updated `README.md` with new features and updated parser counts.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.4.3] - 2026-03-16

### Fixed
- Updated `_hook` command to handle the latest Gemini CLI hook payload schema (specifically supporting the `tool_input` object instead of the older `tool_call_request.arguments` JSON string).
- Improved robustness of hook command to support multiple schema versions.

### Added
- Improved `.gitignore` to include `.beads/` and `test_hook_input.json`.

## [2.0.0] - 2026-05-02
- Estate-wide synchronization surge to v2.0.0.

## [v0.4.2] - 2026-03-16

### Added
- Initial support for token-optimized CLI proxying.
- Parsers for `git status`, `git log`, `ls`, and others.
- Middle-out truncation for large outputs.
