// codemap is an incremental repo intelligence and context-selection CLI tool.
//
// Commands:
//
//	codemap init               Initialize codemap in a project directory
//	codemap build              Scan repo and build/update the code map cache
//	codemap render             Render the code map as markdown
//	codemap select --task PATH Select relevant files for a coding task
//	codemap doctor             Report cache health and diagnostics
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/codemap/internal/build"
	"github.com/codemap/internal/config"
	"github.com/codemap/internal/doctor"
	"github.com/codemap/internal/initcmd"
	"github.com/codemap/internal/langs/golang"
	"github.com/codemap/internal/llm"
	"github.com/codemap/internal/parse"
	"github.com/codemap/internal/render"
	"github.com/codemap/internal/selectpkg"
	"github.com/codemap/internal/store"
	"github.com/codemap/internal/taskfile"
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
		runBuild(repoRoot, st, cfg)
	case "render":
		runRender(st, mdPath)
	case "select":
		runSelect(st, cacheDir)
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
	opts := initcmd.Options{RepoRoot: repoRoot}

	// Parse flags: --provider, --model
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
		}
	}

	res, err := initcmd.Run(opts)
	if err != nil {
		fatal("init: %v", err)
	}
	initcmd.Print(res, os.Stdout)
}

func runBuild(repoRoot string, st store.Store, cfg *config.Config) {
	reg := parse.NewRegistry()
	reg.Register(&golang.Parser{})

	summarizer := newSummarizer(cfg)

	res, err := build.Run(repoRoot, st, reg, summarizer)
	if err != nil {
		fatal("build: %v", err)
	}

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

	var lines []string
	for _, c := range candidates {
		lines = append(lines, c.Entry.Path)
	}
	if err := os.WriteFile(filesPath, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
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

	fmt.Printf("codemap select complete\n")
	fmt.Printf("  task:     %s\n", taskPath)
	fmt.Printf("  selected: %d files\n", len(candidates))
	fmt.Printf("  files:    %s\n", filesPath)
	fmt.Printf("  context:  %s\n", contextPath)
}

func runDoctor(repoRoot string, st store.Store) {
	r, err := doctor.Run(repoRoot, st)
	if err != nil {
		fatal("doctor: %v", err)
	}
	doctor.Print(r, os.Stdout)
}

// newSummarizer returns a Summarizer based on the project config.
// Currently only "mock" is implemented; other providers return mock
// with a warning.
func newSummarizer(cfg *config.Config) llm.Summarizer {
	switch cfg.LLM.Provider {
	case "mock", "":
		return &llm.MockSummarizer{}
	default:
		fmt.Fprintf(os.Stderr, "codemap: LLM provider %q not yet implemented, using mock\n", cfg.LLM.Provider)
		return &llm.MockSummarizer{}
	}
}

func printUsage() {
	fmt.Println(`codemap - incremental repo intelligence and context-selection tool

Usage:
  codemap init                         Initialize codemap in a project
  codemap init --provider anthropic    Initialize with LLM provider
  codemap init --model <model-id>      Initialize with specific model
  codemap build                        Scan repo and build/update the code map cache
  codemap render                       Render the code map as markdown
  codemap select --task PATH           Select relevant files for a coding task
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
