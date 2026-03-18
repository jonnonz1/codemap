# codemap vision

Based on jeremychone's proven workflow (Hacker News, March 2026).

## The approach that works

Three-phase pipeline, each phase using the right model for the job.

### Phase 1: Code map (cheap model, cached, incremental)

Per file in the repo, generate and cache:
- `summary` — one-sentence description
- `when_to_use` — when a dev would need this file
- `public_types` — exported type names
- `public_functions` — exported function names

Cached until file changes (mtime + blake3). High concurrency (32 workers). Uses the cheapest fast model (Flash/Haiku) — roughly $1-2 for 50k LOC. Serialised as clean structured markdown for AI consumption (not JSON). One trip per file, consistent size per entry.

**Status: We have this. Needs concurrency bump to 32.**

### Phase 2: Auto-context (cheap model, per-task — the magic step)

This is where the real value is. Given the full code map (summaries only, not source), the developer's task description, and narrowing globs (knowledge_globs, context_globs) — ask a cheap model to select the actual files needed.

This isn't keyword matching. This isn't deterministic scoring. The LLM reads all the summaries, understands the task, and returns the 5-10 files that matter.

jeremychone's result: 381 candidate files down to 5 context files.

The developer provides globs in YAML frontmatter to narrow the initial pool, but the LLM does the actual intelligent selection.

**Status: WE DON'T HAVE THIS. This is the critical missing piece.**

Our current `codemap select` uses deterministic scoring (word overlap, symbol matching) which doesn't work well. Needs to be replaced with an LLM-based selection step.

### Phase 3: Focused work (big model, small context)

Feed the selected files (full source) + the task description to the big model. Context is typically 30-80k tokens. High precision input, high precision output.

In our case, this is just Claude Code with the right files pre-loaded.

**Status: This is Claude Code. We just need to feed it the right context.**

## What needs to change

### 1. Replace deterministic select with LLM-based auto-context

Current flow:
```
codemap select --task task.md
→ deterministic scoring (keyword overlap, glob match)
→ mediocre file selection
```

New flow:
```
codemap select --task task.md
→ load code map (summaries only)
→ filter by knowledge_globs + context_globs
→ send code map subset + task description to cheap model
→ cheap model returns list of files needed
→ write selected-files.txt + selected-context.md with FULL SOURCE of selected files
```

The key insight: the cheap model reads summaries (small) to select files, then we include the full source (large) of only those files. That's the compression step that makes it work.

### 2. Include full source in selected context, not just summaries

Current selected-context.md has summaries of selected files. Useless — Claude needs the actual code. Output should be full source of selected files, ready to paste into a context window.

### 3. Knowledge files vs context files

jeremychone distinguishes:
- **context files** — source files relevant to the task (narrowed by context_globs)
- **knowledge files** — reference docs, best practices, style guides (narrowed by knowledge_globs)

Both get included in the final context but serve different purposes.

### 4. Stop injecting the full code map into sessions

2688 file summaries at session start is noise. The code map is an intermediate artifact for the auto-context step, not something Claude should read directly.

The SessionStart hook should only inject:
- Brief status (is cache fresh?)
- Instructions to use `codemap select` for context

### 5. Bump concurrency to 32

jeremychone uses 32 concurrent workers for code mapping. Our default of 10 is conservative. Haiku handles 32 easily.

### 6. One-command workflow

Ideal developer experience:
```bash
codemap select --task task.md
# → auto-context runs (cheap model picks files)
# → selected-context.md has full source of selected files
# → Claude Code reads it and starts working
```

## Architecture changes

```
codemap build          — unchanged (code map generation)
codemap select         — REWRITE (LLM-based auto-context)
codemap render         — keep for debugging/inspection
codemap context        — simplify (brief status only)
codemap doctor         — unchanged
codemap statistics     — add auto-context accuracy tracking
```

### select rewrite pseudocode

```
1. Load code map from cache
2. Filter entries by knowledge_globs + context_globs
3. Serialise filtered entries as markdown (summaries only)
4. Build prompt: "Given this code map and this task, which files
   should be in context? Return a JSON list of file paths."
5. Call cheap model (Haiku/Flash)
6. Parse response → list of file paths
7. Read full source of each selected file
8. Write selected-context.md:
   - Task description
   - Knowledge files (full content)
   - Context files (full source)
9. Write selected-files.txt
```

## What not to change

- Code map format and caching (mtime + blake3) — works well
- Per-file LLM summarisation — works well
- Task file format (YAML frontmatter + body) — matches jeremychone's approach
- JSON/JSONL storage — works well
- Go parser for deterministic facts — useful complement to LLM summaries
- Multi-provider support (Anthropic, OpenAI, Google) — good to have

## Success criteria

After the rewrite, `codemap select --task task.md` should produce a selected-context.md that:
1. Contains 3-10 files (not 20+)
2. Includes full source of those files (not just summaries)
3. Is 30-80k tokens (focused, not bloated)
4. Gets it right 90%+ of the time (the files Claude actually needs)
5. Takes <30 seconds (one cheap model call)

## Reference

jeremychone's approach (Hacker News, March 2026):
- "381 context files (1.62 MB) → 5 context files (27.90 KB)"
- "11 knowledge files (30.16 KB) → 3 knowledge files (5.62 KB)"
- Concurrency 32, Flash for code map + auto-context
- "I have zero sed/grep in my workflow. Just this."
- "Higher precision on the input leads to higher precision on the output."
