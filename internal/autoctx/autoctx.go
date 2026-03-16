// Package autoctx implements LLM-based auto-context selection.
// Given a code map and a task description, it asks a cheap model (Haiku/Flash)
// to select the files that should be in context for the task.
package autoctx

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/jonnonz1/codemap/internal/llm"
	"github.com/jonnonz1/codemap/internal/model"
	"github.com/jonnonz1/codemap/internal/taskfile"
)

// Result holds the output of an auto-context selection.
type Result struct {
	ContextFiles   []SelectedFile // source files selected for the task
	KnowledgeFiles []SelectedFile // reference/docs files selected
	FromCache      bool           // true if result was loaded from cache
}

// SelectedFile is a file chosen by the LLM with its full source.
type SelectedFile struct {
	Path   string
	Source string
}

// Options controls auto-context behavior.
type Options struct {
	RepoRoot string
	CacheDir string
	MaxFiles int // hard cap on selected files (default 15)
}

// Select uses an LLM to pick the most relevant files from the code map
// for the given task. Returns full source of selected files.
func Select(cm *model.CodeMap, tf *taskfile.TaskFile, summarizer llm.Summarizer, opts Options) (*Result, error) {
	// Check cache first.
	cacheKey := computeCacheKey(cm, tf)
	if cached, err := loadCache(opts.CacheDir, cacheKey); err == nil {
		return cached, nil
	}

	// Filter code map entries by globs.
	contextCandidates := filterByGlobs(cm, tf.ContextGlobs)
	knowledgeCandidates := filterByGlobs(cm, tf.KnowledgeGlobs)

	// If no globs specified, use all entries as context candidates.
	if len(tf.ContextGlobs) == 0 && len(tf.KnowledgeGlobs) == 0 {
		contextCandidates = allEntries(cm)
	}

	maxFiles := opts.MaxFiles
	if maxFiles <= 0 {
		maxFiles = tf.MaxFiles
	}
	if maxFiles <= 0 {
		maxFiles = 15
	}

	// Build the code map summary for the LLM.
	mapText := renderCodeMapForLLM(contextCandidates, knowledgeCandidates)

	// Ask LLM to select files.
	prompt := buildSelectionPrompt(mapText, tf.Body, maxFiles)
	sr, err := summarizer.Summarize("__auto_context__", []byte(prompt))
	if err != nil {
		return nil, fmt.Errorf("auto-context LLM call: %w", err)
	}

	// Parse the LLM response as a file list.
	selected := parseFileSelection(sr.Summary, cm, opts.RepoRoot)

	// Cap to max.
	if len(selected.ContextFiles) > maxFiles {
		selected.ContextFiles = selected.ContextFiles[:maxFiles]
	}

	// Cache the result.
	saveCache(opts.CacheDir, cacheKey, selected)

	return selected, nil
}

// SelectWithDedicatedCall uses a direct LLM API call instead of the Summarizer
// interface, which is better suited for the auto-context prompt.
func SelectWithDedicatedCall(cm *model.CodeMap, tf *taskfile.TaskFile, caller LLMCaller, opts Options) (*Result, error) {
	// Check cache first.
	cacheKey := computeCacheKey(cm, tf)
	if cached, err := loadCache(opts.CacheDir, cacheKey); err == nil {
		return cached, nil
	}

	contextCandidates := filterByGlobs(cm, tf.ContextGlobs)
	knowledgeCandidates := filterByGlobs(cm, tf.KnowledgeGlobs)

	if len(tf.ContextGlobs) == 0 && len(tf.KnowledgeGlobs) == 0 {
		contextCandidates = allEntries(cm)
	}

	maxFiles := opts.MaxFiles
	if maxFiles <= 0 {
		maxFiles = tf.MaxFiles
	}
	if maxFiles <= 0 {
		maxFiles = 15
	}

	mapText := renderCodeMapForLLM(contextCandidates, knowledgeCandidates)
	prompt := buildSelectionPrompt(mapText, tf.Body, maxFiles)

	response, err := caller.Call(prompt)
	if err != nil {
		return nil, fmt.Errorf("auto-context LLM call: %w", err)
	}

	selected := parseFileSelection(response, cm, opts.RepoRoot)

	if len(selected.ContextFiles) > maxFiles {
		selected.ContextFiles = selected.ContextFiles[:maxFiles]
	}

	saveCache(opts.CacheDir, cacheKey, selected)

	return selected, nil
}

// LLMCaller is a simple interface for making a single LLM call.
type LLMCaller interface {
	Call(prompt string) (string, error)
}

// renderCodeMapForLLM serializes code map entries as clean markdown
// that the LLM can read to make file selections.
func renderCodeMapForLLM(contextEntries, knowledgeEntries []*model.CodeMapEntry) string {
	var b strings.Builder

	if len(contextEntries) > 0 {
		b.WriteString("## Context Files (source code)\n\n")
		for _, e := range contextEntries {
			writeEntryForLLM(&b, e)
		}
	}

	if len(knowledgeEntries) > 0 {
		b.WriteString("\n## Knowledge Files (reference/docs)\n\n")
		for _, e := range knowledgeEntries {
			writeEntryForLLM(&b, e)
		}
	}

	return b.String()
}

func writeEntryForLLM(b *strings.Builder, e *model.CodeMapEntry) {
	fmt.Fprintf(b, "- %s\n", e.Path)
	if e.Summary != "" {
		fmt.Fprintf(b, "  summary: %s\n", e.Summary)
	}
	if e.WhenToUse != "" {
		fmt.Fprintf(b, "  when to use: %s\n", e.WhenToUse)
	}
	if len(e.PublicTypes) > 0 {
		fmt.Fprintf(b, "  types: %s\n", strings.Join(e.PublicTypes, ", "))
	}
	if len(e.PublicFunctions) > 0 {
		fmt.Fprintf(b, "  functions: %s\n", strings.Join(e.PublicFunctions, ", "))
	}
}

func buildSelectionPrompt(codeMap, taskBody string, maxFiles int) string {
	return fmt.Sprintf(`You are a code context selector. Given a code map of a repository and a task description, select the files that should be loaded into context for an AI coding agent to complete the task.

Rules:
- Select ONLY files the agent will need to read or modify
- Prefer fewer files with higher relevance over many files with low relevance
- Include files that define types/interfaces used by the files being modified
- Include relevant test files if the task involves testing
- Maximum %d files
- Return ONLY a JSON object with "context_files" (array of file paths) and "reasoning" (one sentence explaining your selection)

## Code Map

%s

## Task

%s

Return ONLY valid JSON:
{"context_files": ["path/to/file1", "path/to/file2"], "reasoning": "..."}`, maxFiles, codeMap, taskBody)
}

// parseFileSelection extracts file paths from the LLM response and reads
// their full source from disk.
func parseFileSelection(response string, cm *model.CodeMap, repoRoot string) *Result {
	// Try to parse as JSON.
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var parsed struct {
		ContextFiles   []string `json:"context_files"`
		KnowledgeFiles []string `json:"knowledge_files"`
		Reasoning      string   `json:"reasoning"`
	}

	result := &Result{}

	if err := json.Unmarshal([]byte(response), &parsed); err != nil {
		// Fallback: try to extract paths line by line.
		for _, line := range strings.Split(response, "\n") {
			line = strings.TrimSpace(line)
			line = strings.Trim(line, `",-`)
			line = strings.TrimSpace(line)
			if _, exists := cm.Entries[line]; exists {
				result.ContextFiles = append(result.ContextFiles, SelectedFile{
					Path:   line,
					Source: readSource(repoRoot, line),
				})
			}
		}
		return result
	}

	for _, path := range parsed.ContextFiles {
		if _, exists := cm.Entries[path]; exists {
			result.ContextFiles = append(result.ContextFiles, SelectedFile{
				Path:   path,
				Source: readSource(repoRoot, path),
			})
		}
	}
	for _, path := range parsed.KnowledgeFiles {
		if _, exists := cm.Entries[path]; exists {
			result.KnowledgeFiles = append(result.KnowledgeFiles, SelectedFile{
				Path:   path,
				Source: readSource(repoRoot, path),
			})
		}
	}

	return result
}

func readSource(repoRoot, path string) string {
	data, err := os.ReadFile(filepath.Join(repoRoot, path))
	if err != nil {
		return ""
	}
	return string(data)
}

// filterByGlobs returns entries whose paths match any of the given patterns.
func filterByGlobs(cm *model.CodeMap, globs []string) []*model.CodeMapEntry {
	if len(globs) == 0 {
		return nil
	}

	var result []*model.CodeMapEntry
	seen := make(map[string]bool)

	for _, path := range sortedKeys(cm) {
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

func allEntries(cm *model.CodeMap) []*model.CodeMapEntry {
	entries := make([]*model.CodeMapEntry, 0, len(cm.Entries))
	for _, path := range sortedKeys(cm) {
		entries = append(entries, cm.Entries[path])
	}
	return entries
}

func sortedKeys(cm *model.CodeMap) []string {
	keys := make([]string, 0, len(cm.Entries))
	for k := range cm.Entries {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// --- Caching ---

type cachedResult struct {
	ContextFiles   []cachedFile `json:"context_files"`
	KnowledgeFiles []cachedFile `json:"knowledge_files"`
}

type cachedFile struct {
	Path string `json:"path"`
}

func computeCacheKey(cm *model.CodeMap, tf *taskfile.TaskFile) string {
	h := sha256.New()
	// Hash the task body.
	h.Write([]byte(tf.Body))
	// Hash the globs.
	for _, g := range tf.ContextGlobs {
		h.Write([]byte(g))
	}
	for _, g := range tf.KnowledgeGlobs {
		h.Write([]byte(g))
	}
	// Hash entry count + a sample of paths to detect code map changes.
	fmt.Fprintf(h, "%d", len(cm.Entries))
	for _, path := range sortedKeys(cm) {
		e := cm.Entries[path]
		fmt.Fprintf(h, "%s:%s", path, e.Blake3)
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func loadCache(cacheDir, key string) (*Result, error) {
	path := filepath.Join(cacheDir, "select-"+key+".json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cached cachedResult
	if err := json.Unmarshal(data, &cached); err != nil {
		return nil, err
	}

	result := &Result{FromCache: true}
	for _, f := range cached.ContextFiles {
		result.ContextFiles = append(result.ContextFiles, SelectedFile{Path: f.Path})
	}
	for _, f := range cached.KnowledgeFiles {
		result.KnowledgeFiles = append(result.KnowledgeFiles, SelectedFile{Path: f.Path})
	}
	return result, nil
}

func saveCache(cacheDir, key string, r *Result) {
	cached := cachedResult{}
	for _, f := range r.ContextFiles {
		cached.ContextFiles = append(cached.ContextFiles, cachedFile{Path: f.Path})
	}
	for _, f := range r.KnowledgeFiles {
		cached.KnowledgeFiles = append(cached.KnowledgeFiles, cachedFile{Path: f.Path})
	}

	data, err := json.MarshalIndent(cached, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(cacheDir, 0o755)
	os.WriteFile(filepath.Join(cacheDir, "select-"+key+".json"), data, 0o644)
}
