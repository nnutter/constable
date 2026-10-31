package driver_test

import (
	"fmt"
	"go/ast"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"

	"github.com/nnutter/constable/internal/driver"
)

func expectsDiff(diff, stale, outputFailure bool) bool {
	if !diff {
		return false
	}
	if stale {
		return false
	}
	return !outputFailure
}

func fixAnalyzer(path string, fail, conflict, stale bool) *analysis.Analyzer {
	return &analysis.Analyzer{Name: "fixer", Doc: "test fixes", Run: func(pass *analysis.Pass) (any, error) {
		if fail {
			return nil, fmt.Errorf("analysis failed")
		}
		for _, file := range pass.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				id, ok := n.(*ast.Ident)
				if !ok {
					return true
				}
				if id.Name != "Broken" {
					return true
				}
				pass.Report(analysis.Diagnostic{Pos: id.Pos(), Message: "fixable", SuggestedFixes: []analysis.SuggestedFix{{Message: "rename", TextEdits: []analysis.TextEdit{{Pos: id.Pos(), End: id.End(), NewText: []byte("Fixed")}}}}})
				pass.Report(analysis.Diagnostic{Pos: id.Pos(), Message: "unsupported"})
				if conflict {
					pass.Report(analysis.Diagnostic{Pos: id.Pos(), Message: "conflict", SuggestedFixes: []analysis.SuggestedFix{{Message: "other", TextEdits: []analysis.TextEdit{{Pos: id.Pos(), End: id.End(), NewText: []byte("Other")}}}}})
				}
				return true
			})
		}
		if stale {
			if err := os.WriteFile(path, []byte("package fix\nvar Broken = 2\n"), 0o644); err != nil {
				return nil, err
			}
		}
		return nil, nil
	}}
}

func TestFixes(t *testing.T) {
	for _, tc := range []struct {
		name                                              string
		diff, conflict, fail, tests, stale, outputFailure bool
		wantCode                                          int
		wantText                                          string
	}{
		{name: "apply", tests: true, wantCode: 0, wantText: "var Fixed = 1"},
		{name: "diff", diff: true, tests: true, wantCode: 0, wantText: "var Broken = 1"},
		{name: "conflict", conflict: true, wantCode: 1, wantText: "var Broken = 1"},
		{name: "analysis error", fail: true, wantCode: 1, wantText: "var Broken = 1"},
		{name: "same-size source change", stale: true, wantCode: 1, wantText: "var Broken = 2"},
		{name: "same-size source change in diff", diff: true, stale: true, wantCode: 1, wantText: "var Broken = 2"},
		{name: "diff output failure", diff: true, outputFailure: true, wantCode: 1, wantText: "var Broken = 1"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cwd, err := os.Getwd()
			require.NoError(t, err)
			require.NoError(t, os.Chdir(dir))
			t.Cleanup(func() { require.NoError(t, os.Chdir(cwd)) })
			require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/fix\n\ngo 1.25\n"), 0o644))
			path := filepath.Join(dir, "fix.go")
			require.NoError(t, os.WriteFile(path, []byte("package fix\nvar Broken = 1\n"), 0o644))
			if tc.tests {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "fix_test.go"), []byte("package fix\nimport \"testing\"\nfunc TestX(t *testing.T) {}\n"), 0o644))
			}
			git := func(args ...string) {
				t.Helper()
				cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
				out, err := cmd.CombinedOutput()
				require.NoError(t, err, "%s", out)
			}
			git("init", "-q")
			git("add", ".")
			git("-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-qm", "initial")
			a := fixAnalyzer(path, tc.fail, tc.conflict, tc.stale)
			oldout, olderr := os.Stdout, os.Stderr
			stdout, err := os.CreateTemp(t.TempDir(), "stdout")
			require.NoError(t, err)
			stderr, err := os.CreateTemp(t.TempDir(), "stderr")
			require.NoError(t, err)
			if tc.outputFailure {
				require.NoError(t, stdout.Close())
				stdout, err = os.Open(stdout.Name())
				require.NoError(t, err)
			}
			os.Stdout, os.Stderr = stdout, stderr
			code := driver.Run([]string{"."}, []*analysis.Analyzer{a}, driver.Options{Fix: true, Diff: tc.diff, IncludeTests: tc.tests, Context: -1})
			os.Stdout, os.Stderr = oldout, olderr
			require.Equal(t, tc.wantCode, code)
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Contains(t, string(data), tc.wantText)
			_, err = stderr.Seek(0, io.SeekStart)
			require.NoError(t, err)
			diagnostics, err := io.ReadAll(stderr)
			require.NoError(t, err)
			if tc.conflict {
				require.Contains(t, string(diagnostics), "unsupported")
			} else {
				require.NotContains(t, string(diagnostics), "unsupported")
			}
			_, err = stdout.Seek(0, io.SeekStart)
			require.NoError(t, err)
			output, err := io.ReadAll(stdout)
			require.NoError(t, err)
			if expectsDiff(tc.diff, tc.stale, tc.outputFailure) {
				require.Contains(t, string(output), "-var Broken = 1\n")
				require.Contains(t, string(output), "+var Fixed = 1\n")
			} else {
				require.Empty(t, output)
			}
			require.NoError(t, stdout.Close())
			require.NoError(t, stderr.Close())
		})
	}
}
