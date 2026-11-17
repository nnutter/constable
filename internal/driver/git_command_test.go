package driver_test

import (
	"os"
	"os/exec"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return runGitInput(t, dir, "", args...)
}

func runGitInput(t *testing.T, dir, input string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = gitEnvWithoutRepository(os.Environ())
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	return string(out)
}

// gitEnvWithoutRepository matches gitRepositoryEnv in fix.go. git rebase --exec
// exports GIT_DIR and an empty GIT_PREFIX, and Git treats both as stronger than -C.
func gitEnvWithoutRepository(env []string) []string {
	filtered := make([]string, 0, len(env))
	for _, entry := range env {
		key, _, _ := strings.Cut(entry, "=")
		if gitRepositoryEnv(key) {
			continue
		}
		filtered = append(filtered, entry)
	}
	return filtered
}

func gitRepositoryEnv(key string) bool {
	switch key {
	case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_OBJECT_DIRECTORY", "GIT_INDEX_FILE", "GIT_NAMESPACE", "GIT_PREFIX":
		return true
	default:
		return false
	}
}
