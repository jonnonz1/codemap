// Package scan walks a repository tree and returns file paths, respecting
// common ignore rules for directories and files that should be skipped.
package scan

import (
	"io/fs"
	"path/filepath"
	"strings"

	"github.com/jonnonz1/codemap/internal/config"
)

// ignoreDirs are directory names that are always skipped during scanning.
var ignoreDirs = map[string]bool{
	".git":         true,
	".beads":       true,
	".claude":      true,
	"node_modules": true,
	"vendor":       true,
	"dist":         true,
	".next":        true,
	"__pycache__":  true,
	".cache":       true,
	".venv":        true,
	"venv":         true,
	"target":       true, // Rust/Java build output
}

// ignoreFiles are filename patterns that are skipped.
var ignoreFiles = map[string]bool{
	".DS_Store": true,
	"Thumbs.db": true,
}

// ignoreSuffixes are file suffixes that indicate generated or binary files.
var ignoreSuffixes = []string{
	".pb.go",
	".gen.go",
	"_generated.go",
	".min.js",
	".min.css",
	".map",
	".lock",
}

// supportedExtensions maps file extensions to language identifiers.
// Only files with these extensions are indexed.
var supportedExtensions = map[string]string{
	".go":   "go",
	".ts":   "typescript",
	".tsx":  "typescript",
	".js":   "javascript",
	".jsx":  "javascript",
	".py":   "python",
	".rs":   "rust",
	".java": "java",
	".rb":   "ruby",
	".c":    "c",
	".h":    "c",
	".cpp":  "cpp",
	".hpp":  "cpp",
	".cs":   "csharp",
	".sh":   "shell",
	".yaml": "yaml",
	".yml":  "yaml",
	".json": "json",
	".toml": "toml",
	".md":   "markdown",
	".sql":  "sql",
}

// FileInfo holds the path and detected language for a scanned file.
type FileInfo struct {
	Path     string // relative path from repo root
	Language string
	ModTime  int64 // unix timestamp
}

// Dir walks the directory tree rooted at root and returns all indexable files.
// Paths are returned relative to root. Symlinks are skipped to prevent
// infinite loops and duplicate entries. Pass nil for cfg to use defaults.
func Dir(root string, cfg *config.ScanConfig) ([]FileInfo, error) {
	var files []FileInfo
	root = filepath.Clean(root)

	extraDirs := make(map[string]bool)
	var patterns []string

	if cfg != nil {
		for _, d := range cfg.IgnoreDirs {
			extraDirs[d] = true
		}
		if !cfg.NoDefaults {
			patterns = append(patterns, config.DefaultIgnorePatterns...)
		}
		patterns = append(patterns, cfg.IgnorePatterns...)
	} else {
		patterns = append(patterns, config.DefaultIgnorePatterns...)
	}

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}

		name := d.Name()

		// Skip symlinks entirely — they can cause loops or duplicates.
		if d.Type()&fs.ModeSymlink != 0 {
			return nil
		}

		if d.IsDir() {
			if ignoreDirs[name] || extraDirs[name] {
				return filepath.SkipDir
			}
			return nil
		}

		if ignoreFiles[name] {
			return nil
		}

		if strings.HasPrefix(name, ".") {
			return nil
		}

		for _, suffix := range ignoreSuffixes {
			if strings.HasSuffix(name, suffix) {
				return nil
			}
		}

		for _, pat := range patterns {
			if matched, _ := filepath.Match(pat, name); matched {
				return nil
			}
		}

		ext := filepath.Ext(name)
		lang, ok := supportedExtensions[ext]
		if !ok {
			return nil
		}

		rel, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}

		info, err := d.Info()
		if err != nil {
			return nil
		}

		files = append(files, FileInfo{
			Path:     rel,
			Language: lang,
			ModTime:  info.ModTime().Unix(),
		})
		return nil
	})

	return files, err
}
