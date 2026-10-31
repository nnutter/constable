package methodical_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/nnutter/constable/internal/analysis/methodical"
)

type capturedDiagnostic struct {
	message string
	edits   []capturedEdit
}

type capturedEdit struct {
	path       string
	start, end int
	text       []byte
}

func inspectFixes(t *testing.T, dir string) []capturedDiagnostic {
	t.Helper()
	var diagnostics []capturedDiagnostic
	analyzer := &analysis.Analyzer{
		Name: "capturemethodical",
		Doc:  "capture methodical diagnostics",
		Run: func(pass *analysis.Pass) (any, error) {
			pass.Report = func(d analysis.Diagnostic) {
				item := capturedDiagnostic{message: d.Message}
				for _, fix := range d.SuggestedFixes {
					for _, edit := range fix.TextEdits {
						file := pass.Fset.File(edit.Pos)
						item.edits = append(item.edits, capturedEdit{path: file.Name(), start: file.Offset(edit.Pos), end: file.Offset(edit.End), text: edit.NewText})
					}
				}
				diagnostics = append(diagnostics, item)
			}
			return methodical.Analyzer.Run(pass)
		},
	}
	analysistest.Run(t, dir, analyzer, "p")
	return diagnostics
}

func writeFixture(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, "src", "p", name)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func applyCaptured(t *testing.T, diagnostics []capturedDiagnostic) {
	t.Helper()
	var edits []capturedEdit
	for _, diagnostic := range diagnostics {
		edits = append(edits, diagnostic.edits...)
	}
	slices.SortFunc(edits, func(a, b capturedEdit) int {
		if a.path != b.path {
			return strings.Compare(a.path, b.path)
		}
		return b.start - a.start
	})
	for _, edit := range edits {
		data, err := os.ReadFile(edit.path)
		require.NoError(t, err)
		data = append(append(append([]byte(nil), data[:edit.start]...), edit.text...), data[edit.end:]...)
		require.NoError(t, os.WriteFile(edit.path, data, 0o644))
	}
}

func TestFixTargetsDiagnosticAndIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "types.go", `package p

type Unsafe struct{}

type Safe struct{}

func (Safe) Z() {}

func (Safe) A() {}

type Unrelated struct{}

// Keep this method in place.
func (Unrelated) AlreadyCorrect() {}
`)
	writeFixture(t, dir, "other.go", `package p

import "fmt"

func (Unsafe) Show() { fmt.Print("x") }
`)
	diagnostics := inspectFixes(t, dir)
	var unsafeFound, safeFound bool
	for _, d := range diagnostics {
		if strings.Contains(d.message, "type Unsafe") {
			unsafeFound = true
			require.Empty(t, d.edits, "unsafe diagnostic offered a fix")
		}
		if strings.Contains(d.message, "type Safe") {
			safeFound = true
			require.NotEmpty(t, d.edits, "safe diagnostic has no fix")
		}
	}
	require.True(t, unsafeFound, "missing unsafe diagnostic: %+v", diagnostics)
	require.True(t, safeFound, "missing safe diagnostic: %+v", diagnostics)
	applyCaptured(t, diagnostics)
	path := filepath.Join(dir, "src", "p", "types.go")
	first, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(first), "type Unrelated struct{}\n\n// Keep this method in place.\nfunc (Unrelated) AlreadyCorrect() {}")
	next := inspectFixes(t, dir)
	applyCaptured(t, next)
	second, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, first, second, "repeated analysis changed the fixed file")
	for _, d := range next {
		require.NotContains(t, d.message, "type Safe", "fixed diagnostic remains")
	}
}

func TestFixSkipsStandaloneComments(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "types.go", `package p

type T struct{}

func (T) Z() {}

// This section comment has ambiguous ownership.

func (T) A() {}
`)
	diagnostics := inspectFixes(t, dir)
	require.NotEmpty(t, diagnostics, "expected an ordering diagnostic")
	for _, d := range diagnostics {
		require.Empty(t, d.edits, "offered a fix despite a standalone comment")
	}
}
