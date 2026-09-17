# Pith: Token-Optimized CLI Proxy

[![CI and Release](https://github.com/Zkrausman/Pith/actions/workflows/test.yml/badge.svg)](https://github.com/Zkrausman/Pith/actions/workflows/test.yml)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](https://github.com/Zkrausman/Pith/blob/main/LICENSE)

`pith` is a high-performance CLI tool that intercepts terminal command outputs, compresses them, and filters out noise to save tokens for LLMs (like Claude Code, Gemini CLI, etc.).

## 🚀 Features
- **Zero-Overhead Proxy:** Fast Go-based binary for near-instant command interception.
- **Smart Parsers:** 20+ specialized optimizers for common developer commands, now featuring **High Fidelity Parsing** for source code and test results.
- **Reasoning Over Brevity Standard:** Built-in mandate that prioritizes comprehensive technical depth over brevity to prevent LLM intelligence loss.
- **Configurable Storage:** Telemetry and configuration are stored locally under `~/.pith` by default (or `PITH_STORAGE_PATH` if set).
- **Composite Commands:** Intelligent handling of shell-joined commands (e.g., `git status & git log`).
- **Escape Hatch (`pith raw`):** Bypass all parsers when you need the exact, unformatted truth.
- **Middle-Out Truncation:** Automatically keep the start and end of massive outputs, removing the redundant middle.
- **Multi-LLM Integration:** Automated hook setup for Antigravity 2.0, Antigravity CLI, Gemini CLI, Claude Code, Codex, and Pi.
- **Telemetry & Discovery:** Tracks exact token savings and identifies new optimization targets.
- **Interactive Configuration:** Toggle parsers and adjust truncation limits via `pith config`.
- **Interactive Dashboard:** Launch a local web-based analytics dashboard via `pith dashboard`.

## 📦 Installation
1. Download the matching asset from [Releases](https://github.com/Zkrausman/Pith/releases) and verify it with the release `checksums.txt`.
2. Run the installer (defaults to global install for all CLIs). On Windows use `pith-windows-amd64.exe`; on Linux/macOS use the matching executable:
   ```bash
   ./pith install
   ```
   *Note: Pith safely **merges** hooks into your existing `hooks.json` and `settings.json` files and automatically creates a `.bak` backup before any modification. It will NOT overwrite your existing configurations or other hooks.*
3. Restart your terminal.

---

## 🛠️ Power User Features

### 1. Targeted Installation
If you only want to install hooks for a specific CLI or only for the current project:
- **Antigravity 2.0 / CLI:** `pith install --antigravity --global`
- **Local Install (current folder):** `pith install --gemini`
- **Global Install (specific CLI):** `pith install --claude --global`
- **Full Global Sync:** Use `pith install` to ensure all system-wide hooks and settings are up-to-date.

### 2. The Escape Hatch (`pith raw`)
If a parser is being too aggressive and you need to see the raw, bit-for-bit output of a command, prefix it with `raw`:
```bash
pith raw git diff
```
This bypasses all logic and returns the original system output. 

> **Note:** This is intended as a **temporary workaround**. If a specific parser consistently causes issues, you can permanently disable it using the **Parsers Page** in `pith config`.

### 3. Middle-Out Truncation
When a command returns thousands of lines (like a massive log file), Pith prevents context overflow by keeping the most important parts: the **beginning** (setup/context) and the **end** (errors/results).

### 4. Pi output provenance
Pi transforms completed tool results only; Pith never reruns the original command. Host markers such as `[Showing lines 1-... of ... Full output: ...]` are preserved and reported as upstream truncation, including any safe host artifact reference. Pith's own omission markers say **minimized** and report omitted volume in the output text. Transform response `original*` and `retained*` fields count input and final rendered bytes/lines (newline-separated segments, including a trailing empty segment). `omitted*` is exact source omission and is zero when source correspondence is unknown (including parser prose or redaction). Parser markers additionally expose `parserNetReductionKnown` and net line/byte reduction, excluding the marker and redaction; these values are not source omission claims. Parser strategies describe rendered output, and only parsers that emit a minimization marker report that net reduction.

Recognized failures, warnings, final test summaries, diffs, valid JSON values (objects, arrays, and scalars), and Git `status --porcelain`, `worktree list --porcelain`, and `rev-parse` output bypass minimization without an additional size cap. This conservative policy also protects bounded results above the normal compression threshold. Redaction is mandatory in the Pi hook and can reduce retained bytes without being minimization. Pith does not retain raw output by default; retrieve lost host output only through the existing host-provided artifact reference when one is present (Pith cannot restore output already lost upstream). Use `pith raw` when exact command output is required.

### ⚙️ Interactive Configuration (`pith config`)
Pith features a modern, multi-page TUI built with **Bubble Tea** to manage your optimization engine. Use **`Tab`** to switch between the **Parsers** and **Settings** pages.

#### 1. Parsers Page
Toggle individual command optimizers on or off. If a parser is causing issues with a specific command, you can disable it here without affecting the rest of the system.

#### 2. Settings Page (Output Management)
This page controls **Middle-Out Truncation** and analytics precision.

| Setting | Default | Description |
| :--- | :--- | :--- |
| **`MaxLines`** | `500` | The threshold for truncation. Snipping occurs only if a command's output exceeds this total line count. |
| **`HeadLines`** | `100` | The number of lines to preserve at the **top** of the output. This ensures the LLM understands the command's setup, headers, or initial context. |
| **`TailLines`** | `100` | The number of lines to preserve at the **bottom**. This is critical for capturing final results, exit codes, and error stack traces. |
| **`USD/M Tokens`** | `3.00` | The cost rate (USD per 1 Million Input Tokens) used to calculate financial savings in the dashboard. Adjust this to match your preferred model (e.g., $3.00 for Claude 3.5 Sonnet). |
| **`Heuristic`** | `4.00` | Average characters per token. This is used purely for estimation in analytics and does not affect the actual compression logic. |

---

## 🛠️ Supported Parsers
Pith currently optimizes the following commands:
- **Git:** `status`, `log`, `diff`, `branch`, and **composite commands** (e.g., `git status & git log`)
- **PowerShell:** `Get-Content` (JSON minification & truncation), `Get-ChildItem` (ls/dir minification)
- **Filesystem:** `ls`, `dir`, `find`, `tree`, `du`, `where`
- **Text:** `grep`, `rg`, `cat`, `type` (JSON/XML/CSS/HTML minification)
- **Infra/Dev:** `docker ps`, `npm list`, `pip list`, `npm test`, `go test`, `pytest`, `vitest`, `bd` (Beads), `gh` (GitHub), `env`, `set`

> **Note:** Pith features **cross-platform command matching**, meaning it automatically recognizes commands regardless of their extension (`.exe`, `.cmd`, `.bat`, `.ps1`) or whether they are called via a full path.

---

## 📊 Compression Examples

### 1. `git status`
**Raw Output (~80 tokens):**
```text
On branch main
Your branch is up to date with 'origin/main'.

Changes to be committed:
  (use "git restore --staged <file>..." to unstage)
	new file:   main.go

Changes not staged for commit:
  (use "git add <file>..." to update what will be committed)
  (use "git restore <file>..." to discard changes in working directory)
	modified:   pkg/parser/git.go
```

**Pith Output (~25 tokens):**
```text
new file:   main.go
modified:   pkg/parser/git.go
```

### 2. `git log`
**Raw Output (~120 tokens):**
```text
commit 3f7d7f7b12345678
Author: Zachary Krausman <zkrausman@gmail.com>
Date:   Sun Mar 15 22:49:38 2026 -0400

    Update: Display changelog during pith update

commit 1cd63311f8330490
Author: Zachary Krausman <zkrausman@gmail.com>
Date:   Sun Mar 15 22:30:00 2026 -0400

    Initial commit
```

**Pith Output (~35 tokens):**
```text
3f7d7f7 | Zachary Krausman | Mar 15 2026 | Update: Display changelog
1cd6331 | Zachary Krausman | Mar 15 2026 | Initial commit
```

### 3. `ls -l`
**Raw Output (~50 tokens):**
```text
total 8
-rw-r--r--  1 user  group  123 Mar 15 22:30 main.go
-rw-r--r--  1 user  group  456 Mar 15 22:31 README.md
```

**Pith Output (~10 tokens):**
```text
main.go README.md
```

---

## 🧪 Testing & Quality
Pith uses [promptfoo](https://promptfoo.dev/) to evaluate the performance and quality of its token-optimization strategies.

- **Status:** **Work In Progress (WIP)** 🛠️
- **Run evaluations locally:** `npx promptfoo eval` (generated reports are intentionally not committed).

## Security, privacy, and supported platforms

Pith releases are currently built and tested for **Linux amd64**, **Windows amd64**, and **macOS arm64**. Unsupported platforms are not offered release binaries.

Maintainers: see the [verified release publication and failure-triage runbook](docs/releases.md).

The dashboard binds to loopback by default and is intended for the local machine only. Diagnostic logs are disabled by default; update/network behavior and retention limits are described in [PRIVACY.md](PRIVACY.md). Release dependency inventory and license evidence are in [THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md) and the attached CycloneDX SBOM.

“Pith” is an independent project name and is not affiliated with, endorsed by, or sponsored by any LLM provider, terminal client, or third-party dependency named here.

## 📊 Analytics Dashboard
Pith includes an interactive web dashboard to visualize your token savings and discover new optimization opportunities.

- **Launch:** `pith dashboard`
- **Visualization:** Powered by Chart.js for real-time analytics.
- **Ecosystem:** Pith is **[BuildABoard](https://github.com/Zkrausman/BuildABoard) compatible**. The included `board.json` allows you to seamlessly integrate your Pith analytics into the broader BuildABoard dashboard ecosystem.

---

## ⌨️ CLI Usage
- `pith <command>`: Proxy any command manually.
- `pith config`: Toggle parsers on/off.
- `pith gain`: View total tokens saved.
- `pith discover`: See unparsed commands costing you tokens.
- `pith reset --all`: Clear all telemetry data.
- `pith update`: Auto-update to the latest version.


## Architecture

<!-- mermaid-start -->
```mermaid
graph TD
    Main[main.go] --> Config[pkg/config/config.go]
    Main --> Runner[pkg/runner/runner.go]
    Main --> Telemetry[pkg/telemetry/telemetry.go]
    Main --> Install[pkg/install/install.go]
    Main --> Parser[pkg/parser/pith.go]
    
    Runner --> Parser
    Runner --> Telemetry
    
    Config --> TUI[pkg/config/tui.go]
    
    Parser --> GitParser[pkg/parser/git.go]
    Parser --> NPMParser[pkg/parser/npm.go]
    Parser --> BDParser[pkg/parser/bd.go]
    
    Telemetry --> DB[(pith.db)]
    
    Dashboard[pkg/gui/gui.go] --> Telemetry
    Dashboard --> Config
```
<!-- mermaid-end -->
