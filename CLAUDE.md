# codemap

Incremental repo intelligence and context-selection CLI tool for improving coding-agent precision on large repos.

## Build & Test

```bash
go build ./cmd/codemap
go test ./...
```

## Project Structure

- `cmd/codemap/` - CLI entry point
- `internal/model/` - Core data types (CodeMapEntry)
- `internal/scan/` - File system scanning with ignore rules
- `internal/hash/` - BLAKE3 content hashing
- `internal/parse/` - Parser interface
- `internal/langs/golang/` - Go AST parser adapter
- `internal/store/` - JSON/JSONL cache persistence
- `internal/render/` - Markdown rendering
- `internal/taskfile/` - Task file YAML frontmatter parsing
- `internal/selectpkg/` - File selection and scoring
- `internal/llm/` - Summarizer interface and mock

## Conventions

- Idiomatic Go: explicit error handling, small interfaces, no magic
- Tests use table-driven patterns where appropriate
- Cache artifacts go in `.claude/cache/` and are gitignored
- The LLM boundary only enriches semantic fields (summary, when_to_use, keywords)
- Deterministic facts (types, functions, imports) come from parsers, never the LLM
- No embeddings, no vector DB, no TUI
