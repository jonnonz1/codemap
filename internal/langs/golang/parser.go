// Package golang provides a Go source file parser that extracts deterministic
// facts (imports, exported types, exported functions) using the standard
// library's go/ast package.
package golang

import (
	"go/ast"
	"go/parser"
	"go/token"

	"github.com/codemap/internal/model"
)

// Parser extracts structural facts from Go source files.
type Parser struct{}

// Language returns "go".
func (p *Parser) Language() string { return "go" }

// Extensions returns Go file extensions.
func (p *Parser) Extensions() []string { return []string{".go"} }

// Parse populates entry with imports, exported types, and exported functions
// extracted from the Go source in data.
func (p *Parser) Parse(data []byte, entry *model.CodeMapEntry) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, entry.Path, data, parser.ParseComments)
	if err != nil {
		// Partial parse is OK — we still extract what we can.
		if f == nil {
			return err
		}
	}

	// Imports.
	for _, imp := range f.Imports {
		path := imp.Path.Value
		// Strip quotes.
		if len(path) >= 2 {
			path = path[1 : len(path)-1]
		}
		entry.Imports = append(entry.Imports, path)
	}

	// Exported types and functions.
	for _, decl := range f.Decls {
		switch d := decl.(type) {
		case *ast.GenDecl:
			if d.Tok == token.TYPE {
				for _, spec := range d.Specs {
					ts, ok := spec.(*ast.TypeSpec)
					if ok && ts.Name.IsExported() {
						entry.PublicTypes = append(entry.PublicTypes, ts.Name.Name)
					}
				}
			}
		case *ast.FuncDecl:
			if d.Name.IsExported() {
				entry.PublicFunctions = append(entry.PublicFunctions, d.Name.Name)
			}
		}
	}

	return nil
}
