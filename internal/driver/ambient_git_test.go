package driver_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nnutter/gittest"
	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"

	"github.com/nnutter/constable/internal/driver"
)

// git rebase --exec exports GIT_DIR for the repository being rebased. Git
// treats that variable as stronger than -C, so fixture setup must not follow it.
func TestFixLeavesAmbientRepositoryUntouched(t *testing.T) {
	gittest.WithSetenvEnviron(t)
	outer := t.TempDir()
	runGit(t, outer, "init", "-q")
	runGit(t, outer, "config", "user.email", "keep@example.com")
	runGit(t, outer, "config", "user.name", "Keep")
	require.NoError(t, os.WriteFile(filepath.Join(outer, "keep.txt"), []byte("keep\n"), 0o644))
	runGit(t, outer, "add", ".")
	runGit(t, outer, "commit", "-qm", "keep")
	worktree := t.TempDir()
	runGit(t, outer, "worktree", "add", "--detach", "-q", worktree)
	common := strings.TrimSpace(runGit(t, worktree, "rev-parse", "--path-format=absolute", "--git-common-dir"))
	gitDir := strings.TrimSpace(runGit(t, worktree, "rev-parse", "--absolute-git-dir"))
	configPath := filepath.Join(common, "config")
	before, err := os.ReadFile(configPath)
	require.NoError(t, err)

	t.Setenv("GIT_DIR", gitDir)
	t.Setenv("GIT_PREFIX", "")

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/fix\n\ngo 1.25\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.go"), []byte("package fix\nfunc a() { Broken := 1; _ = Broken }\n"), 0o644))
	runGit(t, dir, "init", "-q")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-qm", "initial")
	t.Chdir(dir)
	code := driver.Run([]string{"."}, []*analysis.Analyzer{fixAnalyzer("", false, false, false)}, driver.Options{Fix: true, Context: -1})
	require.Equal(t, 0, code)
	after, err := os.ReadFile("a.go")
	require.NoError(t, err)
	require.Contains(t, string(after), "Fixed")

	config, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.Equal(t, string(before), string(config))
	runGit(t, worktree, "status", "--porcelain")
}
