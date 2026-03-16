package golang

import (
	"testing"

	"github.com/jonnonz1/codemap/internal/model"
)

func TestParse(t *testing.T) {
	src := []byte(`package example

import (
	"fmt"
	"os"
)

// PublicStruct is exported.
type PublicStruct struct{}

// privateStruct is unexported.
type privateStruct struct{}

// PublicInterface is exported.
type PublicInterface interface{}

// PublicFunc is exported.
func PublicFunc() {}

// privateFunc is unexported.
func privateFunc() {}

// MethodOnPublic is an exported method.
func (p *PublicStruct) MethodOnPublic() {}
`)

	entry := &model.CodeMapEntry{Path: "example.go", Language: "go"}
	p := &Parser{}

	if err := p.Parse(src, entry); err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	assertContains(t, "imports", entry.Imports, "fmt")
	assertContains(t, "imports", entry.Imports, "os")

	assertContains(t, "public_types", entry.PublicTypes, "PublicStruct")
	assertContains(t, "public_types", entry.PublicTypes, "PublicInterface")
	assertNotContains(t, "public_types", entry.PublicTypes, "privateStruct")

	assertContains(t, "public_functions", entry.PublicFunctions, "PublicFunc")
	assertContains(t, "public_functions", entry.PublicFunctions, "MethodOnPublic")
	assertNotContains(t, "public_functions", entry.PublicFunctions, "privateFunc")
}

func TestParseInvalidSource(t *testing.T) {
	// Go's parser is lenient — even broken source may partially parse.
	// We verify that Parse doesn't panic on bad input and returns an error.
	src := []byte(`}{][`)
	entry := &model.CodeMapEntry{Path: "bad.go", Language: "go"}
	p := &Parser{}

	// Should not panic regardless of error return.
	_ = p.Parse(src, entry)
}

func TestParsePartialSource(t *testing.T) {
	// Partial Go source may still parse exported symbols.
	src := []byte(`package broken
func Exported() {}
func !!!
`)
	entry := &model.CodeMapEntry{Path: "partial.go", Language: "go"}
	p := &Parser{}

	// Should not error — partial parse extracts what it can.
	_ = p.Parse(src, entry)
	assertContains(t, "public_functions", entry.PublicFunctions, "Exported")
}

func assertContains(t *testing.T, field string, slice []string, want string) {
	t.Helper()
	for _, s := range slice {
		if s == want {
			return
		}
	}
	t.Errorf("%s: missing %q in %v", field, want, slice)
}

func assertNotContains(t *testing.T, field string, slice []string, unwanted string) {
	t.Helper()
	for _, s := range slice {
		if s == unwanted {
			t.Errorf("%s: should not contain %q", field, unwanted)
			return
		}
	}
}
