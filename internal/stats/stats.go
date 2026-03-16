// Package stats tracks codemap usage over time and computes impact metrics
// by comparing file selections against actual git changes.
package stats

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// EventType identifies what kind of codemap operation was logged.
type EventType string

const (
	EventBuild  EventType = "build"
	EventSelect EventType = "select"
)

// Event is a single logged codemap operation.
type Event struct {
	Type      EventType `json:"type"`
	Timestamp time.Time `json:"timestamp"`

	// Build fields.
	TotalFiles  int `json:"total_files,omitempty"`
	Added       int `json:"added,omitempty"`
	Updated     int `json:"updated,omitempty"`
	Unchanged   int `json:"unchanged,omitempty"`
	Removed     int `json:"removed,omitempty"`
	ParseErrors int `json:"parse_errors,omitempty"`

	// Select fields.
	TaskFile      string   `json:"task_file,omitempty"`
	TaskBody      string   `json:"task_body,omitempty"`
	SelectedFiles []string `json:"selected_files,omitempty"`
	SelectedCount int      `json:"selected_count,omitempty"`
	CandidatePool int      `json:"candidate_pool,omitempty"`
	TotalIndexed  int      `json:"total_indexed,omitempty"`
}

// CacheHitRate returns the fraction of files that were unchanged in a build event.
func (e *Event) CacheHitRate() float64 {
	if e.TotalFiles == 0 {
		return 0
	}
	return float64(e.Unchanged) / float64(e.TotalFiles)
}

// Log appends an event to the stats JSONL file.
func Log(cacheDir string, event *Event) error {
	path := filepath.Join(cacheDir, "stats.jsonl")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return err
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	return json.NewEncoder(f).Encode(event)
}

// LoadEvents reads all events from the stats JSONL file.
func LoadEvents(cacheDir string) ([]Event, error) {
	path := filepath.Join(cacheDir, "stats.jsonl")
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var events []Event
	dec := json.NewDecoder(f)
	for dec.More() {
		var e Event
		if err := dec.Decode(&e); err != nil {
			continue // skip malformed lines
		}
		events = append(events, e)
	}
	return events, nil
}

// Report holds computed statistics across all logged events.
type Report struct {
	// Build stats.
	TotalBuilds     int
	TotalFilesIndexed int
	AvgCacheHitRate float64
	LastBuild       time.Time

	// Select stats.
	TotalSelections     int
	AvgSelectedFiles    float64
	AvgContextReduction float64 // 1 - (selected/total), as percentage
	LastSelection       time.Time

	// Accuracy stats (computed against git).
	Evaluations    int
	AvgPrecision   float64 // of selected files, how many were actually changed
	AvgRecall      float64 // of changed files, how many were selected
	AvgF1          float64
	EvalDetails    []EvalDetail

	// Exploration stats (from tool usage tracking).
	TotalReads     int // total Read calls observed
	ExtraReads     int // Read calls for files NOT in codemap selection
	OverheadRatio  float64 // extra / total
}

// EvalDetail holds one selection-vs-git-diff comparison.
type EvalDetail struct {
	TaskFile      string
	Timestamp     time.Time
	Selected      []string
	ActualChanged []string
	Precision     float64
	Recall        float64
	F1            float64
}

// Compute builds a Report from logged events, git changes, and tool usage data.
func Compute(events []Event, gitChanges map[string][]string) *Report {
	return ComputeFull(events, gitChanges, nil)
}

// ComputeFull builds a Report with exploration tracking data.
func ComputeFull(events []Event, gitChanges map[string][]string, toolUses []ToolUseEvent) *Report {
	r := &Report{}

	var cacheHitSum float64
	var selectedSum float64
	var reductionSum float64

	for _, e := range events {
		switch e.Type {
		case EventBuild:
			r.TotalBuilds++
			r.TotalFilesIndexed = e.TotalFiles
			cacheHitSum += e.CacheHitRate()
			if e.Timestamp.After(r.LastBuild) {
				r.LastBuild = e.Timestamp
			}

		case EventSelect:
			r.TotalSelections++
			selectedSum += float64(e.SelectedCount)
			if e.TotalIndexed > 0 {
				reduction := 1.0 - float64(e.SelectedCount)/float64(e.TotalIndexed)
				reductionSum += reduction
			}
			if e.Timestamp.After(r.LastSelection) {
				r.LastSelection = e.Timestamp
			}

			// Evaluate accuracy if we have git changes for this selection.
			// Check for exact task file match or wildcard "*" (all tasks).
			changed, ok := gitChanges[e.TaskFile]
			if !ok {
				changed, ok = gitChanges["*"]
			}
			if ok && len(changed) > 0 {
				detail := evaluate(e, changed)
				r.EvalDetails = append(r.EvalDetails, detail)
				r.Evaluations++
			}
		}
	}

	if r.TotalBuilds > 0 {
		r.AvgCacheHitRate = cacheHitSum / float64(r.TotalBuilds)
	}
	if r.TotalSelections > 0 {
		r.AvgSelectedFiles = selectedSum / float64(r.TotalSelections)
		r.AvgContextReduction = reductionSum / float64(r.TotalSelections)
	}

	if r.Evaluations > 0 {
		var pSum, rSum, fSum float64
		for _, d := range r.EvalDetails {
			pSum += d.Precision
			rSum += d.Recall
			fSum += d.F1
		}
		r.AvgPrecision = pSum / float64(r.Evaluations)
		r.AvgRecall = rSum / float64(r.Evaluations)
		r.AvgF1 = fSum / float64(r.Evaluations)
	}

	// Exploration metrics from tool usage tracking.
	if len(toolUses) > 0 {
		selectEvents := make([]Event, 0)
		for _, e := range events {
			if e.Type == EventSelect {
				selectEvents = append(selectEvents, e)
			}
		}
		r.TotalReads, r.ExtraReads = ExplorationMetrics(selectEvents, toolUses)
		if r.TotalReads > 0 {
			r.OverheadRatio = float64(r.ExtraReads) / float64(r.TotalReads)
		}
	}

	return r
}

func evaluate(e Event, actualChanged []string) EvalDetail {
	selectedSet := make(map[string]bool)
	for _, f := range e.SelectedFiles {
		selectedSet[f] = true
	}
	changedSet := make(map[string]bool)
	for _, f := range actualChanged {
		changedSet[f] = true
	}

	// Precision: of selected files, how many were actually changed.
	truePositives := 0
	for _, f := range e.SelectedFiles {
		if changedSet[f] {
			truePositives++
		}
	}

	precision := 0.0
	if len(e.SelectedFiles) > 0 {
		precision = float64(truePositives) / float64(len(e.SelectedFiles))
	}

	// Recall: of changed files, how many were selected.
	recall := 0.0
	if len(actualChanged) > 0 {
		hits := 0
		for _, f := range actualChanged {
			if selectedSet[f] {
				hits++
			}
		}
		recall = float64(hits) / float64(len(actualChanged))
	}

	f1 := 0.0
	if precision+recall > 0 {
		f1 = 2 * precision * recall / (precision + recall)
	}

	return EvalDetail{
		TaskFile:      e.TaskFile,
		Timestamp:     e.Timestamp,
		Selected:      e.SelectedFiles,
		ActualChanged: actualChanged,
		Precision:     precision,
		Recall:        recall,
		F1:            f1,
	}
}

// Print writes the report to w in a human-readable format.
func Print(r *Report, w io.Writer) {
	fmt.Fprintln(w, "codemap statistics")
	fmt.Fprintln(w, "==================")
	fmt.Fprintln(w)

	// Build stats.
	fmt.Fprintln(w, "Build Performance")
	fmt.Fprintf(w, "  Total builds:        %d\n", r.TotalBuilds)
	fmt.Fprintf(w, "  Files indexed:       %d\n", r.TotalFilesIndexed)
	fmt.Fprintf(w, "  Avg cache hit rate:  %.0f%%\n", r.AvgCacheHitRate*100)
	if !r.LastBuild.IsZero() {
		fmt.Fprintf(w, "  Last build:          %s\n", r.LastBuild.Format("2006-01-02 15:04"))
	}
	fmt.Fprintln(w)

	// Select stats.
	fmt.Fprintln(w, "Context Selection")
	fmt.Fprintf(w, "  Total selections:    %d\n", r.TotalSelections)
	fmt.Fprintf(w, "  Avg files selected:  %.1f\n", r.AvgSelectedFiles)
	fmt.Fprintf(w, "  Avg context saved:   %.0f%%\n", r.AvgContextReduction*100)
	if !r.LastSelection.IsZero() {
		fmt.Fprintf(w, "  Last selection:      %s\n", r.LastSelection.Format("2006-01-02 15:04"))
	}
	fmt.Fprintln(w)

	// Accuracy stats.
	if r.Evaluations > 0 {
		fmt.Fprintln(w, "Selection Accuracy (vs actual git changes)")
		fmt.Fprintf(w, "  Evaluations:         %d\n", r.Evaluations)
		fmt.Fprintf(w, "  Avg precision:       %.0f%% (of selected files, how many were actually needed)\n", r.AvgPrecision*100)
		fmt.Fprintf(w, "  Avg recall:          %.0f%% (of changed files, how many were pre-selected)\n", r.AvgRecall*100)
		fmt.Fprintf(w, "  Avg F1 score:        %.0f%%\n", r.AvgF1*100)
		fmt.Fprintln(w)

		for _, d := range r.EvalDetails {
			fmt.Fprintf(w, "  [%s] %s\n", d.Timestamp.Format("Jan 02 15:04"), d.TaskFile)
			fmt.Fprintf(w, "    selected %d files, %d actually changed\n", len(d.Selected), len(d.ActualChanged))
			fmt.Fprintf(w, "    precision: %.0f%%  recall: %.0f%%  F1: %.0f%%\n", d.Precision*100, d.Recall*100, d.F1*100)
		}
	} else if r.TotalSelections > 0 {
		fmt.Fprintln(w, "Selection Accuracy")
		fmt.Fprintln(w, "  No evaluations yet. After completing a task, run:")
		fmt.Fprintln(w, "    codemap statistics --eval --task <task-file>")
		fmt.Fprintln(w, "  This compares selected files against recent git changes.")
	} else {
		fmt.Fprintln(w, "No selections recorded yet.")
		fmt.Fprintln(w, "  Claude will call codemap_select automatically via MCP.")
	}

	// Exploration overhead.
	if r.TotalReads > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Exploration Overhead")
		fmt.Fprintf(w, "  Total Read calls:    %d\n", r.TotalReads)
		fmt.Fprintf(w, "  Extra reads:         %d (files NOT in codemap selection)\n", r.ExtraReads)
		fmt.Fprintf(w, "  Overhead:            %.0f%%\n", r.OverheadRatio*100)
		if r.OverheadRatio < 0.2 {
			fmt.Fprintln(w, "  Verdict:             codemap is providing good coverage")
		} else if r.OverheadRatio < 0.5 {
			fmt.Fprintln(w, "  Verdict:             moderate — Claude needs some extra exploration")
		} else {
			fmt.Fprintln(w, "  Verdict:             high — codemap selection may need improvement")
		}
	}
}
