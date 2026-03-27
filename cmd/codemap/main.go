// codemap is an incremental repo intelligence and context-selection CLI tool.
//
// Commands:
//
//	codemap init               Initialize codemap in a project directory
//	codemap build              Scan repo and build/update the code map cache
//	codemap render             Render the code map as markdown
//	codemap select --task PATH Select relevant files for a coding task
//	codemap statistics         Show usage stats and selection accuracy
//	codemap doctor             Report cache health and diagnostics
package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/jonnonz1/codemap/internal/autoctx"
	"github.com/jonnonz1/codemap/internal/build"
	"github.com/jonnonz1/codemap/internal/config"
	"github.com/jonnonz1/codemap/internal/context"
	"github.com/jonnonz1/codemap/internal/doctor"
	"github.com/jonnonz1/codemap/internal/initcmd"
	"github.com/jonnonz1/codemap/internal/langs/golang"
	"github.com/jonnonz1/codemap/internal/llm"
	"github.com/jonnonz1/codemap/internal/mcp"
	"github.com/jonnonz1/codemap/internal/parse"
	"github.com/jonnonz1/codemap/internal/render"
	"github.com/jonnonz1/codemap/internal/stats"
	"github.com/jonnonz1/codemap/internal/store"
	"github.com/jonnonz1/codemap/internal/taskfile"
)

// Set via -ldflags at build time. Falls back to Go module build info.
var (
	version = ""
	commit  = ""
	date    = ""
)

func init() {
	if version != "" {
		return
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		version = "dev"
		return
	}
	if info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	} else {
		version = "dev"
	}
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			if len(s.Value) > 7 {
				commit = s.Value[:7]
			} else {
				commit = s.Value
			}
		case "vcs.time":
			date = s.Value
		}
	}
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	repoRoot, err := findRepoRoot()
	if err != nil {
		fatal("finding repo root: %v", err)
	}

	// Load project config (falls back to defaults if .codemap.yaml missing).
	cfg, err := config.Load(filepath.Join(repoRoot, config.FileName))
	if err != nil {
		fatal("loading config: %v", err)
	}

	cacheDir := filepath.Join(repoRoot, cfg.CacheDir)
	jsonPath := filepath.Join(cacheDir, "context-code-map.json")
	jsonlPath := filepath.Join(cacheDir, "context-code-map.jsonl")
	mdPath := filepath.Join(cacheDir, "context-code-map.md")

	st := store.NewJSONStore(jsonPath, jsonlPath)

	switch os.Args[1] {
	case "version", "--version", "-v":
		fmt.Printf("codemap %s\n", version)
		if commit != "" {
			fmt.Printf("  commit: %s\n", commit)
		}
		if date != "" {
			fmt.Printf("  built:  %s\n", date)
		}
		return
	case "init":
		runInit(repoRoot)
	case "build":
		runBuild(repoRoot, st, cfg, cacheDir)
	case "render":
		runRender(st, mdPath)
	case "select":
		runSelect(st, cfg, cacheDir, repoRoot)
	case "context":
		runContext(st)
	case "mcp":
		runMCP(repoRoot, cfg)
	case "track-tool":
		runTrackTool(cacheDir)
	case "statistics", "stats":
		runStatistics(repoRoot, cacheDir)
	case "doctor":
		runDoctor(repoRoot, st)
	case "help", "--help", "-h":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
		printUsage()
		os.Exit(1)
	}
}

func runInit(repoRoot string) {
	opts := initcmd.Options{RepoRoot: repoRoot, Interactive: true}

	// Parse flags: --provider, --model, --api-key
	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--provider":
			if i+1 < len(args) {
				opts.Provider = args[i+1]
				i++
			}
		case "--model":
			if i+1 < len(args) {
				opts.Model = args[i+1]
				i++
			}
		case "--api-key":
			if i+1 < len(args) {
				opts.APIKey = args[i+1]
				i++
			}
		}
	}

	// Skip interactive if all required flags provided.
	if opts.Provider != "" {
		opts.Interactive = false
	}

	res, err := initcmd.Run(opts)
	if err != nil {
		fatal("init: %v", err)
	}
	initcmd.Print(res, os.Stdout)
}

func runBuild(repoRoot string, st store.Store, cfg *config.Config, cacheDir string) {
	reg := parse.NewRegistry()
	reg.Register(&golang.Parser{})

	summarizer := newSummarizer(cfg)

	opts := build.DefaultOptions()
	opts.Scan = &cfg.Scan
	if cfg.LLM.Workers > 0 {
		opts.Workers = cfg.LLM.Workers
	}
	if cfg.LLM.RateLimit > 0 {
		opts.RateLimit = cfg.LLM.RateLimit
	}

	startTime := time.Now()

	progress := func(p build.Progress) {
		switch p.Phase {
		case "scan":
			if p.Current%20 == 0 || p.Current == p.Total {
				if p.Skipped {
					fmt.Fprintf(os.Stderr, "\r  scanning [%d/%d] cached  %s\033[K", p.Current, p.Total, truncatePath(p.Path, 50))
				} else {
					fmt.Fprintf(os.Stderr, "\r  scanning [%d/%d] %s\033[K", p.Current, p.Total, truncatePath(p.Path, 50))
				}
			}
		case "summarize":
			elapsed := time.Since(startTime)
			eta := estimateRemaining(elapsed, p.Current, p.Total-p.Current)
			fmt.Fprintf(os.Stderr, "\r  summarizing [%d/%d] %s  (eta %s)\033[K", p.Current, p.Total, truncatePath(p.Path, 35), eta)
		case "save":
			fmt.Fprintf(os.Stderr, "\r  saving cache...\033[K")
		}
	}

	res, err := build.RunWithOptions(repoRoot, st, reg, summarizer, opts, progress)
	if err != nil {
		fmt.Fprintln(os.Stderr)
		fatal("build: %v", err)
	}

	fmt.Fprintf(os.Stderr, "\r\033[K")

	// Log build event for statistics.
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

	fmt.Printf("codemap build complete\n")
	fmt.Printf("  total files:   %d\n", res.TotalFiles)
	fmt.Printf("  added:         %d\n", res.Added)
	fmt.Printf("  updated:       %d\n", res.Updated)
	fmt.Printf("  unchanged:     %d\n", res.Unchanged)
	fmt.Printf("  removed:       %d\n", res.Removed)
	fmt.Printf("  duration:      %s\n", time.Since(startTime).Round(time.Second))
	if res.ParseErrors > 0 {
		fmt.Printf("  parse errors:  %d\n", res.ParseErrors)
	}
	if res.SkippedTrivial > 0 {
		fmt.Printf("  trivial skip:  %d\n", res.SkippedTrivial)
	}
	if res.SummaryErrors > 0 {
		fmt.Printf("  LLM errors:    %d\n", res.SummaryErrors)
	}
}

func runRender(st store.Store, mdPath string) {
	cm, err := st.Load()
	if err != nil {
		fatal("loading cache: %v", err)
	}
	if len(cm.Entries) == 0 {
		fatal("no entries in cache — run 'codemap build' first")
	}

	if err := os.MkdirAll(filepath.Dir(mdPath), 0o755); err != nil {
		fatal("creating cache dir: %v", err)
	}

	f, err := os.Create(mdPath)
	if err != nil {
		fatal("creating markdown file: %v", err)
	}
	defer f.Close()

	if err := render.Markdown(cm, f); err != nil {
		fatal("rendering markdown: %v", err)
	}

	fmt.Printf("codemap render complete\n")
	fmt.Printf("  entries: %d\n", len(cm.Entries))
	fmt.Printf("  output:  %s\n", mdPath)
}

func runSelect(st store.Store, cfg *config.Config, cacheDir, repoRoot string) {
	taskPath := ""
	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		if args[i] == "--task" && i+1 < len(args) {
			taskPath = args[i+1]
			break
		}
	}
	if taskPath == "" {
		fatal("usage: codemap select --task <path>")
	}

	tf, err := taskfile.Parse(taskPath)
	if err != nil {
		fatal("parsing task file: %v", err)
	}

	cm, err := st.Load()
	if err != nil {
		fatal("loading cache: %v", err)
	}
	if len(cm.Entries) == 0 {
		fatal("no entries in cache — run 'codemap build' first")
	}

	caller := newCaller(cfg)

	fmt.Fprintf(os.Stderr, "  selecting files from %d candidates...\n", len(cm.Entries))

	result, err := autoctx.SelectWithDedicatedCall(cm, tf, caller, autoctx.Options{
		RepoRoot: repoRoot,
		CacheDir: cacheDir,
		MaxFiles: tf.MaxFiles,
	})
	if err != nil {
		fatal("auto-context: %v", err)
	}

	allFiles := append(result.ContextFiles, result.KnowledgeFiles...)

	// Write selected-files.txt
	filesPath := filepath.Join(cacheDir, "selected-files.txt")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		fatal("creating cache dir: %v", err)
	}

	var selectedPaths []string
	for _, f := range allFiles {
		selectedPaths = append(selectedPaths, f.Path)
	}
	if err := os.WriteFile(filesPath, []byte(strings.Join(selectedPaths, "\n")+"\n"), 0o644); err != nil {
		fatal("writing selected files: %v", err)
	}

	// Write selected-context.md with FULL SOURCE.
	contextPath := filepath.Join(cacheDir, "selected-context.md")
	cf, err := os.Create(contextPath)
	if err != nil {
		fatal("creating context file: %v", err)
	}
	defer cf.Close()

	fmt.Fprintln(cf, "# Task Context")
	fmt.Fprintln(cf)
	fmt.Fprintln(cf, "## Task")
	fmt.Fprintln(cf)
	fmt.Fprintln(cf, tf.Body)
	fmt.Fprintln(cf)

	if len(result.KnowledgeFiles) > 0 {
		fmt.Fprintf(cf, "## Knowledge Files (%d)\n\n", len(result.KnowledgeFiles))
		for _, f := range result.KnowledgeFiles {
			writeFileBlock(cf, f)
		}
	}

	fmt.Fprintf(cf, "## Context Files (%d)\n\n", len(result.ContextFiles))
	for _, f := range result.ContextFiles {
		writeFileBlock(cf, f)
	}

	// Log select event for statistics.
	_ = stats.Log(cacheDir, &stats.Event{
		Type:          stats.EventSelect,
		Timestamp:     time.Now(),
		TaskFile:      taskPath,
		TaskBody:      tf.Body,
		SelectedFiles: selectedPaths,
		SelectedCount: len(selectedPaths),
		CandidatePool: len(cm.Entries),
		TotalIndexed:  len(cm.Entries),
	})

	fromCache := ""
	if result.FromCache {
		fromCache = " (cached)"
	}
	fmt.Printf("codemap select complete%s\n", fromCache)
	fmt.Printf("  task:      %s\n", taskPath)
	fmt.Printf("  context:   %d files\n", len(result.ContextFiles))
	fmt.Printf("  knowledge: %d files\n", len(result.KnowledgeFiles))
	fmt.Printf("  files:     %s\n", filesPath)
	fmt.Printf("  context:   %s\n", contextPath)
}

func writeFileBlock(w io.Writer, f autoctx.SelectedFile) {
	ext := filepath.Ext(f.Path)
	lang := ""
	switch ext {
	case ".go":
		lang = "go"
	case ".ts", ".tsx":
		lang = "typescript"
	case ".js", ".jsx":
		lang = "javascript"
	case ".py":
		lang = "python"
	case ".rs":
		lang = "rust"
	default:
		lang = strings.TrimPrefix(ext, ".")
	}
	fmt.Fprintf(w, "### %s\n\n", f.Path)
	fmt.Fprintf(w, "```%s\n%s\n```\n\n", lang, f.Source)
}

// newCaller creates an LLM caller for auto-context selection.
func newCaller(cfg *config.Config) autoctx.LLMCaller {
	provider := cfg.LLM.Provider
	key := cfg.LLM.ResolveAPIKey()

	if key == "" || provider == "" || provider == "mock" {
		return &llm.MockCaller{}
	}

	switch provider {
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

func runStatistics(repoRoot string, cacheDir string) {
	events, err := stats.LoadEvents(cacheDir)
	if err != nil {
		fatal("loading stats: %v", err)
	}

	// Parse flags.
	evalMode := false
	evalTask := ""
	evalCommits := 5 // default: look at last 5 commits
	args := os.Args[2:]
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--eval":
			evalMode = true
		case "--task":
			if i+1 < len(args) {
				evalTask = args[i+1]
				i++
			}
		case "--commits":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &evalCommits)
				i++
			}
		}
	}

	var gitChanges map[string][]string

	if evalMode {
		gitChanges = getGitChanges(repoRoot, evalTask, evalCommits)
	}

	toolUses, _ := stats.LoadToolUses(cacheDir)
	r := stats.ComputeFull(events, gitChanges, toolUses)
	stats.Print(r, os.Stdout)
}

// getGitChanges returns files changed in recent commits, keyed by task file.
// If evalTask is specified, only that task file is evaluated.
// Otherwise, all logged task files are evaluated against git changes.
func getGitChanges(repoRoot, evalTask string, numCommits int) map[string][]string {
	result := make(map[string][]string)

	// Get files changed in last N commits.
	// Try progressively fewer commits if the repo doesn't have enough history.
	var out []byte
	for n := numCommits; n >= 1; n-- {
		cmd := exec.Command("git", "diff", "--name-only", fmt.Sprintf("HEAD~%d", n))
		cmd.Dir = repoRoot
		var err error
		out, err = cmd.Output()
		if err == nil {
			break
		}
		out = nil
	}
	if out == nil {
		// Single-commit repo: list all tracked files as "changed".
		cmd := exec.Command("git", "ls-files")
		cmd.Dir = repoRoot
		out, _ = cmd.Output()
	}

	changed := parseGitOutput(string(out))

	if evalTask != "" {
		result[evalTask] = changed
	} else {
		// Apply same git changes to all task files (best-effort).
		result["*"] = changed
	}

	return result
}

func parseGitOutput(output string) []string {
	var files []string
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			files = append(files, line)
		}
	}
	return files
}

func runTrackTool(cacheDir string) {
	// Called by PostToolUse hook: codemap track-tool <tool> <path>
	args := os.Args[2:]
	toolName := ""
	toolPath := ""
	if len(args) >= 1 {
		toolName = args[0]
	}
	if len(args) >= 2 {
		toolPath = args[1]
	}
	if toolName == "" {
		return
	}
	_ = stats.LogToolUse(cacheDir, &stats.ToolUseEvent{
		Timestamp: time.Now(),
		Tool:      toolName,
		Path:      toolPath,
	})
}

func runMCP(repoRoot string, cfg *config.Config) {
	s := mcp.NewServer()
	mcp.RegisterTools(s, repoRoot, cfg)
	if err := s.Run(); err != nil {
		fatal("mcp server: %v", err)
	}
}

func runContext(st store.Store) {
	if err := context.Inject(st, os.Stdout); err != nil {
		fatal("context: %v", err)
	}
}

func runDoctor(repoRoot string, st store.Store) {
	r, err := doctor.Run(repoRoot, st)
	if err != nil {
		fatal("doctor: %v", err)
	}
	doctor.Print(r, os.Stdout)
}

// newSummarizer returns a Summarizer based on the project config.
func newSummarizer(cfg *config.Config) llm.Summarizer {
	provider := cfg.LLM.Provider
	if provider == "" || provider == "mock" {
		return &llm.MockSummarizer{}
	}

	key := cfg.LLM.ResolveAPIKey()
	if key == "" {
		envHint := cfg.LLM.APIKeyEnv
		if envHint == "" {
			envHint = "the appropriate env var"
		}
		fmt.Fprintf(os.Stderr, "codemap: no API key found (set api_key in .codemap.yaml or %s)\n", envHint)
		fmt.Fprintf(os.Stderr, "codemap: falling back to mock summarizer\n")
		return &llm.MockSummarizer{}
	}

	switch provider {
	case "anthropic":
		fmt.Fprintf(os.Stderr, "codemap: using Anthropic (model: %s)\n", cfg.LLM.Model)
		return llm.NewAnthropicSummarizer(key, cfg.LLM.Model)
	case "openai":
		fmt.Fprintf(os.Stderr, "codemap: using OpenAI (model: %s)\n", cfg.LLM.Model)
		return llm.NewOpenAISummarizer(key, cfg.LLM.Model)
	case "google":
		fmt.Fprintf(os.Stderr, "codemap: using Google Gemini (model: %s)\n", cfg.LLM.Model)
		return llm.NewGoogleSummarizer(key, cfg.LLM.Model)
	default:
		fmt.Fprintf(os.Stderr, "codemap: unknown provider %q, using mock\n", provider)
		return &llm.MockSummarizer{}
	}
}

func printUsage() {
	fmt.Println(`codemap - incremental repo intelligence and context-selection tool

Usage:
  codemap init                         Initialize codemap in a project
  codemap init --provider anthropic    Initialize with LLM provider
  codemap build                        Scan repo and build/update the code map cache
  codemap render                       Render the code map as markdown
  codemap select --task PATH           Select relevant files for a coding task
  codemap context                      Print context injected into Claude sessions
  codemap statistics                   Show usage stats and selection accuracy
  codemap statistics --eval            Evaluate selection accuracy against git changes
  codemap statistics --eval --task X   Evaluate a specific task file
  codemap doctor                       Report cache health and diagnostics
  codemap version                      Print version information`)
}

// findRepoRoot walks up from the current directory to find the repo root
// by looking for a .git directory.
func findRepoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	// Fallback to current directory.
	return os.Getwd()
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "codemap: "+format+"\n", args...)
	os.Exit(1)
}

func truncatePath(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "..." + s[len(s)-n+3:]
}

func estimateRemaining(elapsed time.Duration, completed, remaining int) string {
	if completed == 0 {
		return "calculating..."
	}
	perFile := elapsed / time.Duration(completed)
	eta := perFile * time.Duration(remaining)
	if eta < time.Second {
		return "<1s"
	}
	if eta < time.Minute {
		return fmt.Sprintf("%ds", int(eta.Seconds()))
	}
	return fmt.Sprintf("%dm%ds", int(eta.Minutes()), int(eta.Seconds())%60)
}
