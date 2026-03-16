// Package parse defines the interface for language-specific source file parsers.
package parse

import "github.com/jonnonz1/codemap/internal/model"

// Parser extracts deterministic facts from a source file and populates
// the corresponding fields on a CodeMapEntry. It must not call an LLM.
type Parser interface {
	// Language returns the language identifier this parser handles (e.g. "go").
	Language() string

	// Extensions returns the file extensions this parser handles (e.g. [".go"]).
	Extensions() []string

	// Parse reads source bytes and populates deterministic fields on entry.
	// The entry's Path and Language are already set before Parse is called.
	Parse(source []byte, entry *model.CodeMapEntry) error
}
