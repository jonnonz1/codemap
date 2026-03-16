package taskfile

import "testing"

func TestParseBytes(t *testing.T) {
	input := []byte(`---
knowledge_globs:
  - docs/**
  - src/core/**
context_globs:
  - src/invoices/**
  - tests/invoices/**
max_files: 12
max_tokens: 50000
---

Add soft-delete support to invoices. Preserve existing patterns. Update tests.
`)

	tf, err := ParseBytes(input)
	if err != nil {
		t.Fatalf("ParseBytes() error: %v", err)
	}

	if len(tf.KnowledgeGlobs) != 2 {
		t.Errorf("knowledge_globs count = %d, want 2", len(tf.KnowledgeGlobs))
	}
	if tf.KnowledgeGlobs[0] != "docs/**" {
		t.Errorf("knowledge_globs[0] = %q, want %q", tf.KnowledgeGlobs[0], "docs/**")
	}

	if len(tf.ContextGlobs) != 2 {
		t.Errorf("context_globs count = %d, want 2", len(tf.ContextGlobs))
	}

	if tf.MaxFiles != 12 {
		t.Errorf("max_files = %d, want 12", tf.MaxFiles)
	}
	if tf.MaxTokens != 50000 {
		t.Errorf("max_tokens = %d, want 50000", tf.MaxTokens)
	}
	if tf.Body != "Add soft-delete support to invoices. Preserve existing patterns. Update tests." {
		t.Errorf("body = %q", tf.Body)
	}
}

func TestParseBytesNoFrontmatter(t *testing.T) {
	input := []byte(`Just a task description with no frontmatter.`)

	tf, err := ParseBytes(input)
	if err != nil {
		t.Fatalf("ParseBytes() error: %v", err)
	}

	if tf.Body != "Just a task description with no frontmatter." {
		t.Errorf("body = %q", tf.Body)
	}
	if tf.MaxFiles != 20 {
		t.Errorf("max_files = %d, want default 20", tf.MaxFiles)
	}
}

func TestParseBytesDefaults(t *testing.T) {
	input := []byte(`---
context_globs:
  - src/**
---

Do the thing.
`)

	tf, err := ParseBytes(input)
	if err != nil {
		t.Fatalf("ParseBytes() error: %v", err)
	}

	if tf.MaxFiles != 20 {
		t.Errorf("max_files = %d, want default 20", tf.MaxFiles)
	}
	if tf.MaxTokens != 50000 {
		t.Errorf("max_tokens = %d, want default 50000", tf.MaxTokens)
	}
}

func TestParseBytesCRLF(t *testing.T) {
	input := []byte("---\r\ncontext_globs:\r\n  - src/**\r\n---\r\n\r\nDo the thing.\r\n")

	tf, err := ParseBytes(input)
	if err != nil {
		t.Fatalf("ParseBytes() error: %v", err)
	}

	if len(tf.ContextGlobs) != 1 || tf.ContextGlobs[0] != "src/**" {
		t.Errorf("context_globs = %v, want [src/**]", tf.ContextGlobs)
	}
	if tf.Body != "Do the thing." {
		t.Errorf("body = %q", tf.Body)
	}
}

func TestParseBytesYAMLWithDashes(t *testing.T) {
	// A YAML value that contains "---" indented should NOT be treated as
	// the closing delimiter. Only "---" at column 0 closes the frontmatter.
	input := []byte(`---
context_globs:
  - src/**
  - "some---path/**"
---

Task body here.
`)

	tf, err := ParseBytes(input)
	if err != nil {
		t.Fatalf("ParseBytes() error: %v", err)
	}

	if len(tf.ContextGlobs) != 2 {
		t.Errorf("context_globs count = %d, want 2", len(tf.ContextGlobs))
	}
	if tf.Body != "Task body here." {
		t.Errorf("body = %q", tf.Body)
	}
}
