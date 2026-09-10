package main

import (
	"bytes"
	"maps"
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
	GoMod []string
	Files map[string][]string
}

func repoRoot(t *testing.T) string {
	t.Helper()

	root, err := filepath.Abs(filepath.Join("..", ".."))
	require.NoError(t, err)
	return root
}

func loadTestdata(t *testing.T, rel string) []string {
	t.Helper()

	content, err := os.ReadFile(filepath.Join(repoRoot(t), rel))
	require.NoError(t, err)
	return strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")
}

func extractWants(lines []string) []string {
	var wants []string
	for _, line := range lines {
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
	return wants
}

func assertDiagnostics(t *testing.T, output, moduleDir string, wants []string) {
	t.Helper()

	assert.NotContains(t, output, moduleDir)
	for _, want := range wants {
		assert.Contains(t, output, want)
	}
}

func TestCLIUsingAnalyzerTestData(t *testing.T) {
	binary := buildBinary(t)

	lines := loadTestdata(t, "internal/analysis/nonmutating/testdata/src/a/a.go")
	wants := extractWants(lines)

	moduleDir := writeModule(t, module{
		GoMod: []string{"module example.com/testdata", "", "go 1.26"},
		Files: map[string][]string{"a.go": lines},
	})
	output, err := runConstable(t, binary, moduleDir)
	require.Error(t, err)
	assertDiagnostics(t, output, moduleDir, wants)
}

func TestCLINonmutatingFail(t *testing.T) {
	binary := buildBinary(t)
	moduleDir := writeModule(t, module{
		GoMod: []string{
			"module example.com/failing",
			"",
			"go 1.26",
		},
		Files: map[string][]string{"a.go": {
			"package failing",
			"",
			"//constable:nonmutating",
			"func F(p *int) {",
			"\t*p = 1",
			"}",
		}},
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

	mLines := loadTestdata(t, "internal/analysis/methodical/testdata/src/m/m.go")
	otherLines := loadTestdata(t, "internal/analysis/methodical/testdata/src/m/other.go")
	wants := append(extractWants(mLines), extractWants(otherLines)...)

	moduleDir := writeModule(t, module{
		GoMod: []string{
			"module example.com/methodical",
			"",
			"go 1.26",
		},
		Files: map[string][]string{
			"m.go":     mLines,
			"other.go": otherLines,
		},
	})

	output, err := runConstable(t, binary, moduleDir)

	require.Error(t, err)
	assertDiagnostics(t, output, moduleDir, wants)
}

func TestCLITestifyFail(t *testing.T) {
	binary := buildBinary(t)

	lines := loadTestdata(t, "internal/analysis/testify/testdata/src/t/t.go")
	wants := extractWants(lines)

	moduleDir := writeModule(t, module{
		GoMod: []string{
			"module example.com/testify",
			"",
			"go 1.26",
		},
		Files: map[string][]string{"a.go": lines},
	})

	output, err := runConstable(t, binary, moduleDir)

	require.Error(t, err)
	assertDiagnostics(t, output, moduleDir, wants)
}

func TestCLIRelativeSubdirectory(t *testing.T) {
	binary := buildBinary(t)
	moduleDir := writeModule(t, module{
		GoMod: []string{
			"module example.com/subdir",
			"",
			"go 1.26",
		},
		Files: map[string][]string{
			"a.go": {
				"package subdir",
			},
			"sub/b.go": {
				"package sub",
				"",
				"import \"testing\"",
				"",
				"func F(t *testing.T) {",
				"\tt.Fatal(\"boom\")",
				"}",
			},
		},
	})

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
		Files: map[string][]string{"a.go": {
			"package passing",
			"",
			"//constable:nonmutating",
			"func F(p *int) int {",
			"\treturn *p",
			"}",
		}},
	})

	output, err := runConstable(t, binary, moduleDir)

	require.NoError(t, err, output)
	assert.Empty(t, output)
}

func TestCLIComplexityFail(t *testing.T) {
	binary := buildBinary(t)

	lines := loadTestdata(t, "internal/analysis/complexity/testdata/src/d/d.go")

	moduleDir := writeModule(t, module{
		GoMod: []string{
			"module example.com/complexity",
			"",
			"go 1.26",
		},
		Files: map[string][]string{"a.go": lines},
	})

	output, err := runConstable(t, binary, moduleDir, "-complexity.limit=2")

	require.Error(t, err)
	assert.Contains(t, output, "cyclomatic complexity 3 exceeds limit 2 (function TwoBranches)")
	assert.NotContains(t, output, moduleDir)
}

func TestCLIComplexityPass(t *testing.T) {
	binary := buildBinary(t)

	lines := loadTestdata(t, "internal/analysis/complexity/testdata/src/d/d.go")

	moduleDir := writeModule(t, module{
		GoMod: []string{
			"module example.com/complexitypass",
			"",
			"go 1.26",
		},
		Files: map[string][]string{"a.go": lines},
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

	repoRoot := repoRoot(t)

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
	files := map[string][]string{"go.mod": m.GoMod}
	maps.Copy(files, m.Files)
	for name, lines := range files {
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
