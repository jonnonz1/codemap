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

func TestEstimateTokens(t *testing.T) {
	if got := EstimateTokens(0); got != 0 {
		t.Errorf("EstimateTokens(0) = %d, want 0", got)
	}
	if got := EstimateTokens(400); got != 100 {
		t.Errorf("EstimateTokens(400) = %d, want 100", got)
	}
	// 1 byte → ceil(1/4) = 1
	if got := EstimateTokens(1); got != 1 {
		t.Errorf("EstimateTokens(1) = %d, want 1", got)
	}
}

func TestComputeTokenSavings(t *testing.T) {
	now := time.Now()
	events := []Event{
		{Type: EventSelect, Timestamp: now, TaskFile: "task.md",
			SelectedFiles: []string{"a.go"}, SelectedCount: 1, TotalIndexed: 10,
			TotalTokens: 10000, SelectedTokens: 2000},
		{Type: EventSelect, Timestamp: now, TaskFile: "task2.md",
			SelectedFiles: []string{"b.go"}, SelectedCount: 1, TotalIndexed: 10,
			TotalTokens: 20000, SelectedTokens: 4000},
	}

	r := Compute(events, nil)

	if r.TotalTokensSaved != 24000 {
		t.Errorf("TotalTokensSaved = %d, want 24000", r.TotalTokensSaved)
	}
	if r.TotalTokensTotal != 30000 {
		t.Errorf("TotalTokensTotal = %d, want 30000", r.TotalTokensTotal)
	}
	// (0.8 + 0.8) / 2 = 0.8
	if r.AvgTokenReduction < 0.79 || r.AvgTokenReduction > 0.81 {
		t.Errorf("AvgTokenReduction = %f, want ~0.8", r.AvgTokenReduction)
	}
}

func TestPrintTokenSavings(t *testing.T) {
	r := &Report{
		TotalSelections:   2,
		TotalTokensSaved:  150000,
		TotalTokensTotal:  200000,
		AvgTokenReduction: 0.75,
	}
	var buf bytes.Buffer
	Print(r, &buf)
	out := buf.String()

	if !strings.Contains(out, "Context Window Savings") {
		t.Error("should show Context Window Savings section")
	}
	// Cumulative reduction: 150000/200000 = 75%
	if !strings.Contains(out, "75%") {
		t.Error("should show 75% reduction")
	}
	if !strings.Contains(out, "150.0K") {
		t.Error("should show 150.0K tokens saved")
	}
	if !strings.Contains(out, "Total repo context") {
		t.Error("should show 'Total repo context' label")
	}
	if !strings.Contains(out, "Selected context") {
		t.Error("should show 'Selected context' label")
	}
}

func TestPrintTokenSavingsUsesCumulativeReduction(t *testing.T) {
	// Verify the percentage is the cumulative ratio, not the simple average.
	// Event 1: 1000 total, 100 selected → 90% reduction
	// Event 2: 100000 total, 90000 selected → 10% reduction
	// Simple avg = 50%, cumulative = (900+10000)/101000 = ~10.8%
	r := &Report{
		TotalSelections:   2,
		TotalTokensSaved:  10900,   // 900 + 10000
		TotalTokensTotal:  101000,  // 1000 + 100000
		AvgTokenReduction: 0.50,    // simple avg (should NOT appear in output)
	}
	var buf bytes.Buffer
	Print(r, &buf)
	out := buf.String()

	// Should show ~11% (cumulative), NOT 50% (simple average).
	if strings.Contains(out, "50%") {
		t.Error("should use cumulative reduction (11%), not simple average (50%)")
	}
	if !strings.Contains(out, "11%") {
		t.Errorf("should show cumulative reduction ~11%%, got output:\n%s", out)
	}
}

func TestFormatTokens(t *testing.T) {
	tests := []struct {
		input int
		want  string
	}{
		{500, "500"},
		{1500, "1.5K"},
		{150000, "150.0K"},
		{1500000, "1.5M"},
	}
	for _, tt := range tests {
		got := formatTokens(tt.input)
		if got != tt.want {
			t.Errorf("formatTokens(%d) = %q, want %q", tt.input, got, tt.want)
		}
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
