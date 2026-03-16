package initcmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codemap/internal/config"
)

func TestRunCreatesAll(t *testing.T) {
	root := t.TempDir()

	// Create .git so it looks like a repo.
	if err := os.Mkdir(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}

	res, err := Run(Options{RepoRoot: root})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Config should be created.
	assertFileExists(t, filepath.Join(root, config.FileName))
	assertInSlice(t, "created", res.Created, config.FileName)

	// CLAUDE.md should be created.
	assertFileExists(t, filepath.Join(root, "CLAUDE.md"))
	claudeContent, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if !strings.Contains(string(claudeContent), "codemap:begin") {
		t.Error("CLAUDE.md should contain codemap section marker")
	}
	if !strings.Contains(string(claudeContent), "codemap build") {
		t.Error("CLAUDE.md should contain codemap instructions")
	}

	// .gitignore should be updated.
	gitignore, _ := os.ReadFile(filepath.Join(root, ".gitignore"))
	if !strings.Contains(string(gitignore), ".claude/cache/") {
		t.Error(".gitignore should contain .claude/cache/")
	}

	// Example task should be created.
	assertFileExists(t, filepath.Join(root, "tasks", "example.md"))

	// Cache dir should exist.
	assertFileExists(t, filepath.Join(root, ".claude", "cache"))
}

func TestRunIdempotent(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, ".git"), 0o755)

	// Run twice.
	_, err := Run(Options{RepoRoot: root})
	if err != nil {
		t.Fatalf("first Run() error: %v", err)
	}

	res, err := Run(Options{RepoRoot: root})
	if err != nil {
		t.Fatalf("second Run() error: %v", err)
	}

	// Everything should be skipped on second run.
	if len(res.Created) != 0 {
		t.Errorf("expected no created files on second run, got %v", res.Created)
	}
	for _, s := range res.Skipped {
		t.Logf("skipped: %s", s)
	}
}

func TestRunWithProvider(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, ".git"), 0o755)

	_, err := Run(Options{
		RepoRoot: root,
		Provider: "anthropic",
		Model:    "claude-haiku-4-5-20251001",
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	cfg, err := config.Load(filepath.Join(root, config.FileName))
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}

	if cfg.LLM.Provider != "anthropic" {
		t.Errorf("provider = %q, want %q", cfg.LLM.Provider, "anthropic")
	}
	if cfg.LLM.Model != "claude-haiku-4-5-20251001" {
		t.Errorf("model = %q, want %q", cfg.LLM.Model, "claude-haiku-4-5-20251001")
	}
	if cfg.LLM.APIKeyEnv != "ANTHROPIC_API_KEY" {
		t.Errorf("api_key_env = %q, want %q", cfg.LLM.APIKeyEnv, "ANTHROPIC_API_KEY")
	}
}

func TestRunAppendsToExistingCLAUDE(t *testing.T) {
	root := t.TempDir()
	os.Mkdir(filepath.Join(root, ".git"), 0o755)

	// Create existing CLAUDE.md.
	existing := "# My Project\n\nSome existing content.\n"
	os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte(existing), 0o644)

	res, err := Run(Options{RepoRoot: root})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	assertInSlice(t, "updated", res.Updated, "CLAUDE.md")

	content, _ := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	s := string(content)
	if !strings.HasPrefix(s, "# My Project") {
		t.Error("existing content should be preserved")
	}
	if !strings.Contains(s, "codemap:begin") {
		t.Error("codemap section should be appended")
	}
}

func assertFileExists(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Errorf("expected file to exist: %s", path)
	}
}

func assertInSlice(t *testing.T, label string, slice []string, want string) {
	t.Helper()
	for _, s := range slice {
		if s == want {
			return
		}
	}
	t.Errorf("%s: missing %q in %v", label, want, slice)
}
