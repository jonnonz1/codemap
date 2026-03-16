// Package taskfile parses markdown task files with YAML frontmatter
// used by `codemap select --task`.
package taskfile

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// TaskFile represents a parsed task file with YAML frontmatter and body text.
type TaskFile struct {
	KnowledgeGlobs []string `yaml:"knowledge_globs"`
	ContextGlobs   []string `yaml:"context_globs"`
	MaxFiles       int      `yaml:"max_files"`
	MaxTokens      int      `yaml:"max_tokens"`
	Body           string   `yaml:"-"`
}

// Parse reads and parses a task file from the given path.
func Parse(path string) (*TaskFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseBytes(data)
}

// ParseBytes parses a task file from raw bytes.
func ParseBytes(data []byte) (*TaskFile, error) {
	// Normalize CRLF to LF for consistent parsing.
	data = bytes.ReplaceAll(data, []byte("\r\n"), []byte("\n"))

	frontmatter, body := splitFrontmatter(data)

	tf := &TaskFile{
		MaxFiles:  20,    // sensible default
		MaxTokens: 50000, // sensible default
	}

	if len(frontmatter) > 0 {
		if err := yaml.Unmarshal(frontmatter, tf); err != nil {
			return nil, fmt.Errorf("parsing frontmatter: %w", err)
		}
	}

	tf.Body = string(bytes.TrimSpace(body))
	return tf, nil
}

// splitFrontmatter splits a document into YAML frontmatter and body.
// The closing delimiter must be "---" alone on its own line (no leading
// whitespace). This avoids false matches on "---" inside YAML values.
func splitFrontmatter(data []byte) ([]byte, []byte) {
	const delim = "---\n"

	trimmed := bytes.TrimLeft(data, " \t\n")
	if !bytes.HasPrefix(trimmed, []byte(delim)) {
		return nil, data
	}

	rest := trimmed[len(delim):]

	// Scan line by line to find the closing delimiter. Only a line that is
	// exactly "---" (optionally followed by whitespace then newline) counts.
	// This prevents matching "---" embedded inside YAML string values.
	for i := 0; i < len(rest); {
		// Find end of current line.
		nl := bytes.IndexByte(rest[i:], '\n')
		var line []byte
		if nl < 0 {
			line = rest[i:]
		} else {
			line = rest[i : i+nl]
		}

		if bytes.Equal(bytes.TrimRight(line, " \t"), []byte("---")) {
			fm := rest[:i]
			if nl < 0 {
				return fm, nil
			}
			return fm, rest[i+nl+1:]
		}

		if nl < 0 {
			break
		}
		i += nl + 1
	}

	return nil, data
}
