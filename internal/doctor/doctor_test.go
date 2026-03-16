package doctor

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jonnonz1/codemap/internal/model"
	"github.com/jonnonz1/codemap/internal/store"
)

func TestRunEmpty(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, ".claude", "cache")
	st := store.NewJSONStore(
		filepath.Join(cacheDir, "context-code-map.json"),
		filepath.Join(cacheDir, "context-code-map.jsonl"),
	)

	r, err := Run(root, st)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if r.CacheExists {
		t.Error("cache should not exist yet")
	}
	if r.IndexedFiles != 0 {
		t.Errorf("indexed = %d, want 0", r.IndexedFiles)
	}
}

func TestRunWithCache(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, ".claude", "cache")
	st := store.NewJSONStore(
		filepath.Join(cacheDir, "context-code-map.json"),
		filepath.Join(cacheDir, "context-code-map.jsonl"),
	)

	// Create a cache with entries.
	cm := model.NewCodeMap()
	cm.Entries["main.go"] = &model.CodeMapEntry{
		Path: "main.go", Language: "go", Summary: "entry point",
	}
	cm.Entries["util.go"] = &model.CodeMapEntry{
		Path: "util.go", Language: "go",
	}
	if err := st.Save(cm); err != nil {
		t.Fatal(err)
	}

	// Create the actual files so they're not stale.
	writeFile(t, root, "main.go", "package main")
	writeFile(t, root, "util.go", "package main")

	r, err := Run(root, st)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if !r.CacheExists {
		t.Error("cache should exist")
	}
	if r.IndexedFiles != 2 {
		t.Errorf("indexed = %d, want 2", r.IndexedFiles)
	}
	if r.MissingSummary != 1 {
		t.Errorf("missing_summary = %d, want 1", r.MissingSummary)
	}
}

func TestRunStaleBreakdown(t *testing.T) {
	root := t.TempDir()
	cacheDir := filepath.Join(root, ".claude", "cache")
	st := store.NewJSONStore(
		filepath.Join(cacheDir, "context-code-map.json"),
		filepath.Join(cacheDir, "context-code-map.jsonl"),
	)

	cm := model.NewCodeMap()
	// This file exists on disk — will be counted as "changed" since mtime won't match.
	cm.Entries["exists.go"] = &model.CodeMapEntry{
		Path: "exists.go", Language: "go", ModTimeUnix: 1,
	}
	// This file does NOT exist on disk — will be counted as "deleted".
	cm.Entries["deleted.go"] = &model.CodeMapEntry{
		Path: "deleted.go", Language: "go", ModTimeUnix: 1,
	}
	if err := st.Save(cm); err != nil {
		t.Fatal(err)
	}

	// Create one cached file and one new file.
	writeFile(t, root, "exists.go", "package main")
	writeFile(t, root, "new.go", "package main")

	r, err := Run(root, st)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if r.StaleDeleted != 1 {
		t.Errorf("StaleDeleted = %d, want 1", r.StaleDeleted)
	}
	if r.StaleNew != 1 {
		t.Errorf("StaleNew = %d, want 1", r.StaleNew)
	}
	if r.StaleChanged != 1 {
		t.Errorf("StaleChanged = %d, want 1 (exists.go has mismatched mtime)", r.StaleChanged)
	}
}

func TestPrint(t *testing.T) {
	r := &Report{
		CacheExists:    true,
		JSONLExists:    false,
		MarkdownExists: true,
		IndexedFiles:   42,
		MissingSummary: 3,
		StaleChanged:   1,
		Languages:      map[string]int{"go": 40, "python": 2},
	}

	var buf bytes.Buffer
	Print(r, &buf)
	out := buf.String()

	if !strings.Contains(out, "[+] JSON cache") {
		t.Error("should show cache as present")
	}
	if !strings.Contains(out, "[x] JSONL log") {
		t.Error("should show JSONL as missing")
	}
	if !strings.Contains(out, "Indexed files:     42") {
		t.Error("should show indexed count")
	}
	if !strings.Contains(out, "1 changed") {
		t.Errorf("should show stale breakdown, got:\n%s", out)
	}

	// Languages should be sorted: go before python.
	goIdx := strings.Index(out, "go")
	pyIdx := strings.Index(out, "python")
	if goIdx > pyIdx {
		t.Error("languages should be sorted alphabetically")
	}
}

func TestPrintZeroStale(t *testing.T) {
	r := &Report{
		Languages: make(map[string]int),
	}
	var buf bytes.Buffer
	Print(r, &buf)
	if !strings.Contains(buf.String(), "Stale files:       0") {
		t.Error("should show 'Stale files: 0' when no stale files")
	}
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
