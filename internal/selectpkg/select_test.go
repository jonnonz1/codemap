package selectpkg

import (
	"testing"

	"github.com/jonnonz1/codemap/internal/model"
	"github.com/jonnonz1/codemap/internal/taskfile"
)

func TestSelectBasic(t *testing.T) {
	cm := model.NewCodeMap()
	cm.Entries["src/invoices/create.go"] = &model.CodeMapEntry{
		Path:            "src/invoices/create.go",
		Language:        "go",
		Summary:         "Creates new invoices",
		WhenToUse:       "When creating invoices",
		PublicFunctions: []string{"CreateInvoice"},
	}
	cm.Entries["src/invoices/delete.go"] = &model.CodeMapEntry{
		Path:            "src/invoices/delete.go",
		Language:        "go",
		Summary:         "Deletes invoices",
		WhenToUse:       "When deleting invoices",
		PublicFunctions: []string{"DeleteInvoice"},
	}
	cm.Entries["src/users/user.go"] = &model.CodeMapEntry{
		Path:            "src/users/user.go",
		Language:        "go",
		Summary:         "User management",
		PublicTypes:     []string{"User"},
	}

	tf := &taskfile.TaskFile{
		ContextGlobs: []string{"src/invoices/**"},
		MaxFiles:     10,
		Body:         "Add soft-delete support to invoices",
	}

	results := Select(cm, tf)

	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	// Invoice files should rank higher than user files.
	first := results[0].Entry.Path
	if first != "src/invoices/create.go" && first != "src/invoices/delete.go" {
		t.Errorf("expected invoice file first, got %q", first)
	}
}

func TestSelectDeterministic(t *testing.T) {
	cm := model.NewCodeMap()
	for _, name := range []string{"c.go", "a.go", "b.go"} {
		cm.Entries[name] = &model.CodeMapEntry{
			Path: name, Language: "go",
		}
	}

	tf := &taskfile.TaskFile{MaxFiles: 10, Body: "something"}

	// Run multiple times — results must be identical.
	first := Select(cm, tf)
	for i := 0; i < 10; i++ {
		got := Select(cm, tf)
		if len(got) != len(first) {
			t.Fatalf("run %d: count %d != %d", i, len(got), len(first))
		}
		for j := range got {
			if got[j].Entry.Path != first[j].Entry.Path {
				t.Fatalf("run %d: result[%d] = %q, want %q", i, j, got[j].Entry.Path, first[j].Entry.Path)
			}
		}
	}
}

func TestSelectMaxFiles(t *testing.T) {
	cm := model.NewCodeMap()
	for i := 0; i < 30; i++ {
		path := "src/file" + string(rune('a'+i)) + ".go"
		cm.Entries[path] = &model.CodeMapEntry{
			Path:     path,
			Language: "go",
		}
	}

	tf := &taskfile.TaskFile{
		MaxFiles: 5,
		Body:     "do something",
	}

	results := Select(cm, tf)
	if len(results) > 5 {
		t.Errorf("expected at most 5 results, got %d", len(results))
	}
}

func TestSelectNoGlobs(t *testing.T) {
	cm := model.NewCodeMap()
	cm.Entries["main.go"] = &model.CodeMapEntry{
		Path: "main.go", Language: "go", Summary: "Entry point",
	}

	tf := &taskfile.TaskFile{
		MaxFiles: 10,
		Body:     "fix the entry point",
	}

	results := Select(cm, tf)
	if len(results) == 0 {
		t.Error("expected results when no globs specified")
	}
}

func TestTokenize(t *testing.T) {
	words := tokenize("Add soft-delete support to INVOICES!")
	expected := []string{"add", "soft", "delete", "support", "to", "invoices"}

	if len(words) != len(expected) {
		t.Fatalf("tokenize count = %d, want %d: %v", len(words), len(expected), words)
	}
	for i, w := range words {
		if w != expected[i] {
			t.Errorf("word[%d] = %q, want %q", i, w, expected[i])
		}
	}
}

func TestMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"src/**", "src/foo.go", true},
		{"src/**", "src/bar/baz.go", true},
		{"docs/**", "src/foo.go", false},
		{"**/*.go", "src/foo.go", true},
		{"src/invoices/**", "src/invoices/create.go", true},
		{"src/invoices/**", "src/users/user.go", false},
		{"src/*/models/**", "src/billing/models/invoice.go", true},
		{"src/*/models/**", "src/billing/handlers/handler.go", false},
	}

	for _, tc := range tests {
		got := matchGlob(tc.pattern, tc.path)
		if got != tc.want {
			t.Errorf("matchGlob(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

func TestExpandImportsModuleAware(t *testing.T) {
	cm := model.NewCodeMap()
	cm.Entries["internal/scan/scan.go"] = &model.CodeMapEntry{
		Path: "internal/scan/scan.go", Language: "go",
		Imports: []string{"github.com/jonnonz1/codemap/internal/model"},
	}
	cm.Entries["internal/model/entry.go"] = &model.CodeMapEntry{
		Path: "internal/model/entry.go", Language: "go",
	}
	// This file is in "fmt" directory — should NOT match import "fmt".
	cm.Entries["fmt/helper.go"] = &model.CodeMapEntry{
		Path: "fmt/helper.go", Language: "go",
	}

	selected := []Candidate{
		{Entry: cm.Entries["internal/scan/scan.go"], Score: 5.0},
	}

	result := expandImports(selected, cm, 10)

	// Should expand to include internal/model/entry.go.
	found := false
	for _, c := range result {
		if c.Entry.Path == "internal/model/entry.go" {
			found = true
		}
		if c.Entry.Path == "fmt/helper.go" {
			t.Error("fmt/helper.go should NOT match stdlib import 'fmt'")
		}
	}
	if !found {
		t.Error("expected internal/model/entry.go to be added via import expansion")
	}
}
