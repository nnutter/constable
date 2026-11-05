package driver_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/packages"

	"github.com/nnutter/constable/internal/driver"
)

func TestRunCallsAfterLoadBeforeAnalysis(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { require.NoError(t, os.Chdir(cwd)) })
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/hook\n\ngo 1.25\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hook.go"), []byte("package hook\n"), 0o644))

	var order []string
	var roots []*packages.Package
	analyzer := &analysis.Analyzer{
		Name: "noop",
		Doc:  "reports nothing",
		Run: func(*analysis.Pass) (any, error) {
			order = append(order, "analyze")
			return nil, nil
		},
	}

	code := driver.Run([]string{"."}, []*analysis.Analyzer{analyzer}, driver.Options{
		Context:      -1,
		IncludeTests: true,
		AfterLoad: func(pkgs []*packages.Package) {
			order = append(order, "load")
			roots = pkgs
		},
	})

	require.Equal(t, 0, code)
	require.Equal(t, []string{"load", "analyze"}, order)
	require.Len(t, roots, 1)
	require.Equal(t, "example.com/hook", roots[0].PkgPath)
	require.NotNil(t, roots[0].Module)
	require.Equal(t, "example.com/hook", roots[0].Module.Path)
	require.True(t, roots[0].Module.Main)
}

func TestRunWithoutAfterLoad(t *testing.T) {
	dir := t.TempDir()
	cwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { require.NoError(t, os.Chdir(cwd)) })
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/nohook\n\ngo 1.25\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hook.go"), []byte("package nohook\n"), 0o644))

	analyzer := &analysis.Analyzer{Name: "noop", Doc: "reports nothing", Run: func(*analysis.Pass) (any, error) { return nil, nil }}
	code := driver.Run([]string{"."}, []*analysis.Analyzer{analyzer}, driver.Options{Context: -1, IncludeTests: true})

	require.Equal(t, 0, code)
}
