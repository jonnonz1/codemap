// Package initcmd implements the codemap init command, which sets up a
// project directory with codemap configuration and Claude Code integration.
package initcmd

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/jonnonz1/codemap/internal/config"
)

// Options controls what codemap init creates.
type Options struct {
	RepoRoot    string
	Provider    string // "mock", "anthropic", "openai", "google"
	Model       string
	APIKey      string
	Interactive bool // if true, prompt user for missing values
}

// Result reports what init did.
type Result struct {
	Created []string
	Updated []string
	Skipped []string
}

// Run initializes codemap in the given project directory.
func Run(opts Options) (*Result, error) {
	r := &Result{}

	// Interactive prompts if no flags provided.
	if opts.Interactive && opts.Provider == "" {
		promptUser(&opts)
	}

	// 1. Write .codemap.yaml
	if err := writeConfig(opts, r); err != nil {
		return r, fmt.Errorf("writing config: %w", err)
	}

	// 2. Create cache directory.
	cacheDir := filepath.Join(opts.RepoRoot, ".claude", "cache")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return r, fmt.Errorf("creating cache dir: %w", err)
	}

	// 3. Update .gitignore.
	if err := updateGitignore(opts.RepoRoot, r); err != nil {
		return r, fmt.Errorf("updating .gitignore: %w", err)
	}

	// 4. Create or update CLAUDE.md with codemap section.
	if err := updateClaudeMD(opts.RepoRoot, r); err != nil {
		return r, fmt.Errorf("updating CLAUDE.md: %w", err)
	}

	// 5. Create example task file.
	if err := writeExampleTask(opts.RepoRoot, r); err != nil {
		return r, fmt.Errorf("writing example task: %w", err)
	}

	return r, nil
}

func promptUser(opts *Options) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("codemap init")
	fmt.Println()

	// Provider selection.
	fmt.Println("Select LLM provider for file summaries:")
	fmt.Println("  1) anthropic  (Claude — recommended)")
	fmt.Println("  2) openai     (GPT)")
	fmt.Println("  3) google     (Gemini)")
	fmt.Println("  4) mock       (no LLM, placeholder summaries)")
	fmt.Println()
	fmt.Print("Provider [1]: ")
	choice := readLine(reader)

	switch choice {
	case "", "1", "anthropic":
		opts.Provider = "anthropic"
	case "2", "openai":
		opts.Provider = "openai"
	case "3", "google":
		opts.Provider = "google"
	case "4", "mock":
		opts.Provider = "mock"
	default:
		opts.Provider = "anthropic"
	}

	if opts.Provider == "mock" {
		return
	}

	// Model.
	defaultModel := defaultModelFor(opts.Provider)
	fmt.Printf("Model [%s]: ", defaultModel)
	model := readLine(reader)
	if model == "" {
		opts.Model = defaultModel
	} else {
		opts.Model = model
	}

	// API key.
	fmt.Printf("API key (stored in .codemap.yaml): ")
	key := readLine(reader)
	if key != "" {
		opts.APIKey = key
	}

	fmt.Println()
}

func defaultModelFor(provider string) string {
	switch provider {
	case "anthropic":
		return "claude-haiku-4-5-20251001"
	case "openai":
		return "gpt-4o-mini"
	case "google":
		return "gemini-2.0-flash"
	default:
		return ""
	}
}

func readLine(reader *bufio.Reader) string {
	line, _ := reader.ReadString('\n')
	return strings.TrimSpace(line)
}

// Print writes the init result to w.
func Print(r *Result, w io.Writer) {
	fmt.Fprintln(w, "codemap init complete")
	fmt.Fprintln(w)
	for _, f := range r.Created {
		fmt.Fprintf(w, "  [+] created  %s\n", f)
	}
	for _, f := range r.Updated {
		fmt.Fprintf(w, "  [~] updated  %s\n", f)
	}
	for _, f := range r.Skipped {
		fmt.Fprintf(w, "  [-] skipped  %s (already exists)\n", f)
	}
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Next steps:")
	fmt.Fprintln(w, "  codemap build          # index your repository")
	fmt.Fprintln(w, "  codemap render         # render markdown code map")
	fmt.Fprintln(w, "  codemap doctor         # verify everything is working")
}

func writeConfig(opts Options, r *Result) error {
	path := filepath.Join(opts.RepoRoot, config.FileName)

	if fileExists(path) {
		r.Skipped = append(r.Skipped, config.FileName)
		return nil
	}

	cfg := config.Default()
	if opts.Provider != "" {
		cfg.LLM.Provider = opts.Provider
	}
	if opts.Model != "" {
		cfg.LLM.Model = opts.Model
	}
	if opts.APIKey != "" {
		cfg.LLM.APIKey = opts.APIKey
	}

	// Set fallback env var name based on provider.
	switch cfg.LLM.Provider {
	case "anthropic":
		cfg.LLM.APIKeyEnv = "ANTHROPIC_API_KEY"
	case "openai":
		cfg.LLM.APIKeyEnv = "OPENAI_API_KEY"
	case "google":
		cfg.LLM.APIKeyEnv = "GOOGLE_API_KEY"
	}

	if err := config.Save(cfg, path); err != nil {
		return err
	}
	r.Created = append(r.Created, config.FileName)
	return nil
}

func updateGitignore(root string, r *Result) error {
	path := filepath.Join(root, ".gitignore")
	requiredLines := []string{
		"# codemap",
		".claude/cache/",
		".codemap.yaml",
	}

	existing, _ := os.ReadFile(path)
	content := string(existing)

	if strings.Contains(content, ".claude/cache/") {
		return nil
	}

	addition := "\n" + strings.Join(requiredLines, "\n") + "\n"
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := f.WriteString(addition); err != nil {
		return err
	}
	r.Updated = append(r.Updated, ".gitignore")
	return nil
}

func updateClaudeMD(root string, r *Result) error {
	path := filepath.Join(root, "CLAUDE.md")
	marker := "<!-- codemap:begin -->"

	existing, _ := os.ReadFile(path)
	content := string(existing)

	if strings.Contains(content, marker) {
		r.Skipped = append(r.Skipped, "CLAUDE.md (codemap section)")
		return nil
	}

	section := codemapClaudeSection()

	if len(existing) == 0 {
		if err := os.WriteFile(path, []byte(section), 0o644); err != nil {
			return err
		}
		r.Created = append(r.Created, "CLAUDE.md")
	} else {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()

		if _, err := f.WriteString("\n" + section); err != nil {
			return err
		}
		r.Updated = append(r.Updated, "CLAUDE.md")
	}
	return nil
}

func codemapClaudeSection() string {
	return `<!-- codemap:begin -->
## Code Map

**IMPORTANT: Before making any code changes, read ` + "`.claude/cache/context-code-map.md`" + ` to understand
the structure and purpose of files in this codebase.** This file contains a pre-indexed map of every
source file with summaries, exported types, functions, and imports.

If the code map file is missing or stale, regenerate it:

` + "```bash" + `
codemap build && codemap render
` + "```" + `

### Task-scoped context

For focused work, check if ` + "`.claude/cache/selected-context.md`" + ` exists — it contains
pre-selected files relevant to the current task. To generate it:

1. Create a task file in ` + "`tasks/`" + ` (see ` + "`tasks/example.md`" + ` for the format)
2. Run ` + "`codemap select --task <file>`" + `
3. Read ` + "`.claude/cache/selected-context.md`" + ` for the relevant files and summaries

### Checking freshness

Run ` + "`codemap doctor`" + ` to check if the code map needs rebuilding.
<!-- codemap:end -->
`
}

func writeExampleTask(root string, r *Result) error {
	dir := filepath.Join(root, "tasks")
	path := filepath.Join(dir, "example.md")

	if fileExists(path) {
		r.Skipped = append(r.Skipped, "tasks/example.md")
		return nil
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	content := `---
# Globs for files directly relevant to this task (scored highest).
context_globs: []
#  - src/feature/**
#  - tests/feature/**

# Globs for reference files that provide useful context (scored lower).
knowledge_globs: []
#  - docs/**
#  - src/core/**

# Maximum number of files to select.
max_files: 12

# Maximum token budget for selected context (not yet enforced).
max_tokens: 50000
---

Describe your task here. Be specific about what you want to change,
which patterns to follow, and what tests to update.

The more specific you are, the better codemap can select relevant files.
`

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return err
	}
	r.Created = append(r.Created, "tasks/example.md")
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
