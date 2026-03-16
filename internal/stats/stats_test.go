package stats

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLogAndLoad(t *testing.T) {
	dir := t.TempDir()

	e1 := &Event{
		Type:       EventBuild,
		Timestamp:  time.Now(),
		TotalFiles: 100,
		Unchanged:  90,
		Added:      10,
	}
	e2 := &Event{
		Type:          EventSelect,
		Timestamp:     time.Now(),
		TaskFile:      "task.md",
		SelectedFiles: []string{"a.go", "b.go"},
		SelectedCount: 2,
		TotalIndexed:  100,
	}

	if err := Log(dir, e1); err != nil {
		t.Fatalf("Log build: %v", err)
	}
	if err := Log(dir, e2); err != nil {
		t.Fatalf("Log select: %v", err)
	}

	events, err := LoadEvents(dir)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	if events[0].Type != EventBuild {
		t.Errorf("event[0].Type = %q, want build", events[0].Type)
	}
	if events[1].SelectedCount != 2 {
		t.Errorf("event[1].SelectedCount = %d, want 2", events[1].SelectedCount)
	}
}

func TestLoadEventsEmpty(t *testing.T) {
	dir := t.TempDir()
	events, err := LoadEvents(dir)
	if err != nil {
		t.Fatalf("LoadEvents: %v", err)
	}
	if len(events) != 0 {
		t.Errorf("expected 0 events, got %d", len(events))
	}
}

func TestCacheHitRate(t *testing.T) {
	e := &Event{TotalFiles: 100, Unchanged: 75}
	if got := e.CacheHitRate(); got != 0.75 {
		t.Errorf("CacheHitRate = %f, want 0.75", got)
	}

	empty := &Event{}
	if got := empty.CacheHitRate(); got != 0 {
		t.Errorf("CacheHitRate for empty = %f, want 0", got)
	}
}

func TestComputeBasic(t *testing.T) {
	now := time.Now()
	events := []Event{
		{Type: EventBuild, Timestamp: now, TotalFiles: 50, Unchanged: 40, Added: 10},
		{Type: EventBuild, Timestamp: now, TotalFiles: 50, Unchanged: 50},
		{Type: EventSelect, Timestamp: now, TaskFile: "task.md",
			SelectedFiles: []string{"a.go", "b.go", "c.go"},
			SelectedCount: 3, TotalIndexed: 50},
	}

	r := Compute(events, nil)

	if r.TotalBuilds != 2 {
		t.Errorf("TotalBuilds = %d, want 2", r.TotalBuilds)
	}
	// (0.8 + 1.0) / 2 = 0.9
	if r.AvgCacheHitRate < 0.89 || r.AvgCacheHitRate > 0.91 {
		t.Errorf("AvgCacheHitRate = %f, want ~0.9", r.AvgCacheHitRate)
	}
	if r.TotalSelections != 1 {
		t.Errorf("TotalSelections = %d, want 1", r.TotalSelections)
	}
	if r.AvgSelectedFiles != 3.0 {
		t.Errorf("AvgSelectedFiles = %f, want 3.0", r.AvgSelectedFiles)
	}
	// 1 - 3/50 = 0.94
	if r.AvgContextReduction < 0.93 || r.AvgContextReduction > 0.95 {
		t.Errorf("AvgContextReduction = %f, want ~0.94", r.AvgContextReduction)
	}
}

func TestComputeWithEval(t *testing.T) {
	now := time.Now()
	events := []Event{
		{Type: EventSelect, Timestamp: now, TaskFile: "task.md",
			SelectedFiles: []string{"a.go", "b.go", "c.go", "d.go"},
			SelectedCount: 4, TotalIndexed: 50},
	}
	// Actual changes: a.go, b.go, e.go
	// Precision: 2/4 = 50% (a.go and b.go were selected and changed)
	// Recall: 2/3 = 66% (a.go and b.go were caught, e.go was missed)
	gitChanges := map[string][]string{
		"task.md": {"a.go", "b.go", "e.go"},
	}

	r := Compute(events, gitChanges)

	if r.Evaluations != 1 {
		t.Fatalf("Evaluations = %d, want 1", r.Evaluations)
	}
	if r.AvgPrecision < 0.49 || r.AvgPrecision > 0.51 {
		t.Errorf("AvgPrecision = %f, want 0.5", r.AvgPrecision)
	}
	if r.AvgRecall < 0.66 || r.AvgRecall > 0.68 {
		t.Errorf("AvgRecall = %f, want ~0.67", r.AvgRecall)
	}

	d := r.EvalDetails[0]
	if len(d.Selected) != 4 {
		t.Errorf("Selected count = %d, want 4", len(d.Selected))
	}
	if len(d.ActualChanged) != 3 {
		t.Errorf("ActualChanged count = %d, want 3", len(d.ActualChanged))
	}
}

func TestPrintNoData(t *testing.T) {
	r := &Report{}
	var buf bytes.Buffer
	Print(r, &buf)
	if buf.Len() == 0 {
		t.Error("Print produced no output")
	}
}

func TestPrintWithEval(t *testing.T) {
	r := &Report{
		TotalBuilds:         3,
		TotalFilesIndexed:   50,
		AvgCacheHitRate:     0.85,
		TotalSelections:     2,
		AvgSelectedFiles:    6.5,
		AvgContextReduction: 0.87,
		Evaluations:         1,
		AvgPrecision:        0.5,
		AvgRecall:           0.67,
		AvgF1:               0.57,
		EvalDetails: []EvalDetail{
			{
				TaskFile:      "task.md",
				Timestamp:     time.Now(),
				Selected:      []string{"a.go", "b.go"},
				ActualChanged: []string{"a.go", "c.go"},
				Precision:     0.5,
				Recall:        0.5,
				F1:            0.5,
			},
		},
	}
	var buf bytes.Buffer
	Print(r, &buf)
	out := buf.String()

	if !strings.Contains(out, "85%") {
		t.Error("should show cache hit rate")
	}
	if !strings.Contains(out, "precision") {
		t.Error("should show precision")
	}
	if !strings.Contains(out, "recall") {
		t.Error("should show recall")
	}
}

func TestStatsFileLocation(t *testing.T) {
	dir := t.TempDir()
	Log(dir, &Event{Type: EventBuild, Timestamp: time.Now()})

	path := filepath.Join(dir, "stats.jsonl")
	if _, err := os.Stat(path); err != nil {
		t.Errorf("stats.jsonl not created at %s", path)
	}
}
