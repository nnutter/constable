package driver_test

import (
	"go/ast"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"

	"github.com/nnutter/constable/internal/driver"
)

func runFixWithCommits(t *testing.T, analyzer *analysis.Analyzer, opts driver.Options) int {
	t.Helper()
	return runFixWithCommitsAnalyzers(t, []*analysis.Analyzer{analyzer}, opts)
}

func runFixWithCommitsAnalyzers(t *testing.T, analyzers []*analysis.Analyzer, opts driver.Options) int {
	t.Helper()
	out, err := os.CreateTemp(t.TempDir(), "out")
	require.NoError(t, err)
	errout, err := os.CreateTemp(t.TempDir(), "err")
	require.NoError(t, err)
	oldout, olderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = out, errout
	code := driver.Run([]string{"."}, analyzers, opts)
	os.Stdout, os.Stderr = oldout, olderr
	require.NoError(t, out.Close())
	require.NoError(t, errout.Close())
	return code
}

// renamingAnalyzer fixes only occurrences of from, under its own name, so
// commit tests can tell analyzers apart.
func renamingAnalyzer(name, from, to string) *analysis.Analyzer {
	return &analysis.Analyzer{Name: name, Doc: "rename " + from, Run: func(pass *analysis.Pass) (any, error) {
		for _, file := range pass.Files {
			ast.Inspect(file, func(n ast.Node) bool {
				id, ok := n.(*ast.Ident)
				if !ok {
					return true
				}
				if id.Name != from {
					return true
				}
				pass.Report(analysis.Diagnostic{Pos: id.Pos(), Message: "rename " + from, SuggestedFixes: []analysis.SuggestedFix{{Message: "rename", TextEdits: []analysis.TextEdit{{Pos: id.Pos(), End: id.End(), NewText: []byte(to)}}}}})
				return true
			})
		}
		return nil, nil
	}}
}

func gitLogSubjects(t *testing.T, dir string) []string {
	t.Helper()
	out := runGit(t, dir, "log", "--format=%s")
	trimmed := strings.TrimSpace(out)
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "\n")
}

func gitShowNames(t *testing.T, dir, rev string) string {
	t.Helper()
	return strings.TrimSpace(runGit(t, dir, "show", "--format=", "--name-only", rev))
}

func TestFixCommitsCreatesPerAnalyzerCommits(t *testing.T) {
	dir := fixRepoFiles(t, map[string]string{
		"a.go": "package fix\nfunc a() { Broken := 1; _ = Broken }\n",
		"z.go": "package fix\nfunc z() { Broken := 2; _ = Broken }\n",
	})
	code := runFixWithCommits(t, fixAnalyzer("", false, false, false), driver.Options{Fix: true, FixCommits: true, Context: -1})
	require.Equal(t, 0, code)
	for _, name := range []string{"a.go", "z.go"} {
		data, err := os.ReadFile(name)
		require.NoError(t, err)
		require.Contains(t, string(data), "Fixed")
	}
	// Both files were fixed by the same analyzer, so one commit holds both.
	subjects := gitLogSubjects(t, dir)
	require.Equal(t, []string{
		"Refactor: Apply constable's fixer fixes",
		"initial",
	}, subjects)
	require.Equal(t, "a.go\nz.go", gitShowNames(t, dir, "HEAD"))
	require.Empty(t, strings.TrimSpace(runGit(t, dir, "status", "--porcelain")))
}

func TestFixCommitsSeparatesAnalyzers(t *testing.T) {
	dir := fixRepoFiles(t, map[string]string{
		"a.go": "package fix\nfunc a() { Alpha := 1; _ = Alpha }\n",
		"b.go": "package fix\nfunc b() { Bravo := 2; _ = Bravo }\n",
	})
	analyzers := []*analysis.Analyzer{
		renamingAnalyzer("afixer", "Alpha", "FixedAlpha"),
		renamingAnalyzer("bfixer", "Bravo", "FixedBravo"),
	}
	code := runFixWithCommitsAnalyzers(t, analyzers, driver.Options{Fix: true, FixCommits: true, Context: -1})
	require.Equal(t, 0, code)
	require.Equal(t, []string{
		"Refactor: Apply constable's bfixer fixes",
		"Refactor: Apply constable's afixer fixes",
		"initial",
	}, gitLogSubjects(t, dir))
	require.Equal(t, "b.go", gitShowNames(t, dir, "HEAD"))
	require.Equal(t, "a.go", gitShowNames(t, dir, "HEAD~1"))
}

func TestFixCommitsCombinesAnalyzersInOneFile(t *testing.T) {
	dir := fixRepoFiles(t, map[string]string{
		"fix.go": "package fix\nfunc f() { Alpha := 1; Bravo := 2; _ = Alpha; _ = Bravo }\n",
	})
	analyzers := []*analysis.Analyzer{
		renamingAnalyzer("afixer", "Alpha", "FixedAlpha"),
		renamingAnalyzer("bfixer", "Bravo", "FixedBravo"),
	}
	code := runFixWithCommitsAnalyzers(t, analyzers, driver.Options{Fix: true, FixCommits: true, Context: -1})
	require.Equal(t, 0, code)
	require.Equal(t, []string{
		"Refactor: Apply constable's afixer + bfixer fixes",
		"initial",
	}, gitLogSubjects(t, dir))
	require.Equal(t, "fix.go", gitShowNames(t, dir, "HEAD"))
}

func TestFixCommitsLeavesOtherStagedChangesUncommitted(t *testing.T) {
	dir := fixRepoFiles(t, map[string]string{
		"a.go":      "package fix\nfunc a() { Broken := 1; _ = Broken }\n",
		"other.txt": "one\n",
	})
	require.NoError(t, os.WriteFile("other.txt", []byte("two\n"), 0o644))
	runGit(t, dir, "add", "other.txt")
	code := runFixWithCommits(t, fixAnalyzer("", false, false, false), driver.Options{Fix: true, FixCommits: true, Context: -1})
	require.Equal(t, 0, code)
	require.Equal(t, []string{"Refactor: Apply constable's fixer fixes", "initial"}, gitLogSubjects(t, dir))
	require.Equal(t, "a.go", gitShowNames(t, dir, "HEAD"))
	require.Contains(t, runGit(t, dir, "status", "--porcelain"), "M  other.txt")
}

func TestFixWithoutCommitsLeavesChangesUncommitted(t *testing.T) {
	dir := fixRepo(t, "package fix\nvar Broken = 1\n")
	code := runFixWithCommits(t, fixAnalyzer("", false, false, false), driver.Options{Fix: true, Context: -1})
	require.Equal(t, 0, code)
	require.Equal(t, []string{"initial"}, gitLogSubjects(t, dir))
	require.NotEmpty(t, strings.TrimSpace(runGit(t, dir, "status", "--porcelain")))
}

func TestFixCommitsWithNoFixesCreatesNoCommit(t *testing.T) {
	dir := fixRepo(t, "package fix\nvar Fixed = 1\n")
	code := runFixWithCommits(t, fixAnalyzer("", false, false, false), driver.Options{Fix: true, FixCommits: true, Context: -1})
	require.Equal(t, 0, code)
	require.Equal(t, []string{"initial"}, gitLogSubjects(t, dir))
}

func TestFixCommitsRequiresFixWithoutDiff(t *testing.T) {
	for _, tc := range []struct {
		name string
		opts driver.Options
	}{
		{name: "without fix", opts: driver.Options{FixCommits: true, Context: -1}},
		{name: "with diff", opts: driver.Options{Fix: true, Diff: true, FixCommits: true, Context: -1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := fixRepo(t, "package fix\nvar Broken = 1\n")
			code := runFixWithCommits(t, fixAnalyzer("", false, false, false), tc.opts)
			require.Equal(t, 1, code)
			require.Equal(t, []string{"initial"}, gitLogSubjects(t, dir))
			data, err := os.ReadFile("fix.go")
			require.NoError(t, err)
			require.Contains(t, string(data), "Broken")
		})
	}
}
