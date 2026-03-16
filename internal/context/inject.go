// Package context generates the compact context payload that gets injected
// into Claude Code sessions via the SessionStart hook.
package context

import (
	"fmt"
	"io"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jonnonz1/codemap/internal/model"
	"github.com/jonnonz1/codemap/internal/store"
)

// Inject writes a compact code map summary to w, designed to be injected
// into Claude's context at session start. Includes directory tree and
// per-file summaries truncated to fit within a reasonable token budget.
func Inject(st store.Store, w io.Writer) error {
	cm, err := st.Load()
	if err != nil || len(cm.Entries) == 0 {
		fmt.Fprintln(w, "No code map found. Run: codemap build && codemap render")
		return nil
	}

	fmt.Fprintf(w, "=== Code Map (%d files) ===\n\n", len(cm.Entries))

	// Directory tree summary.
	tree := buildDirTree(cm)
	fmt.Fprintln(w, "Directory structure:")
	for _, line := range tree {
		fmt.Fprintf(w, "  %s\n", line)
	}
	fmt.Fprintln(w)

	// Per-file summaries — the actual value.
	paths := sortedPaths(cm)
	fmt.Fprintln(w, "File summaries:")
	for _, path := range paths {
		e := cm.Entries[path]
		line := fmt.Sprintf("- %s", path)
		if e.Summary != "" {
			line += fmt.Sprintf(": %s", e.Summary)
		}
		fmt.Fprintln(w, line)

		if e.WhenToUse != "" {
			fmt.Fprintf(w, "    when to use: %s\n", e.WhenToUse)
		}
		if len(e.PublicTypes) > 0 {
			fmt.Fprintf(w, "    types: %s\n", strings.Join(e.PublicTypes, ", "))
		}
		if len(e.PublicFunctions) > 0 {
			fmt.Fprintf(w, "    functions: %s\n", strings.Join(e.PublicFunctions, ", "))
		}
	}

	fmt.Fprintln(w)
	fmt.Fprintln(w, "For the full code map with imports and keywords: .claude/cache/context-code-map.md")

	return nil
}

// buildDirTree returns a compact directory listing with file counts.
func buildDirTree(cm *model.CodeMap) []string {
	dirs := make(map[string]int)
	for path := range cm.Entries {
		dir := filepath.Dir(path)
		dirs[dir]++
	}

	sorted := make([]string, 0, len(dirs))
	for dir := range dirs {
		sorted = append(sorted, dir)
	}
	sort.Strings(sorted)

	var lines []string
	for _, dir := range sorted {
		lines = append(lines, fmt.Sprintf("%s/ (%d files)", dir, dirs[dir]))
	}
	return lines
}

func sortedPaths(cm *model.CodeMap) []string {
	paths := make([]string, 0, len(cm.Entries))
	for p := range cm.Entries {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths
}
