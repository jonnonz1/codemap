// Package model defines the core data types for codemap.
package model

// CodeMapEntry represents the indexed metadata for a single source file.
type CodeMapEntry struct {
	Path            string   `json:"path"`
	Language        string   `json:"language"`
	ModTimeUnix     int64    `json:"mod_time_unix"`
	Blake3          string   `json:"blake3"`
	Summary         string   `json:"summary,omitempty"`
	WhenToUse       string   `json:"when_to_use,omitempty"`
	PublicTypes     []string `json:"public_types,omitempty"`
	PublicFunctions []string `json:"public_functions,omitempty"`
	Imports         []string `json:"imports,omitempty"`
	Keywords        []string `json:"keywords,omitempty"`
	TestFiles       []string `json:"test_files,omitempty"`
}

// CodeMap holds the full set of indexed entries keyed by file path.
type CodeMap struct {
	Entries map[string]*CodeMapEntry `json:"entries"`
}

// NewCodeMap returns an empty CodeMap ready for use.
func NewCodeMap() *CodeMap {
	return &CodeMap{Entries: make(map[string]*CodeMapEntry)}
}
