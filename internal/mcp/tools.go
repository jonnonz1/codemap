package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jonnonz1/codemap/internal/autoctx"
	"github.com/jonnonz1/codemap/internal/build"
	"github.com/jonnonz1/codemap/internal/config"
	"github.com/jonnonz1/codemap/internal/doctor"
	"github.com/jonnonz1/codemap/internal/langs/golang"
	"github.com/jonnonz1/codemap/internal/llm"
	"github.com/jonnonz1/codemap/internal/model"
	"github.com/jonnonz1/codemap/internal/parse"
	"github.com/jonnonz1/codemap/internal/stats"
	"github.com/jonnonz1/codemap/internal/store"
	"github.com/jonnonz1/codemap/internal/taskfile"
)

// RegisterTools adds all codemap MCP tools to the server.
func RegisterTools(s *Server, repoRoot string, cfg *config.Config) {
	cacheDir := filepath.Join(repoRoot, cfg.CacheDir)
	jsonPath := filepath.Join(cacheDir, "context-code-map.json")
	jsonlPath := filepath.Join(cacheDir, "context-code-map.jsonl")
	st := store.NewJSONStore(jsonPath, jsonlPath)

	s.RegisterTool(Tool{
		Name:        "codemap_select",
		Description: "Select the most relevant source files for a coding task. Returns full source code of selected files. Uses a cheap model to read pre-indexed file summaries and intelligently pick files. Call this FIRST when starting any task.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task": map[string]string{
					"type":        "string",
					"description": "Natural language description of the coding task",
				},
				"context_globs": map[string]any{
					"type":        "array",
					"items":       map[string]string{"type": "string"},
					"description": "Glob patterns to narrow candidate files (e.g. src/invoices/**)",
				},
				"knowledge_globs": map[string]any{
					"type":        "array",
					"items":       map[string]string{"type": "string"},
					"description": "Glob patterns for reference/docs files",
				},
				"max_files": map[string]any{
					"type":        "number",
					"description": "Maximum files to return (default 10)",
				},
			},
			"required": []string{"task"},
		},
	}, func(params json.RawMessage) (any, error) {
		var input struct {
			Task           string   `json:"task"`
			ContextGlobs   []string `json:"context_globs"`
			KnowledgeGlobs []string `json:"knowledge_globs"`
			MaxFiles       int      `json:"max_files"`
		}
		if err := json.Unmarshal(params, &input); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}

		cm, err := st.Load()
		if err != nil || len(cm.Entries) == 0 {
			return nil, fmt.Errorf("no code map found — run codemap build first")
		}

		tf := &taskfile.TaskFile{
			ContextGlobs:   input.ContextGlobs,
			KnowledgeGlobs: input.KnowledgeGlobs,
			MaxFiles:       input.MaxFiles,
			Body:           input.Task,
		}

		caller := newCallerFromConfig(cfg)

		result, err := autoctx.SelectWithDedicatedCall(cm, tf, caller, autoctx.Options{
			RepoRoot: repoRoot,
			CacheDir: cacheDir,
			MaxFiles: input.MaxFiles,
		})
		if err != nil {
			return nil, err
		}

		// Log selection event.
		var selectedPaths []string
		for _, f := range result.ContextFiles {
			selectedPaths = append(selectedPaths, f.Path)
		}
		for _, f := range result.KnowledgeFiles {
			selectedPaths = append(selectedPaths, f.Path)
		}

		// Compute token estimates from actual file sizes.
		totalBytes := 0
		for _, e := range cm.Entries {
			info, err := os.Stat(filepath.Join(repoRoot, e.Path))
			if err == nil {
				totalBytes += int(info.Size())
			}
		}
		selectedBytes := 0
		for _, f := range result.ContextFiles {
			selectedBytes += len(f.Source)
		}
		for _, f := range result.KnowledgeFiles {
			selectedBytes += len(f.Source)
		}

		_ = stats.Log(cacheDir, &stats.Event{
			Type:           stats.EventSelect,
			Timestamp:      time.Now(),
			TaskFile:       "__mcp__",
			TaskBody:       input.Task,
			SelectedFiles:  selectedPaths,
			SelectedCount:  len(selectedPaths),
			TotalIndexed:   len(cm.Entries),
			CandidatePool:  countCandidates(cm, tf),
			TotalTokens:    stats.EstimateTokens(totalBytes),
			SelectedTokens: stats.EstimateTokens(selectedBytes),
		})

		// Build response with full source.
		var sb strings.Builder
		if result.FromCache {
			sb.WriteString("(from cache)\n\n")
		}

		sb.WriteString(fmt.Sprintf("Selected %d context files and %d knowledge files from %d candidates.\n\n",
			len(result.ContextFiles), len(result.KnowledgeFiles), len(cm.Entries)))

		if len(result.KnowledgeFiles) > 0 {
			sb.WriteString("## Knowledge Files\n\n")
			for _, f := range result.KnowledgeFiles {
				writeSourceBlock(&sb, f)
			}
		}

		sb.WriteString("## Context Files\n\n")
		for _, f := range result.ContextFiles {
			writeSourceBlock(&sb, f)
		}

		return sb.String(), nil
	})

	s.RegisterTool(Tool{
		Name:        "codemap_status",
		Description: "Check code map health: whether the index exists, how many files are indexed, and how many are stale.",
		InputSchema: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}, func(params json.RawMessage) (any, error) {
		r, _ := doctor.Run(repoRoot, st)
		return map[string]any{
			"indexed_files":    r.IndexedFiles,
			"missing_summaries": r.MissingSummary,
			"stale_changed":    r.StaleChanged,
			"stale_new":        r.StaleNew,
			"stale_deleted":    r.StaleDeleted,
			"has_cache":        r.CacheExists,
			"languages":        r.Languages,
		}, nil
	})

	s.RegisterTool(Tool{
		Name:        "codemap_build",
		Description: "Rebuild the code map index. Incremental — only re-indexes changed files. Fast for small changes, slower for initial build.",
		InputSchema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"force": map[string]any{
					"type":        "boolean",
					"description": "Force full rebuild (ignore cache)",
				},
			},
		},
	}, func(params json.RawMessage) (any, error) {
		reg := parse.NewRegistry()
		reg.Register(&golang.Parser{})

		summarizer := newSummarizerFromConfig(cfg)

		start := time.Now()
		res, err := build.Run(repoRoot, st, reg, summarizer)
		if err != nil {
			return nil, err
		}

		_ = stats.Log(cacheDir, &stats.Event{
			Type:        stats.EventBuild,
			Timestamp:   time.Now(),
			TotalFiles:  res.TotalFiles,
			Added:       res.Added,
			Updated:     res.Updated,
			Unchanged:   res.Unchanged,
			Removed:     res.Removed,
			ParseErrors: res.ParseErrors,
		})

		return map[string]any{
			"total_files": res.TotalFiles,
			"added":       res.Added,
			"updated":     res.Updated,
			"unchanged":   res.Unchanged,
			"removed":     res.Removed,
			"duration":    time.Since(start).Round(time.Second).String(),
		}, nil
	})
}

func writeSourceBlock(sb *strings.Builder, f autoctx.SelectedFile) {
	ext := filepath.Ext(f.Path)
	lang := strings.TrimPrefix(ext, ".")
	switch ext {
	case ".ts", ".tsx":
		lang = "typescript"
	case ".js", ".jsx":
		lang = "javascript"
	}
	fmt.Fprintf(sb, "### %s\n\n```%s\n%s\n```\n\n", f.Path, lang, f.Source)
}

func countCandidates(cm *model.CodeMap, _ *taskfile.TaskFile) int {
	return len(cm.Entries)
}

func newCallerFromConfig(cfg *config.Config) autoctx.LLMCaller {
	key := cfg.LLM.ResolveAPIKey()
	if key == "" || cfg.LLM.Provider == "" || cfg.LLM.Provider == "mock" {
		return &llm.MockCaller{}
	}
	switch cfg.LLM.Provider {
	case "anthropic":
		return llm.NewAnthropicCaller(key, cfg.LLM.Model)
	case "openai":
		return llm.NewOpenAICaller(key, cfg.LLM.Model)
	case "google":
		return llm.NewGoogleCaller(key, cfg.LLM.Model)
	default:
		return &llm.MockCaller{}
	}
}

func newSummarizerFromConfig(cfg *config.Config) llm.Summarizer {
	key := cfg.LLM.ResolveAPIKey()
	if key == "" || cfg.LLM.Provider == "" || cfg.LLM.Provider == "mock" {
		return &llm.MockSummarizer{}
	}
	switch cfg.LLM.Provider {
	case "anthropic":
		return llm.NewAnthropicSummarizer(key, cfg.LLM.Model)
	case "openai":
		return llm.NewOpenAISummarizer(key, cfg.LLM.Model)
	case "google":
		return llm.NewGoogleSummarizer(key, cfg.LLM.Model)
	default:
		return &llm.MockSummarizer{}
	}
}
