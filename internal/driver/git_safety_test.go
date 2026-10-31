package driver_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis"

	"github.com/nnutter/constable/internal/driver"
)

func TestFixGitSafety(t *testing.T) {
	for _, tc := range []struct {
		name, change string
		diff         bool
		wantCode     int
	}{
		{name: "clean", wantCode: 0},
		{name: "unrelated changes", change: "unrelated", wantCode: 0},
		{name: "unstaged", change: "unstaged", wantCode: 1},
		{name: "staged", change: "staged", wantCode: 1},
		{name: "untracked", change: "untracked", wantCode: 1},
		{name: "assume unchanged", change: "assume", wantCode: 1},
		{name: "diff permits dirty files", change: "unstaged", diff: true, wantCode: 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			git := func(args ...string) {
				t.Helper()
				cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
				out, err := cmd.CombinedOutput()
				require.NoError(t, err, "%s", out)
			}
			write := func(name, value string) {
				t.Helper()
				require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(value), 0o644))
			}
			git("init", "-q")
			git("config", "user.email", "test@example.com")
			git("config", "user.name", "Test")
			write("go.mod", "module example.com/fix\n\ngo 1.25\n")
			write("a.go", "package fix\nfunc a() { Broken := 1; _ = Broken }\n")
			write("z.go", "package fix\nfunc z() { Broken := 2; _ = Broken }\n")
			git("add", ".")
			git("commit", "-qm", "initial")
			switch tc.change {
			case "unrelated":
				write("unrelated.txt", "dirty")
			case "unstaged", "staged":
				write("z.go", "package fix\nfunc z() { Broken := 3; _ = Broken }\n")
				if tc.change == "staged" {
					git("add", "z.go")
				}
			case "untracked":
				git("rm", "--cached", "z.go")
				git("commit", "-qm", "untrack")
			case "assume":
				git("update-index", "--assume-unchanged", "z.go")
			}
			cwd, err := os.Getwd()
			require.NoError(t, err)
			require.NoError(t, os.Chdir(dir))
			t.Cleanup(func() { require.NoError(t, os.Chdir(cwd)) })
			before, err := os.ReadFile("a.go")
			require.NoError(t, err)
			code := driver.Run([]string{"."}, []*analysis.Analyzer{fixAnalyzer("", false, false, false)}, driver.Options{Fix: true, Diff: tc.diff, Context: -1})
			require.Equal(t, tc.wantCode, code)
			after, err := os.ReadFile("a.go")
			require.NoError(t, err)
			if tc.wantCode == 1 {
				require.Equal(t, before, after)
			} else if tc.diff {
				require.Equal(t, before, after)
			} else {
				require.Contains(t, string(after), "Fixed")
			}
		})
	}
}
