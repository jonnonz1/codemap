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
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/jonnonz1/codemap/internal/build"
	"github.com/jonnonz1/codemap/internal/config"
	"github.com/jonnonz1/codemap/internal/doctor"
	"github.com/jonnonz1/codemap/internal/initcmd"
	"github.com/jonnonz1/codemap/internal/langs/golang"
	"github.com/jonnonz1/codemap/internal/llm"
	"github.com/jonnonz1/codemap/internal/parse"
	"github.com/jonnonz1/codemap/internal/render"
	"github.com/jonnonz1/codemap/internal/selectpkg"
	"github.com/jonnonz1/codemap/internal/stats"
	"github.com/jonnonz1/codemap/internal/store"
	"github.com/jonnonz1/codemap/internal/taskfile"
)

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
	case "init":
		runInit(repoRoot)
	case "build":
		runBuild(repoRoot, st, cfg, cacheDir)
	case "render":
		runRender(st, mdPath)
	case "select":
		runSelect(st, cacheDir)
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
	isMock := cfg.LLM.Provider == "" || cfg.LLM.Provider == "mock"

	startTime := time.Now()
	llmCalls := 0

	progress := func(p build.Progress) {
		if p.Summarized {
			llmCalls++
		}

		// Show progress every 10 files, or on every LLM call.
		if p.Summarized || p.Current%10 == 0 || p.Current == p.Total {
			elapsed := time.Since(startTime)

			if p.Skipped {
				fmt.Fprintf(os.Stderr, "\r  [%d/%d] cached  %s", p.Current, p.Total, truncatePath(p.Path, 50))
			} else if p.Summarized {
				// Estimate remaining time based on LLM calls.
				remaining := estimateRemaining(elapsed, llmCalls, p.Total-p.Current)
				fmt.Fprintf(os.Stderr, "\r  [%d/%d] summarizing  %s  (eta %s)", p.Current, p.Total, truncatePath(p.Path, 40), remaining)
			} else {
				fmt.Fprintf(os.Stderr, "\r  [%d/%d] indexing  %s", p.Current, p.Total, truncatePath(p.Path, 50))
			}

			// Clear rest of line.
			fmt.Fprintf(os.Stderr, "\033[K")
		}
	}

	res, err := build.RunWithProgress(repoRoot, st, reg, summarizer, progress)
	if err != nil {
		fmt.Fprintln(os.Stderr) // newline after progress
		fatal("build: %v", err)
	}

	// Clear progress line.
	if !isMock || res.TotalFiles > 10 {
		fmt.Fprintf(os.Stderr, "\r\033[K")
	}

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
	if res.ParseErrors > 0 {
		fmt.Printf("  parse errors:  %d\n", res.ParseErrors)
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

func runSelect(st store.Store, cacheDir string) {
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

	candidates := selectpkg.Select(cm, tf)

	// Write selected-files.txt
	filesPath := filepath.Join(cacheDir, "selected-files.txt")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		fatal("creating cache dir: %v", err)
	}

	var selectedFiles []string
	for _, c := range candidates {
		selectedFiles = append(selectedFiles, c.Entry.Path)
	}
	if err := os.WriteFile(filesPath, []byte(strings.Join(selectedFiles, "\n")+"\n"), 0o644); err != nil {
		fatal("writing selected files: %v", err)
	}

	// Write selected-context.md
	contextPath := filepath.Join(cacheDir, "selected-context.md")
	selectedMap := &selectpkg.SelectedCodeMap{
		Task:       tf,
		Candidates: candidates,
	}
	cf, err := os.Create(contextPath)
	if err != nil {
		fatal("creating context file: %v", err)
	}
	defer cf.Close()

	if err := selectpkg.RenderContext(selectedMap, cf); err != nil {
		fatal("rendering context: %v", err)
	}

	// Log select event for statistics.
	_ = stats.Log(cacheDir, &stats.Event{
		Type:          stats.EventSelect,
		Timestamp:     time.Now(),
		TaskFile:      taskPath,
		TaskBody:      tf.Body,
		SelectedFiles: selectedFiles,
		SelectedCount: len(selectedFiles),
		TotalIndexed:  len(cm.Entries),
	})

	fmt.Printf("codemap select complete\n")
	fmt.Printf("  task:     %s\n", taskPath)
	fmt.Printf("  selected: %d files\n", len(candidates))
	fmt.Printf("  files:    %s\n", filesPath)
	fmt.Printf("  context:  %s\n", contextPath)
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

	r := stats.Compute(events, gitChanges)
	stats.Print(r, os.Stdout)
}

// getGitChanges returns files changed in recent commits, keyed by task file.
// If evalTask is specified, only that task file is evaluated.
// Otherwise, all logged task files are evaluated against git changes.
func getGitChanges(repoRoot, evalTask string, numCommits int) map[string][]string {
	result := make(map[string][]string)

	// Get files changed in last N commits.
	cmd := exec.Command("git", "diff", "--name-only", fmt.Sprintf("HEAD~%d", numCommits))
	cmd.Dir = repoRoot
	out, err := cmd.Output()
	if err != nil {
		// Fallback: try fewer commits.
		cmd = exec.Command("git", "diff", "--name-only", "HEAD~1")
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
  codemap statistics                   Show usage stats and selection accuracy
  codemap statistics --eval            Evaluate selection accuracy against git changes
  codemap statistics --eval --task X   Evaluate a specific task file
  codemap doctor                       Report cache health and diagnostics`)
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
