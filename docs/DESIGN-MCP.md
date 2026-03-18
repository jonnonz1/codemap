# MCP Tool Design

## The problem

Without codemap, Claude Code's workflow for something like "add pagination to invoices" looks like this:

1. Glob for `*invoice*` — 47 results
2. Read 5 files to understand the structure
3. Grep for `fetchInvoices` — 12 results
4. Read 4 more files
5. Grep for pagination patterns — 8 results
6. Read 3 more files
7. Finally start coding

That's 60-100k tokens burned on exploration alone. 12+ files read, many only partially useful. Plus the Glob/Grep results themselves. Plus wrong turns.

With codemap auto-context:

1. Claude calls `codemap_select` with "add pagination to invoices"
2. Haiku reads the pre-indexed summaries, picks 5 files
3. Tool returns full source of those 5 files
4. Claude starts coding

30-50k tokens. 5 focused files. No exploration waste.

## Why it matters

jeremychone's numbers: 381 files (1.62 MB) compressed to 5 files (27.90 KB). That's a 98% reduction in candidate context — a cheap model reading summaries (not source) and making an intelligent selection.

It's not just about size though — it's precision:
- No exploration artifacts (failed Globs, wrong files read)
- No partial reads of files that turned out to be irrelevant
- The cheap model (Haiku) burns its own tokens for selection, not the expensive model's context window
- One tool call replaces 10+ Glob/Grep/Read cycles

## MCP tools

### `codemap_select`

The main tool. Claude calls this when it has a task and needs focused context.

```
Input:
  task: string              — natural language task description
  context_globs: string[]   — optional, narrow to specific directories
  knowledge_globs: string[] — optional, include reference files
  max_files: number         — optional, default 10

Output:
  context_files: [
    {path: string, source: string}
  ]
  knowledge_files: [
    {path: string, source: string}
  ]
  reasoning: string      — one sentence explaining the selection
  from_cache: boolean    — true if selection was cached
  session_id: string     — unique ID for this selection (used by statistics)
```

Internally:
1. Check if code map exists and is reasonably fresh
2. If stale, run incremental build (fast for small changes)
3. Filter code map by globs
4. Send filtered summaries + task to Haiku
5. Haiku returns file list
6. Read full source of selected files
7. Log the selection event with session_id
8. Return to Claude

### `codemap_status`

Quick check on code map health.

```
Input: (none)

Output:
  indexed_files: number
  stale_files: number
  last_build: string
  has_cache: boolean
```

### `codemap_build`

Trigger an incremental rebuild.

```
Input:
  force: boolean — optional, rebuild everything

Output:
  total_files: number
  added: number
  updated: number
  unchanged: number
  duration: string
```

## Measurement — what we can actually prove

### Only measure what's real

No guessing about "tokens saved" or counterfactual comparisons. Only metrics from actual observed data.

### Data collection

**Per-selection event (logged by codemap_select):**
```json
{
  "session_id": "abc123",
  "timestamp": "2026-03-17T10:30:00Z",
  "task": "add pagination to invoices",
  "candidates": 381,
  "selected": 5,
  "selected_paths": [...],
  "selected_bytes": 28430,
  "candidate_bytes": 1620000,
  "selection_time_ms": 2400,
  "from_cache": false
}
```

**Per-session exploration tracking (via PostToolUse hook):**

A PostToolUse hook logs every Read/Glob/Grep call Claude makes after receiving codemap context. This tracks exploration beyond what codemap provided.

```json
{
  "session_id": "abc123",
  "tool": "Read",
  "path": "src/components/InvoiceList.tsx",
  "in_selection": true,
  "timestamp": "2026-03-17T10:31:00Z"
}
```

**Per-session outcome (via git diff at session end):**

When the session ends (or user runs `codemap statistics`), compare files codemap selected against files actually modified via `git diff --name-only`.

### Metrics from real data

**Selection hit rate:**
```
hit_rate = files_modified_that_were_selected / files_modified_total
```
Codemap selected 5 files. User modified 4. 3 of those 4 were in the selection. Hit rate = 75%. Ground truth.

**Selection precision:**
```
precision = files_modified_that_were_selected / files_selected_total
```
Selected 5, modified 3 of them. Precision = 60%. High precision means codemap didn't include junk.

**Exploration overhead:**
```
extra_reads = Read_calls_for_files_NOT_in_selection
total_reads = all_Read_calls_in_session
overhead = extra_reads / total_reads
```
Claude made 8 Read calls. 5 were for files codemap selected. 3 were additional exploration. Overhead = 37%. Low overhead means codemap gave Claude everything it needed.

**Context compression (real, not estimated):**
```
compression = selected_bytes / candidate_bytes
```
381 candidates = 1.62 MB. 5 selected = 27.9 KB. Compression = 98.3%. Both numbers are measured.

**Tool call reduction:**
```
exploration_calls = Glob + Grep + Read calls AFTER codemap_select
```
Track per session, compare over time. Trending toward 0-2 means codemap is providing complete context. Staying at 10+ means the selection needs work.

### What `codemap statistics` shows

```
codemap statistics
==================

Selection Accuracy (last 10 sessions)
  Avg hit rate:          82%
  Avg precision:         65%
  Avg compression:       97%

Exploration Overhead (last 10 sessions)
  Avg extra Read calls:  1.8
  Avg total Read calls:  6.2
  Overhead ratio:        29%

Build Performance
  Total builds:          12
  Avg cache hit rate:    94%
  Files indexed:         2688

Recent Sessions:
  [Mar 17 10:30] "add pagination to invoices"
    selected 5 files, modified 4, hit rate 75%, 2 extra reads
  [Mar 17 14:15] "refactor auth middleware"
    selected 8 files, modified 6, hit rate 83%, 0 extra reads
  [Mar 16 09:00] "fix date formatting bug"
    selected 3 files, modified 2, hit rate 100%, 1 extra read
```

### How exploration tracking works

A PostToolUse hook in `.claude/settings.json` logs tool calls:

```json
{
  "hooks": {
    "PostToolUse": [
      {
        "matcher": "Read|Glob|Grep",
        "hooks": [
          {
            "type": "command",
            "command": "codemap track-tool $TOOL_NAME $TOOL_INPUT"
          }
        ]
      }
    ]
  }
}
```

Optional but enables the exploration overhead metric. Without it, you still get selection accuracy from git diff.

### What we don't claim

- We don't claim "X tokens saved" — can't know the counterfactual
- We don't claim "X% faster" — too many variables
- We don't compare sessions with vs without codemap — not a controlled experiment

We report: how accurate was the file selection (hit rate, precision), how much did Claude explore beyond the selection (overhead), and how much did the code map compress the candidate pool (compression ratio). All from real data.

## MCP server configuration

`.claude/settings.json`:
```json
{
  "mcpServers": {
    "codemap": {
      "command": "codemap",
      "args": ["mcp"],
      "type": "stdio"
    }
  }
}
```

## Claude Code integration

CLAUDE.md instructs Claude:

```
This project uses codemap for focused context selection.

When you receive a task:
1. Call codemap_select with the task description
2. Read the returned source files — they are pre-selected for relevance
3. Only use Glob/Grep/Read for files NOT covered by the selection
4. If codemap_status shows stale data, call codemap_build first
```

## Implementation plan

1. Add `codemap mcp` command — stdio JSON-RPC MCP server
2. Implement codemap_select, codemap_status, codemap_build tools
3. Add session tracking (log selections, track tool usage via hook)
4. Update `codemap init` to register MCP server + hooks in settings.json
5. Update `codemap statistics` to compute real metrics from session data
6. Update CLAUDE.md template with MCP tool usage instructions
7. Test end-to-end on a real project

## Open questions

1. **Auto-build on select?** Should codemap_select auto-rebuild if stale? Leaning yes for incremental (fast), no for initial (slow).

2. **Token budget?** Should codemap_select cap total source bytes returned? If 5 files = 200KB, should it truncate? Probably yes — 80KB default cap.

3. **PostToolUse hook feasibility?** Need to verify that Claude Code passes enough info to the hook (tool name, input path) to track reads.
