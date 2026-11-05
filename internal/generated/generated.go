// Package generated recognizes Go files that a tool writes and that a person
// must not edit.
package generated

import "go/ast"

// include reports whether analyzers report in generated files.
var include bool

// Include makes analyzers report in generated files when enabled is true.
//
// The driver sets it from -include-generated, so every analyzer follows one
// answer without carrying a flag of its own.
func Include(enabled bool) {
	include = enabled
}

// Skipped reports whether analyzers omit the diagnostics of file.
//
// A diagnostic in a generated file is not actionable because the next run of
// the generating tool overwrites the file. The mark is the Go convention,
// "// Code generated <tool> DO NOT EDIT.", so an author controls it with the
// comment alone. Include overrides the rule for a run that wants every
// diagnostic.
func Skipped(file *ast.File) bool {
	return !include && ast.IsGenerated(file)
}
