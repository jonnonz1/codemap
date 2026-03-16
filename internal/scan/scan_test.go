package scan

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDir(t *testing.T) {
	root := t.TempDir()

	// Create a valid Go file.
	writeFile(t, root, "main.go", "package main")
	// Create a file in a nested directory.
	writeFile(t, root, "pkg/util.go", "package pkg")
	// Create a file that should be ignored (node_modules).
	writeFile(t, root, "node_modules/dep.js", "var x")
	// Create a generated file.
	writeFile(t, root, "api.pb.go", "package api")
	// Create a dotfile (should be ignored).
	writeFile(t, root, ".hidden.go", "package hidden")
	// Create a file with unsupported extension.
	writeFile(t, root, "data.csv", "a,b,c")
	// Create a .git dir file (should be ignored).
	writeFile(t, root, ".git/config", "bare = false")

	files, err := Dir(root)
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
