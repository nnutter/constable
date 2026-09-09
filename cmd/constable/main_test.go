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
	require.Error(t, err)
	assert.NotContains(t, output, moduleDir)
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

	require.Error(t, err)
	assert.Contains(t, output, "//constable:nonmutating function mutates pointer parameter p")
	assert.NotContains(t, output, moduleDir)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.NotEmpty(t, lines)
	assert.True(t, strings.HasPrefix(strings.TrimSpace(lines[0]), "a.go:5:2:"), "expected relative path, got: %s", lines[0])
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

	require.Error(t, err)
	assert.Contains(t, output, "method A of type T should be sorted before method B")
}

func TestCLITestifyFail(t *testing.T) {
	binary := buildBinary(t)
	moduleDir := writeModule(t, module{
		GoMod: []string{
			"module example.com/testify",
			"",
			"go 1.26",
		},
		GoFile: []string{
			"package testify",
			"",
			"import \"testing\"",
			"",
			"func F(t *testing.T) {",
			"\tt.Fatal(\"boom\")",
			"}",
		},
	})

	output, err := runConstable(t, binary, moduleDir)

	require.Error(t, err)
	assert.Contains(t, output, "use testify/require instead of testing.Fatal")
	assert.NotContains(t, output, moduleDir)
	lines := strings.Split(strings.TrimSpace(output), "\n")
	require.NotEmpty(t, lines)
	assert.True(t, strings.HasPrefix(strings.TrimSpace(lines[0]), "a.go:6:4:"), "expected relative path, got: %s", lines[0])
}

func TestCLIRelativeSubdirectory(t *testing.T) {
	binary := buildBinary(t)
	moduleDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "go.mod"), []byte("module example.com/subdir\n\ngo 1.26\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "a.go"), []byte("package subdir\n"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(moduleDir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(moduleDir, "sub", "b.go"), []byte("package sub\n\nimport \"testing\"\n\nfunc F(t *testing.T) {\n\tt.Fatal(\"boom\")\n}\n"), 0o644))

	output, err := runConstable(t, binary, moduleDir)

	require.Error(t, err)
	assert.Contains(t, output, "use testify/require instead of testing.Fatal")
	assert.NotContains(t, output, moduleDir)
	assert.Contains(t, output, "sub/b.go:6:4:")
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

func TestCLIComplexityFail(t *testing.T) {
	binary := buildBinary(t)

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)

	src := filepath.Join(repoRoot, "internal/analysis/complexity/testdata/src/d/d.go")
	content, err := os.ReadFile(src)
	require.NoError(t, err)

	moduleDir := writeModule(t, module{
		GoMod: []string{
			"module example.com/complexity",
			"",
			"go 1.26",
		},
		GoFile: strings.Split(strings.TrimSuffix(string(content), "\n"), "\n"),
	})

	output, err := runConstable(t, binary, moduleDir, "-complexity.limit=2")

	require.Error(t, err)
	assert.Contains(t, output, "cyclomatic complexity 3 exceeds limit 2 (function TwoBranches)")
	assert.NotContains(t, output, moduleDir)
}

func TestCLIComplexityPass(t *testing.T) {
	binary := buildBinary(t)
	moduleDir := writeModule(t, module{
		GoMod: []string{
			"module example.com/complexitypass",
			"",
			"go 1.26",
		},
		GoFile: []string{
			"package complexitypass",
			"",
			"func F(x int) int {",
			"\tif x > 0 {",
			"\t\treturn 1",
			"\t}",
			"\treturn 0",
			"}",
		},
	})

	output, err := runConstable(t, binary, moduleDir)

	require.NoError(t, err, output)
	assert.Empty(t, output)
}

func buildBinary(t *testing.T) string {
	t.Helper()

	return buildBinaryWithLDFlags(t)
}

func TestCLIVersionInjected(t *testing.T) {
	binary := buildBinaryWithLDFlags(t, "-X", "main.version=v9.9.9-test")

	command := exec.Command(binary, "-V")
	output, err := command.CombinedOutput()
	require.NoError(t, err)
	assert.Equal(t, strings.TrimSpace(filepath.Base(binary))+" v9.9.9-test", strings.TrimSpace(string(output)))
}

func buildBinaryWithLDFlags(t *testing.T, ldflags ...string) string {
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
	args := []string{"build", "-o", binary}
	if len(ldflags) > 0 {
		args = append(args, "-ldflags", strings.Join(ldflags, " "))
	}
	args = append(args, "./cmd/constable")
	command := exec.Command("go", args...)
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

func runConstable(t *testing.T, binary string, moduleDir string, extraArgs ...string) (string, error) {
	t.Helper()

	args := append(extraArgs, "./...")
	command := exec.Command(binary, args...)
	command.Dir = moduleDir
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()

	return output.String(), err
}
