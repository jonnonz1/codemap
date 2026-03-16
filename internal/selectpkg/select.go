// Package selectpkg implements deterministic file selection and scoring
// for codemap select --task.
package selectpkg

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/codemap/internal/model"
	"github.com/codemap/internal/taskfile"
)

// Candidate is a file with its computed relevance score.
type Candidate struct {
	Entry *model.CodeMapEntry
	Score float64
}

// Select picks the most relevant files from the code map for the given task.
// It returns candidates sorted by score descending, capped to tf.MaxFiles.
func Select(cm *model.CodeMap, tf *taskfile.TaskFile) []Candidate {
	// Determine candidate pool by matching globs.
	allGlobs := append(tf.KnowledgeGlobs, tf.ContextGlobs...)
	candidates := filterByGlobs(cm, allGlobs)

	if len(candidates) == 0 {
		// If no globs specified or no matches, use all entries.
		// Iterate in sorted order for determinism.
		for _, path := range sortedKeys(cm.Entries) {
			candidates = append(candidates, cm.Entries[path])
		}
	}

	// Score each candidate.
	taskWords := tokenize(tf.Body)
	scored := make([]Candidate, 0, len(candidates))
	for _, e := range candidates {
		score := scoreEntry(e, tf, taskWords)
		scored = append(scored, Candidate{Entry: e, Score: score})
	}

	// Sort by score descending, then path ascending for stability.
	sort.Slice(scored, func(i, j int) bool {
		if scored[i].Score != scored[j].Score {
			return scored[i].Score > scored[j].Score
		}
		return scored[i].Entry.Path < scored[j].Entry.Path
	})

	// Cap to max_files.
	max := tf.MaxFiles
	if max <= 0 {
		max = 20
	}
	if len(scored) > max {
		scored = scored[:max]
	}

	// One-hop import expansion: add files imported by selected files
	// that aren't already in the result.
	scored = expandImports(scored, cm, max)

	return scored
}

// filterByGlobs returns entries whose paths match any of the given glob patterns.
// Iterates entries in sorted order for deterministic results.
func filterByGlobs(cm *model.CodeMap, globs []string) []*model.CodeMapEntry {
	if len(globs) == 0 {
		return nil
	}

	var result []*model.CodeMapEntry
	seen := make(map[string]bool)

	for _, path := range sortedKeys(cm.Entries) {
		e := cm.Entries[path]
		for _, g := range globs {
			matched, _ := doublestar.Match(g, e.Path)
			if matched && !seen[e.Path] {
				result = append(result, e)
				seen[e.Path] = true
				break
			}
		}
	}
	return result
}

// matchGlob matches a path against a glob pattern using doublestar for ** support.
func matchGlob(pattern, path string) bool {
	matched, _ := doublestar.Match(pattern, path)
	return matched
}

// Scoring weights — documented here for tuning.
const (
	weightContextGlob  = 3.0 // file is in a context_glob (direct work area)
	weightKnowledgeGlob = 1.0 // file is in a knowledge_glob (reference area)
	weightWordOverlap  = 0.5 // per task-word match in summary or when_to_use
	weightSymbolMatch  = 2.0 // task mentions an exported symbol name
	weightTestProximity = 0.5 // file has associated test files
	weightKeyword      = 1.0 // per keyword match
	weightImportHop    = 0.1 // added via one-hop import expansion
)

// scoreEntry computes a relevance score for an entry against the task.
func scoreEntry(e *model.CodeMapEntry, tf *taskfile.TaskFile, taskWords []string) float64 {
	var score float64

	// Path relevance: boost files in context_globs over knowledge_globs.
	for _, g := range tf.ContextGlobs {
		if matchGlob(g, e.Path) {
			score += weightContextGlob
			break
		}
	}
	for _, g := range tf.KnowledgeGlobs {
		if matchGlob(g, e.Path) {
			score += weightKnowledgeGlob
			break
		}
	}

	// Summary relevance: count task word matches in summary.
	summaryWords := tokenize(e.Summary)
	score += float64(countOverlap(taskWords, summaryWords)) * weightWordOverlap

	// WhenToUse relevance.
	whenWords := tokenize(e.WhenToUse)
	score += float64(countOverlap(taskWords, whenWords)) * weightWordOverlap

	// Public symbol relevance: boost if task mentions exported names.
	for _, sym := range e.PublicTypes {
		symLower := strings.ToLower(sym)
		for _, tw := range taskWords {
			if tw == symLower {
				score += weightSymbolMatch
			}
		}
	}
	for _, sym := range e.PublicFunctions {
		symLower := strings.ToLower(sym)
		for _, tw := range taskWords {
			if tw == symLower {
				score += weightSymbolMatch
			}
		}
	}

	// Test file proximity: boost non-test files that have associated tests.
	if len(e.TestFiles) > 0 {
		score += weightTestProximity
	}

	// Keyword relevance.
	for _, kw := range e.Keywords {
		kwLower := strings.ToLower(kw)
		for _, tw := range taskWords {
			if tw == kwLower {
				score += weightKeyword
			}
		}
	}

	return score
}

// expandImports adds one-hop imports of selected files that exist in the code map.
// Uses module-path-aware matching: an import like "github.com/codemap/internal/scan"
// matches files under "internal/scan/" only if the import path ends with a "/"
// boundary followed by the file's directory.
func expandImports(selected []Candidate, cm *model.CodeMap, max int) []Candidate {
	selectedPaths := make(map[string]bool)
	for _, c := range selected {
		selectedPaths[c.Entry.Path] = true
	}

	// Collect imported paths from selected entries.
	importedPkgs := make(map[string]bool)
	for _, c := range selected {
		for _, imp := range c.Entry.Imports {
			importedPkgs[imp] = true
		}
	}

	// Find code map entries that match imported packages.
	// Iterate in sorted order for deterministic expansion.
	for _, path := range sortedKeys(cm.Entries) {
		e := cm.Entries[path]
		if selectedPaths[e.Path] {
			continue
		}
		if len(selected) >= max {
			break
		}

		// Match: import path must end with "/"+dir or equal dir exactly.
		dir := filepath.Dir(e.Path)
		for pkg := range importedPkgs {
			if strings.HasSuffix(pkg, "/"+dir) || pkg == dir {
				selected = append(selected, Candidate{Entry: e, Score: weightImportHop})
				selectedPaths[e.Path] = true
				break
			}
		}
	}

	return selected
}

// sortedKeys returns the keys of a map in sorted order.
func sortedKeys(m map[string]*model.CodeMapEntry) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// tokenize splits text into lowercase word tokens.
func tokenize(s string) []string {
	s = strings.ToLower(s)
	words := strings.FieldsFunc(s, func(r rune) bool {
		return !((r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_')
	})
	return words
}

// countOverlap counts how many words in a appear in b.
func countOverlap(a, b []string) int {
	bSet := make(map[string]bool, len(b))
	for _, w := range b {
		bSet[w] = true
	}
	count := 0
	for _, w := range a {
		if bSet[w] {
			count++
		}
	}
	return count
}
