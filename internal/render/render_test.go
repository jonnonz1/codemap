package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/codemap/internal/model"
)

func TestMarkdown(t *testing.T) {
	cm := model.NewCodeMap()
	cm.Entries["b.go"] = &model.CodeMapEntry{
		Path:            "b.go",
		Language:        "go",
		Summary:         "Utility helpers",
		WhenToUse:       "When you need helpers",
		PublicTypes:     []string{"Helper"},
		PublicFunctions: []string{"DoStuff"},
		Imports:         []string{"fmt"},
		Keywords:        []string{"util"},
		TestFiles:       []string{"b_test.go"},
	}
	cm.Entries["a.go"] = &model.CodeMapEntry{
		Path:     "a.go",
		Language: "go",
		Summary:  "Main entry point",
	}

	var buf bytes.Buffer
	if err := Markdown(cm, &buf); err != nil {
		t.Fatalf("Markdown() error: %v", err)
	}

	out := buf.String()

	// Verify sorting: a.go should appear before b.go.
	aIdx := strings.Index(out, "- a.go")
	bIdx := strings.Index(out, "- b.go")
	if aIdx < 0 || bIdx < 0 {
		t.Fatalf("missing entries in output:\n%s", out)
	}
	if aIdx > bIdx {
		t.Error("expected a.go before b.go in sorted output")
	}

	// Verify fields rendered for b.go.
	checks := []string{
		"summary: Utility helpers",
		"when to use: When you need helpers",
		"public types: Helper",
		"public functions: DoStuff",
		"imports: fmt",
		"keywords: util",
		"test files: b_test.go",
	}
	for _, check := range checks {
		if !strings.Contains(out, check) {
			t.Errorf("output missing %q", check)
		}
	}

	// Verify header.
	if !strings.HasPrefix(out, "# Code Map\n") {
		t.Error("output should start with '# Code Map'")
	}
}
