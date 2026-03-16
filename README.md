# codemap

Incremental repo intelligence and context-selection CLI tool for improving coding-agent precision on large repositories.

Codemap scans your repo, builds a per-file index using parser-based extraction for deterministic facts (types, functions, imports), and uses a pluggable LLM boundary only for semantic fields like summaries. It then selects the smallest useful set of files for a given coding task and renders model-friendly markdown context.

## Install

```bash
go install github.com/codemap/cmd/codemap@latest
```

Or build from source:

```bash
git clone https://github.com/jonnonz1/codemap.git
cd codemap
go build ./cmd/codemap
```

## Quick Start

```bash
# Index your repository
codemap build

# Render the code map as markdown
codemap render

# Select relevant files for a task
codemap select --task task.md

# Check cache health
codemap doctor
```

## Commands

### `codemap build`

Scans the repository, parses source files, and writes the code map cache.

- Respects ignore rules (`.git`, `node_modules`, `vendor`, `dist`, etc.)
- Uses **mtime + BLAKE3** for incremental rebuilds — unchanged files are skipped
- Extracts imports, exported types, and exported functions from Go files via `go/ast`
- Enriches entries with summary/keywords via a pluggable `Summarizer` interface
- Writes full cache to `.claude/cache/context-code-map.json`
- Appends changed entries to `.claude/cache/context-code-map.jsonl`

```
$ codemap build
codemap build complete
  total files:   29
  added:         29
  updated:       0
  unchanged:     0
  removed:       0
```

### `codemap render`

Renders the cached code map as stable, sorted markdown to `.claude/cache/context-code-map.md`.

Output format:
```markdown
- internal/scan/scan.go
  - summary: Source file scan.go
  - public types: FileInfo
  - public functions: Dir
  - imports: io/fs, path/filepath, strings
```

### `codemap select --task <path>`

Selects the most relevant files for a coding task. Task files use markdown with YAML frontmatter:

```markdown
---
knowledge_globs:
  - docs/**
  - src/core/**
context_globs:
  - src/invoices/**
  - tests/invoices/**
max_files: 12
max_tokens: 50000
---

Add soft-delete support to invoices. Preserve existing patterns. Update tests.
```

Selection behavior:
- Constrains candidates by `knowledge_globs` + `context_globs` (supports `**` patterns)
- Scores files by path relevance, summary/keyword overlap, public symbol matches, and test proximity
- Expands one hop via imports to pull in nearby useful files
- All scoring is deterministic — no embeddings, no vector DB

Outputs:
- `.claude/cache/selected-files.txt` — one file path per line
- `.claude/cache/selected-context.md` — task description + selected file summaries

### `codemap doctor`

Reports cache health and diagnostics.

```
$ codemap doctor
codemap doctor
==============

  [+] JSON cache
  [+] JSONL log
  [+] Markdown render

Indexed files:     29
Missing summaries: 0
Stale files:       0

Languages:
  go              26 files
  markdown        3 files
```

## Architecture

```
cmd/codemap/          CLI entry point
internal/
  model/              CodeMapEntry, CodeMap types
  scan/               File system scanning with ignore rules
  hash/               BLAKE3 content hashing
  parse/              Parser interface + registry
  langs/golang/       Go AST parser (extracts types, functions, imports)
  store/              JSON/JSONL cache persistence (atomic writes)
  llm/                Summarizer interface + MockSummarizer
  build/              Incremental build orchestrator
  render/             Markdown rendering
  taskfile/           Task file YAML frontmatter parsing
  selectpkg/          File selection, scoring, and import expansion
  doctor/             Cache diagnostics
```

### Design Principles

- **Deterministic facts** (types, functions, imports) come from parsers, never an LLM
- **Semantic fields** (summary, when_to_use, keywords) come from a pluggable `Summarizer` interface
- A `MockSummarizer` ships by default — the tool works locally with no model configured
- Language adapters implement the `parse.Parser` interface for future TypeScript/Python/Rust support
- Cache writes are atomic (write to temp file, then rename)
- Map iterations are sorted for deterministic output across runs

## Testing

```bash
go test ./...
```

Tests cover: file scanning, BLAKE3 hashing, Go AST parsing, JSON store round-trips, markdown rendering, task file parsing (including CRLF and edge cases), file selection scoring, deterministic ordering, import expansion, and doctor diagnostics.

## Cache Artifacts

All artifacts are written to `.claude/cache/` and are gitignored:

| File | Format | Purpose |
|------|--------|---------|
| `context-code-map.json` | JSON | Full code map cache |
| `context-code-map.jsonl` | JSONL | Append log of changed entries |
| `context-code-map.md` | Markdown | Rendered code map for agents |
| `selected-files.txt` | Text | Selected file paths |
| `selected-context.md` | Markdown | Task context for agents |

## Dependencies

Minimal — only three external dependencies:

- [`lukechampine.com/blake3`](https://pkg.go.dev/lukechampine.com/blake3) — BLAKE3 hashing
- [`gopkg.in/yaml.v3`](https://pkg.go.dev/gopkg.in/yaml.v3) — YAML frontmatter parsing
- [`github.com/bmatcuk/doublestar/v4`](https://pkg.go.dev/github.com/bmatcuk/doublestar/v4) — `**` glob pattern matching

## License

MIT
