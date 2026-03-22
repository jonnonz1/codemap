// Package doctor implements the codemap doctor diagnostic command.
package doctor

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/jonnonz1/codemap/internal/model"
	"github.com/jonnonz1/codemap/internal/scan"
	"github.com/jonnonz1/codemap/internal/store"
)

// Report holds the diagnostic results from a doctor run.
type Report struct {
	CacheExists    bool
	JSONLExists    bool
	MarkdownExists bool
	IndexedFiles   int
	MissingSummary int
	StaleChanged   int // cached files whose mtime differs from disk
	StaleDeleted   int // cached files no longer on disk
	StaleNew       int // on-disk files not yet in cache
	Languages      map[string]int
}

// Run performs diagnostics and returns a report.
func Run(repoRoot string, st store.Store) (*Report, error) {
	cacheDir := filepath.Join(repoRoot, ".claude", "cache")

	r := &Report{
		Languages: make(map[string]int),
	}

	// Check cache file existence.
	r.CacheExists = fileExists(filepath.Join(cacheDir, "context-code-map.json"))
	r.JSONLExists = fileExists(filepath.Join(cacheDir, "context-code-map.jsonl"))
	r.MarkdownExists = fileExists(filepath.Join(cacheDir, "context-code-map.md"))

	// Load cache for analysis.
	cm, err := st.Load()
	if err != nil {
		return r, nil // non-fatal — just means cache is empty/missing
	}

	r.IndexedFiles = len(cm.Entries)

	for _, e := range cm.Entries {
		if e.Summary == "" {
			r.MissingSummary++
		}
		r.Languages[e.Language]++
	}

	// Compare current files to cache for staleness.
	files, err := scan.Dir(repoRoot, nil)
	if err == nil {
		currentFiles := make(map[string]int64)
		for _, f := range files {
			currentFiles[f.Path] = f.ModTime
		}
		countStale(cm, currentFiles, r)
	}

	return r, nil
}

// Print writes the report to w in a human-readable format.
func Print(r *Report, w io.Writer) {
	fmt.Fprintln(w, "codemap doctor")
	fmt.Fprintln(w, "==============")
	fmt.Fprintln(w)

	printCheck(w, "JSON cache", r.CacheExists)
	printCheck(w, "JSONL log", r.JSONLExists)
	printCheck(w, "Markdown render", r.MarkdownExists)
	fmt.Fprintln(w)

	fmt.Fprintf(w, "Indexed files:     %d\n", r.IndexedFiles)
	fmt.Fprintf(w, "Missing summaries: %d\n", r.MissingSummary)

	total := r.StaleChanged + r.StaleDeleted + r.StaleNew
	if total > 0 {
		fmt.Fprintf(w, "Stale:             %d (%d changed, %d deleted, %d new)\n",
			total, r.StaleChanged, r.StaleDeleted, r.StaleNew)
	} else {
		fmt.Fprintf(w, "Stale files:       0\n")
	}
	fmt.Fprintln(w)

	if len(r.Languages) > 0 {
		fmt.Fprintln(w, "Languages:")
		langs := make([]string, 0, len(r.Languages))
		for lang := range r.Languages {
			langs = append(langs, lang)
		}
		sort.Strings(langs)
		for _, lang := range langs {
			fmt.Fprintf(w, "  %-15s %d files\n", lang, r.Languages[lang])
		}
	}
}

func printCheck(w io.Writer, label string, ok bool) {
	mark := "x"
	if ok {
		mark = "+"
	}
	fmt.Fprintf(w, "  [%s] %s\n", mark, label)
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// countStale populates the StaleChanged, StaleDeleted, and StaleNew fields
// on the report by comparing cache entries against current disk state.
func countStale(cm *model.CodeMap, current map[string]int64, r *Report) {
	for path, e := range cm.Entries {
		modTime, exists := current[path]
		if !exists {
			r.StaleDeleted++
		} else if modTime != e.ModTimeUnix {
			r.StaleChanged++
		}
	}
	for path := range current {
		if _, exists := cm.Entries[path]; !exists {
			r.StaleNew++
		}
	}
}
