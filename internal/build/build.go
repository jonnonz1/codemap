// Package build orchestrates the incremental code map build pipeline:
// scan files, check mtime/blake3 for changes, parse, summarize, and persist.
package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jonnonz1/codemap/internal/hash"
	"github.com/jonnonz1/codemap/internal/llm"
	"github.com/jonnonz1/codemap/internal/model"
	"github.com/jonnonz1/codemap/internal/parse"
	"github.com/jonnonz1/codemap/internal/scan"
	"github.com/jonnonz1/codemap/internal/store"
)

// Result summarizes what happened during a build.
type Result struct {
	TotalFiles  int
	Unchanged   int
	Updated     int
	Added       int
	Removed     int
	ParseErrors int
}

// Run performs an incremental code map build rooted at repoRoot.
func Run(repoRoot string, st store.Store, registry *parse.Registry, summarizer llm.Summarizer) (*Result, error) {
	files, err := scan.Dir(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("scanning: %w", err)
	}

	existing, err := st.Load()
	if err != nil {
		return nil, fmt.Errorf("loading cache: %w", err)
	}

	// Pre-index test files by directory for O(1) lookup.
	testIndex := buildTestIndex(files)

	res := &Result{TotalFiles: len(files)}
	var changed []*model.CodeMapEntry
	seen := make(map[string]bool)

	for _, fi := range files {
		seen[fi.Path] = true

		prev, exists := existing.Entries[fi.Path]

		// Fast path: mtime unchanged — skip entirely.
		if exists && prev.ModTimeUnix == fi.ModTime {
			res.Unchanged++
			continue
		}

		// Read file content for hashing.
		data, err := os.ReadFile(filepath.Join(repoRoot, fi.Path))
		if err != nil {
			continue
		}

		h := hash.Blake3Hex(data)

		// If hash unchanged despite mtime change, just update mtime.
		// Copy into new entry rather than mutating the shared map value.
		if exists && prev.Blake3 == h {
			updated := *prev
			updated.ModTimeUnix = fi.ModTime
			existing.Entries[fi.Path] = &updated
			res.Unchanged++
			continue
		}

		// Build new entry.
		entry := &model.CodeMapEntry{
			Path:        fi.Path,
			Language:    fi.Language,
			ModTimeUnix: fi.ModTime,
			Blake3:      h,
		}

		// Run language parser if available.
		ext := filepath.Ext(fi.Path)
		if p := registry.ForExtension(ext); p != nil {
			if err := p.Parse(data, entry); err != nil {
				res.ParseErrors++
			}
		}

		// Associate test files from pre-built index.
		entry.TestFiles = testFilesForPath(fi.Path, testIndex)

		// Run summarizer.
		if summarizer != nil {
			sr, err := summarizer.Summarize(fi.Path, data)
			if err == nil {
				entry.Summary = sr.Summary
				entry.WhenToUse = sr.WhenToUse
				entry.Keywords = sr.Keywords
			}
		}

		existing.Entries[fi.Path] = entry
		changed = append(changed, entry)

		if exists {
			res.Updated++
		} else {
			res.Added++
		}
	}

	// Remove entries for files that no longer exist.
	for path := range existing.Entries {
		if !seen[path] {
			delete(existing.Entries, path)
			res.Removed++
		}
	}

	// Persist.
	if err := st.Save(existing); err != nil {
		return nil, fmt.Errorf("saving cache: %w", err)
	}
	if err := st.AppendChanged(changed); err != nil {
		return nil, fmt.Errorf("appending JSONL: %w", err)
	}

	return res, nil
}

// buildTestIndex groups test file paths by their directory. This is built
// once per build so that testFilesForPath is O(1) per lookup instead of O(n).
func buildTestIndex(files []scan.FileInfo) map[string][]string {
	idx := make(map[string][]string)
	for _, f := range files {
		if strings.HasSuffix(f.Path, "_test.go") {
			dir := filepath.Dir(f.Path)
			idx[dir] = append(idx[dir], f.Path)
		}
	}
	return idx
}

// testFilesForPath returns test files in the same directory as path.
// Returns nil for test files themselves to avoid circular references.
func testFilesForPath(path string, idx map[string][]string) []string {
	if strings.HasSuffix(filepath.Base(path), "_test.go") {
		return nil
	}
	return idx[filepath.Dir(path)]
}
