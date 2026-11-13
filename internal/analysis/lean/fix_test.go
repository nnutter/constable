package lean_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/analysistest"

	"github.com/nnutter/constable/internal/analysis/lean"
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
		Name: "capturelean",
		Doc:  "capture lean diagnostics",
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
			return lean.Analyzer.Run(pass)
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

func findDiagnostic(t *testing.T, diagnostics []capturedDiagnostic, substr string) capturedDiagnostic {
	t.Helper()
	for i := range diagnostics {
		if strings.Contains(diagnostics[i].message, substr) {
			return diagnostics[i]
		}
	}
	require.Fail(t, "missing diagnostic", substr)
	return capturedDiagnostic{}
}

func TestFixRemovesSingleDeclaration(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "only.go", `package p

// unusedHelper reports nothing.
func unusedHelper() int { return 1 }

func Keep() int { return 2 }
`)
	diagnostics := inspectFixes(t, dir)
	target := findDiagnostic(t, diagnostics, "unusedHelper")
	require.NotEmpty(t, target.edits, "safe diagnostic has no fix")
	applyCaptured(t, diagnostics)
	data, err := os.ReadFile(filepath.Join(dir, "src", "p", "only.go"))
	require.NoError(t, err)
	require.Contains(t, string(data), "func Keep()")
	require.NotContains(t, string(data), "unusedHelper")
	next := inspectFixes(t, dir)
	applyCaptured(t, next)
	second, err := os.ReadFile(filepath.Join(dir, "src", "p", "only.go"))
	require.NoError(t, err)
	require.Equal(t, string(data), string(second), "repeated analysis changed the fixed file")
	for _, d := range next {
		require.NotContains(t, d.message, "unusedHelper", "fixed diagnostic remains")
	}
}

func TestFixRemovesSingleGroupMember(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "group.go", `package p

var (
	unusedGroup = 1
	usedGroup   = 2
)

var _ = usedGroup
`)
	diagnostics := inspectFixes(t, dir)
	target := findDiagnostic(t, diagnostics, "unusedGroup")
	require.NotEmpty(t, target.edits, "group member has no fix")
	applyCaptured(t, diagnostics)
	data, err := os.ReadFile(filepath.Join(dir, "src", "p", "group.go"))
	require.NoError(t, err)
	require.Contains(t, string(data), "usedGroup")
	require.NotContains(t, string(data), "unusedGroup")
	next := inspectFixes(t, dir)
	applyCaptured(t, next)
	second, err := os.ReadFile(filepath.Join(dir, "src", "p", "group.go"))
	require.NoError(t, err)
	require.Equal(t, string(data), string(second), "repeated analysis changed the fixed file")
}

func TestFixSkipsGroupedConstant(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "consts.go", `package p

const (
	unusedConst = 1
	usedConst   = 2
)

const _ = usedConst
`)
	diagnostics := inspectFixes(t, dir)
	target := findDiagnostic(t, diagnostics, "unusedConst")
	require.Empty(t, target.edits, "grouped constant offered a fix")
}

func TestFixSkipsStandaloneComments(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "only.go", `package p

// This section comment has ambiguous ownership.

func unusedHelper() int { return 1 }

func Keep() int { return 2 }
`)
	diagnostics := inspectFixes(t, dir)
	target := findDiagnostic(t, diagnostics, "unusedHelper")
	require.Empty(t, target.edits, "offered a fix despite a standalone comment")
}

func TestFixRemovesPartialImport(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "imps.go", `package p

import (
	f "fmt"
	"strings"
)

func unused() {
	f.Println("x")
}

func Keep() string {
	return strings.ToUpper("x")
}
`)
	diagnostics := inspectFixes(t, dir)
	target := findDiagnostic(t, diagnostics, "unused unexported function unused")
	require.NotEmpty(t, target.edits, "safe diagnostic has no fix")
	applyCaptured(t, diagnostics)
	data, err := os.ReadFile(filepath.Join(dir, "src", "p", "imps.go"))
	require.NoError(t, err)
	require.NotContains(t, string(data), `"fmt"`)
	require.Contains(t, string(data), `"strings"`)
	require.Contains(t, string(data), "strings.ToUpper")
}

func TestFixRemovesGroupedTypeMember(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "types.go", `package p

type (
	unusedType struct{ x int }
	usedType   struct{ y int }
)

var _ = usedType{}
`)
	diagnostics := inspectFixes(t, dir)
	target := findDiagnostic(t, diagnostics, "unusedType")
	require.NotEmpty(t, target.edits, "group member has no fix")
	applyCaptured(t, diagnostics)
	data, err := os.ReadFile(filepath.Join(dir, "src", "p", "types.go"))
	require.NoError(t, err)
	require.Contains(t, string(data), "usedType")
	require.NotContains(t, string(data), "unusedType")
}

func TestFixRemovesDocumentedVariable(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "vars.go", `package p

// unusedCounter says nothing.
var unusedCounter = 1

var usedCounter = 2

var _ = usedCounter
`)
	diagnostics := inspectFixes(t, dir)
	target := findDiagnostic(t, diagnostics, "unusedCounter")
	require.NotEmpty(t, target.edits, "documented variable has no fix")
	applyCaptured(t, diagnostics)
	data, err := os.ReadFile(filepath.Join(dir, "src", "p", "vars.go"))
	require.NoError(t, err)
	require.NotContains(t, string(data), "unusedCounter")
	require.Contains(t, string(data), "usedCounter")
}

func TestFixSkipsGoDirective(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "only.go", "//go:generate echo hi\n\npackage p\n\nfunc unusedHelper() int { return 1 }\n\nfunc Keep() int { return 2 }\n")
	diagnostics := inspectFixes(t, dir)
	target := findDiagnostic(t, diagnostics, "unusedHelper")
	require.Empty(t, target.edits, "offered a fix despite a Go directive")
}

func TestFixRemovesOrphanedImport(t *testing.T) {
	dir := t.TempDir()
	writeFixture(t, dir, "imp.go", `package p

import "fmt"

func unused() {
	fmt.Println("x")
}

func Keep() int { return 2 }
`)
	diagnostics := inspectFixes(t, dir)
	target := findDiagnostic(t, diagnostics, "unused unexported function unused")
	require.NotEmpty(t, target.edits, "safe diagnostic has no fix")
	applyCaptured(t, diagnostics)
	data, err := os.ReadFile(filepath.Join(dir, "src", "p", "imp.go"))
	require.NoError(t, err)
	require.NotContains(t, string(data), "fmt")
	require.NotContains(t, string(data), "unused")
	require.Contains(t, string(data), "func Keep()")
}
