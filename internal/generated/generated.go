// Package generated recognizes Go files that a tool writes and that a person
// must not edit.
package generated

import "go/ast"

// Skipped reports whether analyzers omit the diagnostics of file.
//
// A diagnostic in a generated file is not actionable because the next run of
// the generating tool overwrites the file. The mark is the Go convention,
// "// Code generated <tool> DO NOT EDIT.", so an author controls it with the
// comment alone.
func Skipped(file *ast.File) bool {
	return ast.IsGenerated(file)
}
