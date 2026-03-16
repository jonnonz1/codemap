# codemap

Code map your repo so AI coding agents find the right files on the first try.

Codemap indexes every file in your repository with a one-line summary, then uses a cheap fast model to select only the files relevant to a task. Instead of your agent spending 10+ tool calls exploring the codebase, it gets the exact files it needs in one shot.

**Inspired by [jeremychone's approach](https://news.ycombinator.com/item?id=43367518):** code map with a cheap model, auto-context with a cheap model, then code with a big model. Higher precision on the input leads to higher precision on the output.

## How It Works

```
                    codemap build                    codemap select
                    (once, cached)                   (per task, fast)
                         |                                |
    Your repo -----> Per-file index -----> Cheap model picks files -----> Agent gets
    2688 files       summary, types,       "381 candidates ->            focused context
                     functions, imports     5 files (27 KB)"             30-80k tokens
```

1. **`codemap build`** — Indexes every file with a one-line summary using a cheap model (Haiku/Flash). Cached incrementally via mtime + BLAKE3 — only changed files get re-indexed.

2. **`codemap select`** — Given a task description, sends the summaries (not source) to a cheap model which picks the 5-10 files that matter. Returns their **full source code**.

3. **Your agent** — Gets exactly the files it needs. No grep. No glob. No wrong turns.

## Install

```bash
go install github.com/jonnonz1/codemap/cmd/codemap@latest
```

## Quick Start

```bash
cd your-project

# Interactive setup — picks provider, model, API key
codemap init

# Index your repo (first run takes minutes, subsequent runs are seconds)
codemap build

# Register MCP server with Claude Code
claude mcp add codemap -- codemap mcp
```

That's it. Start a Claude Code session and it will discover `codemap_select` as a native tool.

## Setup

`codemap init` prompts you interactively:

```
Select LLM provider for file summaries:
  1) anthropic  (Claude — recommended)
  2) openai     (GPT)
  3) google     (Gemini)
  4) mock       (no LLM, placeholder summaries)

Provider [1]: 1
Model [claude-haiku-4-5-20251001]:
API key (stored in .codemap.yaml): sk-ant-...
```

This creates:
- **`.codemap.yaml`** — config with your API key (gitignored)
- **`CLAUDE.md` section** — tells Claude Code to use codemap tools
- **SessionStart hook** — injects code map status at session start
- **Example task file** in `tasks/`

## Claude Code Integration (MCP)

Codemap runs as an MCP server that Claude Code calls natively:

```bash
# Register once
claude mcp add codemap -- codemap mcp
```

Claude gets three tools:

| Tool | What it does |
|------|-------------|
| `codemap_select` | Given a task, returns full source of the most relevant files |
| `codemap_status` | Check if the index is fresh or stale |
| `codemap_build` | Trigger an incremental rebuild |

When you give Claude a task, it calls `codemap_select` first, gets focused context, and starts coding — no exploration needed.

## CLI Commands

```bash
codemap init                    # Interactive project setup
codemap build                   # Index repo (incremental, cached)
codemap render                  # Render code map as markdown
codemap select --task task.md   # Select files for a task (CLI mode)
codemap context                 # Show what gets injected at session start
codemap doctor                  # Check cache health
codemap statistics              # View usage metrics
codemap statistics --eval       # Evaluate selection accuracy vs git
```

## Measuring Impact

Codemap tracks real metrics — no guesses, no estimates:

```bash
$ codemap statistics --eval
codemap statistics
==================

Build Performance
  Total builds:        12
  Files indexed:       2688
  Avg cache hit rate:  94%

Context Selection
  Total selections:    8
  Avg files selected:  6.2
  Avg context saved:   97%

Selection Accuracy (vs actual git changes)
  Evaluations:         5
  Avg precision:       65%  (of selected files, how many were actually needed)
  Avg recall:          82%  (of changed files, how many were pre-selected)

Exploration Overhead
  Total Read calls:    48
  Extra reads:         7   (files NOT in codemap selection)
  Overhead:            15%
  Verdict:             codemap is providing good coverage
```

**What each metric means:**
- **Hit rate / recall** — Did codemap predict the files you actually changed?
- **Precision** — Did codemap include junk files you didn't need?
- **Exploration overhead** — Did Claude need to search beyond what codemap gave it?
- **Context saved** — How much was the candidate pool compressed?

All computed from observed data (git diff, tool call logs). No counterfactuals.

## Task Files

For CLI-based selection (without MCP), write a task file:

```markdown
---
context_globs:
  - src/invoices/**
  - tests/invoices/**
knowledge_globs:
  - src/types/**
max_files: 10
---

Add soft-delete support to invoices. Preserve existing patterns. Update tests.
```

```bash
codemap select --task task.md
cat .claude/cache/selected-context.md  # full source of selected files
```

## How Indexing Works

Each file in the code map has:
- **summary** — one-sentence description (from LLM)
- **when_to_use** — when a developer would need this file (from LLM)
- **public_types** — exported type names (from parser)
- **public_functions** — exported function names (from parser)
- **imports** — dependencies (from parser)
- **keywords** — domain terms (from LLM)

Deterministic facts come from parsers (currently Go via `go/ast`). Semantic fields come from the cheap LLM. The index is cached as JSON and only rebuilt for files that actually changed (mtime + BLAKE3 hash).

## Providers

| Provider | Model | Cost for 2700 files |
|----------|-------|-------------------|
| Anthropic | claude-haiku-4-5-20251001 | ~$2-3 |
| OpenAI | gpt-4o-mini | ~$1-2 |
| Google | gemini-2.0-flash | ~$0.50 |
| Mock | (none) | Free |

The mock provider works without any API key — useful for testing the workflow before committing to a provider.

## Configuration

`.codemap.yaml` (gitignored, contains API key):

```yaml
llm:
  provider: anthropic
  model: claude-haiku-4-5-20251001
  api_key: sk-ant-...
  workers: 32        # concurrent API calls during build
  rate_limit: 50     # max requests per minute
cache_dir: .claude/cache
```

## Requirements

- Go 1.22+
- An LLM API key (or use mock mode)
- Claude Code (for MCP integration) or any agent that reads markdown

## License

MIT
