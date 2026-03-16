package selectpkg

import (
	"fmt"
	"io"
	"strings"

	"github.com/codemap/internal/taskfile"
)

// SelectedCodeMap bundles the task and selected candidates for rendering.
type SelectedCodeMap struct {
	Task       *taskfile.TaskFile
	Candidates []Candidate
}

// RenderContext writes a markdown document with the task description and
// selected file summaries, suitable for feeding to a coding agent.
func RenderContext(s *SelectedCodeMap, w io.Writer) error {
	fmt.Fprintln(w, "# Task Context")
	fmt.Fprintln(w)

	if s.Task.Body != "" {
		fmt.Fprintln(w, "## Task")
		fmt.Fprintln(w)
		fmt.Fprintln(w, s.Task.Body)
		fmt.Fprintln(w)
	}

	fmt.Fprintf(w, "## Selected Files (%d)\n", len(s.Candidates))
	fmt.Fprintln(w)

	for _, c := range s.Candidates {
		e := c.Entry
		fmt.Fprintf(w, "- %s (score: %.1f)\n", e.Path, c.Score)

		if e.Summary != "" {
			fmt.Fprintf(w, "  - summary: %s\n", e.Summary)
		}
		if e.WhenToUse != "" {
			fmt.Fprintf(w, "  - when to use: %s\n", e.WhenToUse)
		}
		if len(e.PublicTypes) > 0 {
			fmt.Fprintf(w, "  - public types: %s\n", strings.Join(e.PublicTypes, ", "))
		}
		if len(e.PublicFunctions) > 0 {
			fmt.Fprintf(w, "  - public functions: %s\n", strings.Join(e.PublicFunctions, ", "))
		}
		if len(e.Imports) > 0 {
			fmt.Fprintf(w, "  - imports: %s\n", strings.Join(e.Imports, ", "))
		}
		if len(e.Keywords) > 0 {
			fmt.Fprintf(w, "  - keywords: %s\n", strings.Join(e.Keywords, ", "))
		}
		if len(e.TestFiles) > 0 {
			fmt.Fprintf(w, "  - test files: %s\n", strings.Join(e.TestFiles, ", "))
		}
	}

	return nil
}
