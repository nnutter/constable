// Package generated recognizes Go files that a tool writes and that a person
// must not edit.
package generated

import "go/ast"

// IsGenerated reports whether file carries the standard generated-code
// comment, "// Code generated <tool> DO NOT EDIT.".
//
// Analyzers skip generated files. A diagnostic there is not actionable
// because the next run of the generating tool overwrites the file. The check
// is the Go convention, so an author controls it with the comment alone.
func IsGenerated(file *ast.File) bool {
	return ast.IsGenerated(file)
}
