// Package render produces model-friendly markdown from the code map cache.
package render

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/jonnonz1/codemap/internal/model"
)

// Markdown writes the code map as a stable, sorted markdown document to w.
func Markdown(cm *model.CodeMap, w io.Writer) error {
	paths := make([]string, 0, len(cm.Entries))
	for p := range cm.Entries {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	fmt.Fprintln(w, "# Code Map")
	fmt.Fprintln(w)

	for _, path := range paths {
		e := cm.Entries[path]

		fmt.Fprintf(w, "- %s\n", e.Path)

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
