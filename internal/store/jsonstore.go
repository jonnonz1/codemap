package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/codemap/internal/model"
)

// JSONStore persists the code map as a JSON file and appends changes to a JSONL file.
type JSONStore struct {
	jsonPath  string
	jsonlPath string
}

// NewJSONStore creates a store backed by the given JSON and JSONL file paths.
func NewJSONStore(jsonPath, jsonlPath string) *JSONStore {
	return &JSONStore{jsonPath: jsonPath, jsonlPath: jsonlPath}
}

// Load reads the code map from the JSON cache file.
// Returns an empty CodeMap if the file does not exist.
func (s *JSONStore) Load() (*model.CodeMap, error) {
	data, err := os.ReadFile(s.jsonPath)
	if os.IsNotExist(err) {
		return model.NewCodeMap(), nil
	}
	if err != nil {
		return nil, err
	}

	var cm model.CodeMap
	if err := json.Unmarshal(data, &cm); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", s.jsonPath, err)
	}
	if cm.Entries == nil {
		cm.Entries = make(map[string]*model.CodeMapEntry)
	}
	return &cm, nil
}

// Save writes the full code map to the JSON cache file atomically by writing
// to a temporary file first, then renaming. This prevents corruption if the
// process crashes mid-write.
func (s *JSONStore) Save(cm *model.CodeMap) error {
	dir := filepath.Dir(s.jsonPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	data, err := json.MarshalIndent(cm, "", "  ")
	if err != nil {
		return err
	}

	// Write to temp file in same directory (same filesystem for atomic rename).
	tmp, err := os.CreateTemp(dir, ".codemap-*.json.tmp")
	if err != nil {
		return fmt.Errorf("creating temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("writing temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("closing temp file: %w", err)
	}

	// Atomic rename replaces the old file.
	if err := os.Rename(tmpPath, s.jsonPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("renaming temp file: %w", err)
	}

	return nil
}

// AppendChanged writes changed entries to the JSONL log file, one entry per line.
func (s *JSONStore) AppendChanged(entries []*model.CodeMapEntry) error {
	if len(entries) == 0 {
		return nil
	}

	if err := os.MkdirAll(filepath.Dir(s.jsonlPath), 0o755); err != nil {
		return err
	}

	f, err := os.OpenFile(s.jsonlPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return nil
}
