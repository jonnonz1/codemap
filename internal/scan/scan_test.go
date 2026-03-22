package scan

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/jonnonz1/codemap/internal/config"
)

func TestDir(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "main.go", "package main")
	writeFile(t, root, "pkg/util.go", "package pkg")
	writeFile(t, root, "node_modules/dep.js", "var x")
	writeFile(t, root, "api.pb.go", "package api")
	writeFile(t, root, ".hidden.go", "package hidden")
	writeFile(t, root, "data.csv", "a,b,c")
	writeFile(t, root, ".git/config", "bare = false")
	writeFile(t, root, "README.md", "# hello")
	writeFile(t, root, "config.json", "{}")
	writeFile(t, root, "schema.yaml", "key: val")

	files, err := Dir(root, nil)
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}

	got := make(map[string]string)
	for _, f := range files {
		got[f.Path] = f.Language
	}

	tests := []struct {
		path   string
		lang   string
		exists bool
	}{
		{"main.go", "go", true},
		{"pkg/util.go", "go", true},
		{"node_modules/dep.js", "", false},
		{"api.pb.go", "", false},
		{".hidden.go", "", false},
		{"data.csv", "", false},
		{".git/config", "", false},
		{"README.md", "", false},
		{"config.json", "", false},
		{"schema.yaml", "", false},
	}

	for _, tc := range tests {
		lang, found := got[tc.path]
		if found != tc.exists {
			t.Errorf("path %q: found=%v, want found=%v", tc.path, found, tc.exists)
		}
		if found && lang != tc.lang {
			t.Errorf("path %q: lang=%q, want %q", tc.path, lang, tc.lang)
		}
	}
}

func TestDirNoDefaults(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "main.go", "package main")
	writeFile(t, root, "README.md", "# hello")
	writeFile(t, root, "config.json", "{}")

	cfg := &config.ScanConfig{NoDefaults: true}
	files, err := Dir(root, cfg)
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}

	got := make(map[string]string)
	for _, f := range files {
		got[f.Path] = f.Language
	}

	if _, ok := got["README.md"]; !ok {
		t.Error("README.md should be included when no_defaults is true")
	}
	if _, ok := got["config.json"]; !ok {
		t.Error("config.json should be included when no_defaults is true")
	}
}

func TestDirCustomIgnore(t *testing.T) {
	root := t.TempDir()

	writeFile(t, root, "main.go", "package main")
	writeFile(t, root, "generated_api.go", "package main")
	writeFile(t, root, "build/output.go", "package build")

	cfg := &config.ScanConfig{
		IgnorePatterns: []string{"generated_*.go"},
		IgnoreDirs:     []string{"build"},
	}
	files, err := Dir(root, cfg)
	if err != nil {
		t.Fatalf("Dir() error: %v", err)
	}

	got := make(map[string]string)
	for _, f := range files {
		got[f.Path] = f.Language
	}

	if _, ok := got["main.go"]; !ok {
		t.Error("main.go should be included")
	}
	if _, ok := got["generated_api.go"]; ok {
		t.Error("generated_api.go should be excluded by custom pattern")
	}
	if _, ok := got["build/output.go"]; ok {
		t.Error("build/output.go should be excluded by custom ignore dir")
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
