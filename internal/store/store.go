// Package store handles persistence of the code map cache as JSON and JSONL.
package store

import "github.com/jonnonz1/codemap/internal/model"

// Store defines the interface for reading and writing the code map cache.
type Store interface {
	// Load reads the full code map from the cache. Returns an empty CodeMap
	// if the cache does not exist yet.
	Load() (*model.CodeMap, error)

	// Save writes the full code map to the cache as JSON.
	Save(cm *model.CodeMap) error

	// AppendChanged writes changed entries to the JSONL log.
	AppendChanged(entries []*model.CodeMapEntry) error
}
