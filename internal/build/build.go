// Package build orchestrates the incremental code map build pipeline:
// scan files, check mtime/blake3 for changes, parse, summarize, and persist.
package build

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jonnonz1/codemap/internal/hash"
	"github.com/jonnonz1/codemap/internal/llm"
	"github.com/jonnonz1/codemap/internal/model"
	"github.com/jonnonz1/codemap/internal/parse"
	"github.com/jonnonz1/codemap/internal/scan"
	"github.com/jonnonz1/codemap/internal/store"
)

// Result summarizes what happened during a build.
type Result struct {
	TotalFiles    int
	Unchanged     int
	Updated       int
	Added         int
	Removed       int
	ParseErrors   int
	SummaryErrors int
}

// Progress is called during a build to report current status.
type Progress struct {
	Phase   string // "scan", "summarize", "save"
	Current int
	Total   int
	Path    string
	Skipped bool
}

// ProgressFunc is an optional callback for build progress updates.
type ProgressFunc func(Progress)

// Options controls build behavior.
type Options struct {
	Workers   int // max concurrent LLM calls (default 10)
	RateLimit int // max requests per minute (default 50, 0 = no limit)
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() Options {
	return Options{Workers: 32, RateLimit: 50}
}

// Run performs an incremental code map build rooted at repoRoot.
func Run(repoRoot string, st store.Store, registry *parse.Registry, summarizer llm.Summarizer) (*Result, error) {
	return RunWithOptions(repoRoot, st, registry, summarizer, DefaultOptions(), nil)
}

// RunWithProgress performs a build with an optional progress callback.
func RunWithProgress(repoRoot string, st store.Store, registry *parse.Registry, summarizer llm.Summarizer, onProgress ProgressFunc) (*Result, error) {
	return RunWithOptions(repoRoot, st, registry, summarizer, DefaultOptions(), onProgress)
}

// RunWithOptions performs a build with full control over concurrency and progress.
func RunWithOptions(repoRoot string, st store.Store, registry *parse.Registry, summarizer llm.Summarizer, opts Options, onProgress ProgressFunc) (*Result, error) {
	files, err := scan.Dir(repoRoot)
	if err != nil {
		return nil, fmt.Errorf("scanning: %w", err)
	}

	existing, err := st.Load()
	if err != nil {
		return nil, fmt.Errorf("loading cache: %w", err)
	}

	testIndex := buildTestIndex(files)

	res := &Result{TotalFiles: len(files)}
	seen := make(map[string]bool)

	// Phase 1: scan, hash, parse — fast, no network.
	var needSummary []*model.CodeMapEntry
	var needSummaryData [][]byte

	for i, fi := range files {
		seen[fi.Path] = true
		prev, exists := existing.Entries[fi.Path]

		if exists && prev.ModTimeUnix == fi.ModTime {
			res.Unchanged++
			if onProgress != nil {
				onProgress(Progress{Phase: "scan", Current: i + 1, Total: len(files), Path: fi.Path, Skipped: true})
			}
			continue
		}

		data, err := os.ReadFile(filepath.Join(repoRoot, fi.Path))
		if err != nil {
			continue
		}

		h := hash.Blake3Hex(data)

		if exists && prev.Blake3 == h {
			updated := *prev
			updated.ModTimeUnix = fi.ModTime
			existing.Entries[fi.Path] = &updated
			res.Unchanged++
			if onProgress != nil {
				onProgress(Progress{Phase: "scan", Current: i + 1, Total: len(files), Path: fi.Path, Skipped: true})
			}
			continue
		}

		entry := &model.CodeMapEntry{
			Path:        fi.Path,
			Language:    fi.Language,
			ModTimeUnix: fi.ModTime,
			Blake3:      h,
		}

		ext := filepath.Ext(fi.Path)
		if p := registry.ForExtension(ext); p != nil {
			if err := p.Parse(data, entry); err != nil {
				res.ParseErrors++
			}
		}

		entry.TestFiles = testFilesForPath(fi.Path, testIndex)

		existing.Entries[fi.Path] = entry
		if exists {
			res.Updated++
		} else {
			res.Added++
		}

		if summarizer != nil {
			needSummary = append(needSummary, entry)
			needSummaryData = append(needSummaryData, data)
		}

		if onProgress != nil {
			onProgress(Progress{Phase: "scan", Current: i + 1, Total: len(files), Path: fi.Path})
		}
	}

	// Phase 2: summarize changed files in parallel with rate limiting.
	if len(needSummary) > 0 && summarizer != nil {
		var completed int64
		total := len(needSummary)
		var summaryErrors int64

		workers := opts.Workers
		if workers <= 0 {
			workers = 10
		}
		if total < workers {
			workers = total
		}

		// Rate limiter: token bucket refilled at opts.RateLimit per minute.
		rateLimit := opts.RateLimit
		if rateLimit <= 0 {
			rateLimit = 50
		}
		ticker := time.NewTicker(time.Minute / time.Duration(rateLimit))
		defer ticker.Stop()

		var wg sync.WaitGroup
		ch := make(chan int, total)
		for i := 0; i < total; i++ {
			ch <- i
		}
		close(ch)

		for w := 0; w < workers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for idx := range ch {
					<-ticker.C // wait for rate limit token

					entry := needSummary[idx]
					data := needSummaryData[idx]

					sr, err := summarizer.Summarize(entry.Path, data)
					if err == nil {
						entry.Summary = sr.Summary
						entry.WhenToUse = sr.WhenToUse
						entry.Keywords = sr.Keywords
					} else {
						atomic.AddInt64(&summaryErrors, 1)
					}

					done := int(atomic.AddInt64(&completed, 1))
					if onProgress != nil {
						onProgress(Progress{Phase: "summarize", Current: done, Total: total, Path: entry.Path})
					}
				}
			}()
		}
		wg.Wait()
		res.SummaryErrors = int(summaryErrors)
	}

	// Remove entries for deleted files.
	for path := range existing.Entries {
		if !seen[path] {
			delete(existing.Entries, path)
			res.Removed++
		}
	}

	// Phase 3: persist.
	if onProgress != nil {
		onProgress(Progress{Phase: "save"})
	}

	var changed []*model.CodeMapEntry
	for _, e := range needSummary {
		changed = append(changed, e)
	}

	if err := st.Save(existing); err != nil {
		return nil, fmt.Errorf("saving cache: %w", err)
	}
	if err := st.AppendChanged(changed); err != nil {
		return nil, fmt.Errorf("appending JSONL: %w", err)
	}

	return res, nil
}

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

func testFilesForPath(path string, idx map[string][]string) []string {
	if strings.HasSuffix(filepath.Base(path), "_test.go") {
		return nil
	}
	return idx[filepath.Dir(path)]
}
