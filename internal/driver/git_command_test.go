package driver_test

import (
	"strings"
	"testing"

	"github.com/nnutter/gittest"
	"github.com/stretchr/testify/require"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return runGitInput(t, dir, "", args...)
}

func runGitInput(t *testing.T, dir, input string, args ...string) string {
	t.Helper()
	cmd := gittest.Command(t, dir, args...)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s", out)
	return string(out)
}
