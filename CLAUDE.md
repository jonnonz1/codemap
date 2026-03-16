# codemap

Incremental repo intelligence and context-selection CLI for AI coding agents.

## Build & Test

```bash
go build ./cmd/codemap
go test ./...
```

## Project Structure

```
cmd/codemap/          CLI + MCP server entry point
internal/
  model/              CodeMapEntry, CodeMap types
  scan/               File system scanning with ignore rules
  hash/               BLAKE3 content hashing
  parse/              Parser interface + registry
  langs/golang/       Go AST parser (types, functions, imports)
  store/              JSON/JSONL cache (atomic writes)
  llm/                Summarizer interface, Anthropic/OpenAI/Google/Mock
  build/              Incremental build orchestrator (concurrent, rate-limited)
  autoctx/            LLM-based auto-context file selection
  render/             Markdown rendering
  taskfile/           Task file YAML frontmatter parsing
  selectpkg/          Deterministic scoring (legacy, replaced by autoctx)
  context/            Session context injection
  mcp/                MCP JSON-RPC server + tool handlers
  config/             .codemap.yaml config loading
  initcmd/            codemap init (interactive setup)
  doctor/             Cache diagnostics
  stats/              Usage metrics + exploration tracking
```

## Conventions

- Idiomatic Go: explicit error handling, small interfaces
- LLM boundary: only enriches semantic fields (summary, when_to_use, keywords)
- Deterministic facts (types, functions, imports) come from parsers, never the LLM
- Cache artifacts in `.claude/cache/` (gitignored)
- No embeddings, no vector DB, no TUI
