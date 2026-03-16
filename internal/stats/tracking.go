package stats

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// ToolUseEvent records a single tool call made by Claude during a session.
type ToolUseEvent struct {
	Timestamp time.Time `json:"timestamp"`
	Tool      string    `json:"tool"`      // Read, Glob, Grep, etc.
	Path      string    `json:"path"`      // file path if applicable
}

// LogToolUse appends a tool use event to the tracking log.
func LogToolUse(cacheDir string, event *ToolUseEvent) error {
	path := filepath.Join(cacheDir, "tool-usage.jsonl")
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

// LoadToolUses reads all tool use events from the tracking log.
func LoadToolUses(cacheDir string) ([]ToolUseEvent, error) {
	path := filepath.Join(cacheDir, "tool-usage.jsonl")
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var events []ToolUseEvent
	dec := json.NewDecoder(f)
	for dec.More() {
		var e ToolUseEvent
		if err := dec.Decode(&e); err != nil {
			continue
		}
		events = append(events, e)
	}
	return events, nil
}

// ExplorationMetrics computes how much Claude explored beyond codemap selections.
func ExplorationMetrics(selections []Event, toolUses []ToolUseEvent) (totalReads, extraReads int) {
	// Build set of all selected file paths.
	selectedPaths := make(map[string]bool)
	for _, sel := range selections {
		for _, p := range sel.SelectedFiles {
			selectedPaths[p] = true
		}
	}

	for _, tu := range toolUses {
		if tu.Tool == "Read" && tu.Path != "" {
			totalReads++
			if !selectedPaths[tu.Path] {
				extraReads++
			}
		}
	}
	return
}
