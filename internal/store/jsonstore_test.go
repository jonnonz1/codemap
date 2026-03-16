package store

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jonnonz1/codemap/internal/model"
)

func TestJSONStoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "cache", "map.json")
	jsonlPath := filepath.Join(dir, "cache", "map.jsonl")
	s := NewJSONStore(jsonPath, jsonlPath)

	// Load from missing file should return empty map.
	cm, err := s.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(cm.Entries) != 0 {
		t.Errorf("expected empty entries, got %d", len(cm.Entries))
	}

	// Add an entry and save.
	cm.Entries["main.go"] = &model.CodeMapEntry{
		Path:            "main.go",
		Language:        "go",
		Blake3:          "abc123",
		PublicFunctions: []string{"Main"},
	}
	if err := s.Save(cm); err != nil {
		t.Fatalf("Save() error: %v", err)
	}

	// Reload and verify.
	cm2, err := s.Load()
	if err != nil {
		t.Fatalf("Load() after save error: %v", err)
	}
	e, ok := cm2.Entries["main.go"]
	if !ok {
		t.Fatal("entry main.go not found after reload")
	}
	if e.Blake3 != "abc123" {
		t.Errorf("blake3 = %q, want %q", e.Blake3, "abc123")
	}
	if len(e.PublicFunctions) != 1 || e.PublicFunctions[0] != "Main" {
		t.Errorf("public_functions = %v, want [Main]", e.PublicFunctions)
	}
}

func TestAppendChanged(t *testing.T) {
	dir := t.TempDir()
	jsonlPath := filepath.Join(dir, "cache", "map.jsonl")
	s := NewJSONStore(filepath.Join(dir, "cache", "map.json"), jsonlPath)

	entries := []*model.CodeMapEntry{
		{Path: "a.go", Language: "go"},
		{Path: "b.go", Language: "go"},
	}
	if err := s.AppendChanged(entries); err != nil {
		t.Fatalf("AppendChanged() error: %v", err)
	}

	data, err := os.ReadFile(jsonlPath)
	if err != nil {
		t.Fatalf("reading JSONL: %v", err)
	}
	// Should have 2 lines (each JSON line ends with \n).
	lines := 0
	for _, b := range data {
		if b == '\n' {
			lines++
		}
	}
	if lines != 2 {
		t.Errorf("expected 2 JSONL lines, got %d", lines)
	}
}
