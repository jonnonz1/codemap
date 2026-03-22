// Package build orchestrates the incremental code map build pipeline:
// scan files, check mtime/blake3 for changes, parse, summarize, and persist.
package build

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jonnonz1/codemap/internal/config"
	"github.com/jonnonz1/codemap/internal/hash"
	"github.com/jonnonz1/codemap/internal/llm"
	"github.com/jonnonz1/codemap/internal/model"
	"github.com/jonnonz1/codemap/internal/parse"
	"github.com/jonnonz1/codemap/internal/scan"
	"github.com/jonnonz1/codemap/internal/store"
)

// Result summarizes what happened during a build.
type Result struct {
	TotalFiles     int
	Unchanged      int
	Updated        int
	Added          int
	Removed        int
	ParseErrors    int
	SummaryErrors  int
	SkippedTrivial int
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
	Workers   int                // max concurrent LLM calls (default 10)
	RateLimit int                // max requests per minute (default 50, 0 = no limit)
	Scan      *config.ScanConfig // file scanning filter config (nil = defaults)
	BatchSize int                // files per LLM batch call (default 5, 1 = no batching)
}

// DefaultOptions returns sensible defaults.
func DefaultOptions() Options {
	return Options{Workers: 32, RateLimit: 50, BatchSize: 5}
}

// Run performs an incremental code map build rooted at repoRoot.
func Run(repoRoot string, st store.Store, registry *parse.Registry, summarizer llm.Summarizer) (*Result, error) {
	return RunWithOptions(repoRoot, st, registry, summarizer, DefaultOptions(), nil)
}

// RunWithProgress performs a build with an optional progress callback.
func RunWithProgress(repoRoot string, st store.Store, registry *parse.Registry, summarizer llm.Summarizer, onProgress ProgressFunc) (*Result, error) {
	return RunWithOptions(repoRoot, st, registry, summarizer, DefaultOptions(), onProgress)
}

// processedFile holds the result of a single file's read/hash/parse.
type processedFile struct {
	index int
	entry *model.CodeMapEntry
	data  []byte
	isNew bool
}

// RunWithOptions performs a build with full control over concurrency and progress.
func RunWithOptions(repoRoot string, st store.Store, registry *parse.Registry, summarizer llm.Summarizer, opts Options, onProgress ProgressFunc) (*Result, error) {
	files, err := scan.Dir(repoRoot, opts.Scan)
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

	// Phase 1: parallel file read, hash, parse.
	// First pass: identify which files need processing vs cached.
	type fileJob struct {
		index int
		fi    scan.FileInfo
		prev  *model.CodeMapEntry
	}

	var jobs []fileJob
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

		jobs = append(jobs, fileJob{index: i, fi: fi, prev: prev})
	}

	// Process changed files concurrently.
	cpuWorkers := runtime.NumCPU()
	if len(jobs) < cpuWorkers {
		cpuWorkers = len(jobs)
	}
	if cpuWorkers == 0 {
		cpuWorkers = 1
	}

	var needSummary []*model.CodeMapEntry
	var needSummaryData [][]byte
	var parseErrors int64

	if len(jobs) > 0 {
		results := make([]processedFile, len(jobs))
		var wg sync.WaitGroup
		jobCh := make(chan int, len(jobs))
		for i := range jobs {
			jobCh <- i
		}
		close(jobCh)

		for w := 0; w < cpuWorkers; w++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for ji := range jobCh {
					j := jobs[ji]
					data, err := os.ReadFile(filepath.Join(repoRoot, j.fi.Path))
					if err != nil {
						results[ji] = processedFile{index: j.index}
						continue
					}

					h := hash.Blake3Hex(data)

					if j.prev != nil && j.prev.Blake3 == h {
						updated := *j.prev
						updated.ModTimeUnix = j.fi.ModTime
						results[ji] = processedFile{
							index: j.index,
							entry: &updated,
						}
						continue
					}

					entry := &model.CodeMapEntry{
						Path:        j.fi.Path,
						Language:    j.fi.Language,
						ModTimeUnix: j.fi.ModTime,
						Blake3:      h,
					}

					ext := filepath.Ext(j.fi.Path)
					if p := registry.ForExtension(ext); p != nil {
						if err := p.Parse(data, entry); err != nil {
							atomic.AddInt64(&parseErrors, 1)
						}
					}

					entry.TestFiles = testFilesForPath(j.fi.Path, testIndex)

					results[ji] = processedFile{
						index: j.index,
						entry: entry,
						data:  data,
						isNew: j.prev == nil,
					}
				}
			}()
		}
		wg.Wait()

		res.ParseErrors = int(parseErrors)

		// Collect results sequentially to maintain deterministic ordering.
		for _, r := range results {
			if r.entry == nil {
				continue
			}

			fi := files[r.index]
			existing.Entries[fi.Path] = r.entry

			if r.data == nil {
				// Hash-matched: content unchanged, only mtime updated.
				res.Unchanged++
				if onProgress != nil {
					onProgress(Progress{Phase: "scan", Current: r.index + 1, Total: len(files), Path: fi.Path, Skipped: true})
				}
				continue
			}

			if r.isNew {
				res.Added++
			} else {
				res.Updated++
			}

			if summarizer != nil && !isTrivial(r.entry, r.data) {
				needSummary = append(needSummary, r.entry)
				needSummaryData = append(needSummaryData, r.data)
			} else if summarizer != nil {
				res.SkippedTrivial++
			}

			if onProgress != nil {
				onProgress(Progress{Phase: "scan", Current: r.index + 1, Total: len(files), Path: fi.Path})
			}
		}
	}

	// Phase 2: summarize changed files with batching + rate limiting.
	if len(needSummary) > 0 && summarizer != nil {
		batchSize := opts.BatchSize
		if batchSize <= 0 {
			batchSize = 5
		}

		batcher, isBatchable := summarizer.(llm.BatchSummarizer)

		if isBatchable && batchSize > 1 {
			summarizeBatched(batcher, needSummary, needSummaryData, batchSize, opts, res, onProgress)
		} else {
			summarizeIndividual(summarizer, needSummary, needSummaryData, opts, res, onProgress)
		}
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

const trivialMaxBytes = 500

// isTrivial returns true if a file is too simple to warrant an LLM call.
// Trivial = small file with no public API surface.
func isTrivial(entry *model.CodeMapEntry, data []byte) bool {
	if len(data) > trivialMaxBytes {
		return false
	}
	if len(entry.PublicFunctions) > 0 || len(entry.PublicTypes) > 0 {
		return false
	}
	return true
}

// summarizeIndividual calls the LLM once per file (original behavior).
func summarizeIndividual(summarizer llm.Summarizer, entries []*model.CodeMapEntry, data [][]byte, opts Options, res *Result, onProgress ProgressFunc) {
	var completed int64
	total := len(entries)
	var summaryErrors int64

	workers := opts.Workers
	if workers <= 0 {
		workers = 10
	}
	if total < workers {
		workers = total
	}

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
				<-ticker.C

				entry := entries[idx]
				sr, err := summarizer.Summarize(entry.Path, data[idx])
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

// summarizeBatched groups files into batches and calls the LLM once per batch.
func summarizeBatched(batcher llm.BatchSummarizer, entries []*model.CodeMapEntry, data [][]byte, batchSize int, opts Options, res *Result, onProgress ProgressFunc) {
	type batch struct {
		entries []*model.CodeMapEntry
		data    [][]byte
		paths   []string
	}

	var batches []batch
	for i := 0; i < len(entries); i += batchSize {
		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}
		b := batch{
			entries: entries[i:end],
			data:    data[i:end],
		}
		for _, e := range b.entries {
			b.paths = append(b.paths, e.Path)
		}
		batches = append(batches, b)
	}

	total := len(entries)
	var completed int64
	var summaryErrors int64

	workers := opts.Workers
	if workers <= 0 {
		workers = 10
	}
	if len(batches) < workers {
		workers = len(batches)
	}

	rateLimit := opts.RateLimit
	if rateLimit <= 0 {
		rateLimit = 50
	}
	ticker := time.NewTicker(time.Minute / time.Duration(rateLimit))
	defer ticker.Stop()

	var wg sync.WaitGroup
	ch := make(chan int, len(batches))
	for i := range batches {
		ch <- i
	}
	close(ch)

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for bi := range ch {
				<-ticker.C

				b := batches[bi]
				results, err := batcher.SummarizeBatch(b.paths, b.data)
				if err != nil {
					atomic.AddInt64(&summaryErrors, int64(len(b.entries)))
					done := int(atomic.AddInt64(&completed, int64(len(b.entries))))
					if onProgress != nil {
						onProgress(Progress{Phase: "summarize", Current: done, Total: total, Path: b.paths[len(b.paths)-1]})
					}
					continue
				}

				for i, entry := range b.entries {
					if i < len(results) && results[i] != nil {
						entry.Summary = results[i].Summary
						entry.WhenToUse = results[i].WhenToUse
						entry.Keywords = results[i].Keywords
					} else {
						atomic.AddInt64(&summaryErrors, 1)
					}

					done := int(atomic.AddInt64(&completed, 1))
					if onProgress != nil {
						onProgress(Progress{Phase: "summarize", Current: done, Total: total, Path: entry.Path})
					}
				}
			}
		}()
	}
	wg.Wait()
	res.SummaryErrors = int(summaryErrors)
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
