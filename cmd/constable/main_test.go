package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type module struct {
	GoMod  []string
	GoFile []string
}

func TestCLIUsingAnalyzerTestData(t *testing.T) {
	binary := buildBinary(t)

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	src := filepath.Join(repoRoot, "internal/analysis/nonmutating/testdata/src/a/a.go")
	content, err := os.ReadFile(src)
	require.NoError(t, err)

	m := module{
		GoMod:  []string{"module example.com/testdata", "", "go 1.26"},
		GoFile: strings.Split(strings.TrimSuffix(string(content), "\n"), "\n"),
	}

	// Grab the "want" lines from our analysistest.TestData() so the CLI tests
	// automatically match the analyzer tests.
	var wants []string
	for _, line := range m.GoFile {
		_, after, ok := strings.Cut(line, `// want "`)
		if !ok {
			continue
		}
		before, _, ok := strings.Cut(after, `"`)
		if !ok {
			continue
		}
		wants = append(wants, before)
	}

	moduleDir := writeModule(t, m)
	output, err := runConstable(t, binary, moduleDir)
	assert.Error(t, err)
	for _, want := range wants {
		assert.Contains(t, output, want)
	}
}

func TestCLINonmutatingFail(t *testing.T) {
	binary := buildBinary(t)
	moduleDir := writeModule(t, module{
		GoMod: []string{
			"module example.com/failing",
			"",
			"go 1.26",
		},
		GoFile: []string{
			"package failing",
			"",
			"//constable:nonmutating",
			"func F(p *int) {",
			"\t*p = 1",
			"}",
		},
	})

	output, err := runConstable(t, binary, moduleDir)

	assert.Error(t, err)
	assert.Contains(t, output, "a.go:5:2")
	assert.Contains(t, output, "//constable:nonmutating function mutates pointer parameter p")
}

func TestCLIMethodicalFail(t *testing.T) {
	binary := buildBinary(t)
	moduleDir := writeModule(t, module{
		GoMod: []string{
			"module example.com/methodical",
			"",
			"go 1.26",
		},
		GoFile: []string{
			"package methodical",
			"",
			"type T struct{}",
			"",
			"func (t T) B() {}",
			"",
			"func (t T) A() {}",
		},
	})

	output, err := runConstable(t, binary, moduleDir)

	assert.Error(t, err)
	assert.Contains(t, output, "method A of type T should be sorted before method B")
}

func TestCLINonmutating(t *testing.T) {
	binary := buildBinary(t)
	moduleDir := writeModule(t, module{
		GoMod: []string{
			"module example.com/passing",
			"",
			"go 1.26",
		},
		GoFile: []string{
			"package passing",
			"",
			"//constable:nonmutating",
			"func F(p *int) int {",
			"\treturn *p",
			"}",
		},
	})

	output, err := runConstable(t, binary, moduleDir)

	require.NoError(t, err, output)
	assert.Empty(t, output)
}

func buildBinary(t *testing.T) string {
	t.Helper()

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	tmpRoot := filepath.Join(repoRoot, "tmp")
	require.NoError(t, os.MkdirAll(tmpRoot, 0o755))
	t.Setenv("GOTMPDIR", tmpRoot)

	binary := filepath.Join(t.TempDir(), "constable")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	command := exec.Command("go", "build", "-o", binary, "./cmd/constable")
	command.Dir = repoRoot
	output, err := command.CombinedOutput()
	require.NoError(t, err, string(output))

	return binary
}

func writeModule(t *testing.T, m module) string {
	t.Helper()

	moduleDir := t.TempDir()
	for name, lines := range map[string][]string{"go.mod": m.GoMod, "a.go": m.GoFile} {
		path := filepath.Join(moduleDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
		require.NoError(t, os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644))
	}

	return moduleDir
}

func runConstable(t *testing.T, binary string, moduleDir string) (string, error) {
	t.Helper()

	command := exec.Command(binary, "./...")
	command.Dir = moduleDir
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()

	return output.String(), err
}
