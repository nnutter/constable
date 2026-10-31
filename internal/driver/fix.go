package driver

import (
	"bytes"
	"fmt"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
)

type fixEdit struct {
	start, end int
	text       string
}
type fixFile struct {
	original []byte
	edits    []fixEdit
	result   []byte
}

// outputFixes validates the complete final batch before producing any output.
// Each file contains its original on-disk contents and its final result.
func outputFixes(files map[string]*fixFile, cwd string, printDiff bool) error {
	var names []string
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		if err := validateFixFile(name, files[name], printDiff); err != nil {
			return err
		}
	}
	for _, name := range names {
		file := files[name]
		if file == nil {
			return fmt.Errorf("missing fix file %s", name)
		}
		if err := outputFixFile(name, file, cwd, printDiff); err != nil {
			return err
		}
	}
	return nil
}

func validateFixFile(name string, file *fixFile, printDiff bool) error {
	if file == nil {
		return fmt.Errorf("missing fix file %s", name)
	}
	if err := checkFixFile(name, file, "since analysis"); err != nil {
		return err
	}
	if !printDiff {
		if !bytes.Equal(file.original, file.result) {
			return checkGitClean(name)
		}
	}
	return nil
}

func collectFixes(graph *checker.Graph, sources map[*token.File][]byte) (map[string]*fixFile, error) {
	files := make(map[string]*fixFile)
	for act := range graph.All() {
		if act.Err != nil {
			return nil, fmt.Errorf("analysis failed; suggested fixes not applied")
		}
	}
	for act := range graph.All() {
		if !act.IsRoot {
			continue
		}
		for _, diagnostic := range act.Diagnostics {
			if len(diagnostic.SuggestedFixes) == 0 {
				continue
			}
			for _, edit := range diagnostic.SuggestedFixes[0].TextEdits {
				if err := collectFixEdit(files, sources, act.Package.Fset, edit, diagnostic.Message); err != nil {
					return nil, err
				}
			}
		}
	}
	return files, nil
}

func collectFixEdit(files map[string]*fixFile, sources map[*token.File][]byte, fset *token.FileSet, edit analysis.TextEdit, message string) error {
	file := fset.File(edit.Pos)
	if file == nil {
		return fmt.Errorf("invalid suggested fix for %s", message)
	}
	if fset.File(edit.End) != file {
		return fmt.Errorf("invalid suggested fix for %s", message)
	}
	if edit.End < edit.Pos {
		return fmt.Errorf("invalid suggested fix for %s", message)
	}
	name := file.Name()
	source, ok := sources[file]
	if !ok {
		return fmt.Errorf("no analyzed source for %s", name)
	}
	f := files[name]
	if f == nil {
		f = &fixFile{original: source}
		files[name] = f
	} else if !bytes.Equal(f.original, source) {
		return fmt.Errorf("inconsistent analyzed source for %s", name)
	}
	f.edits = append(f.edits, fixEdit{file.Offset(edit.Pos), file.Offset(edit.End), string(edit.NewText)})
	return nil
}

func prepareFixFile(name string, f *fixFile) error {
	slices.SortFunc(f.edits, func(a, b fixEdit) int {
		if a.start != b.start {
			return a.start - b.start
		}
		if a.end != b.end {
			return a.end - b.end
		}
		return strings.Compare(a.text, b.text)
	})
	f.edits = slices.Compact(f.edits)
	var result bytes.Buffer
	end := 0
	for i, edit := range f.edits {
		if edit.start < end {
			return fmt.Errorf("conflicting suggested fixes in %s", name)
		}
		if i > 0 {
			if edit.start == f.edits[i-1].start {
				return fmt.Errorf("conflicting suggested fixes in %s", name)
			}
		}
		result.Write(f.original[end:edit.start])
		result.WriteString(edit.text)
		end = edit.end
	}
	result.Write(f.original[end:])
	f.result = result.Bytes()
	return nil
}

func checkFixFile(name string, f *fixFile, when string) error {
	current, err := os.ReadFile(name) // #nosec G304 -- The analyzed package determines the source path.
	if err != nil {
		return fmt.Errorf("read %s: %w", name, err)
	}
	if !bytes.Equal(current, f.original) {
		return fmt.Errorf("file %s changed %s", name, when)
	}
	return nil
}

// check rejects files that Git cannot confirm are tracked and safe to change.
func checkGitClean(name string) error {
	abs, err := filepath.Abs(name)
	if err != nil {
		return fmt.Errorf("resolve %s: %w", name, err)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return fmt.Errorf("inspect %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("file %s is not a regular file", name)
	}
	dir, base := filepath.Dir(abs), filepath.Base(abs)
	pathspec := ":(literal)" + base
	git := func(args ...string) ([]byte, error) {
		cmd := exec.Command("git", append([]string{"--no-optional-locks", "-C", dir}, args...)...) // #nosec G204 -- The path is passed as a literal Git pathspec.
		return cmd.Output()
	}
	tracked, err := git("ls-files", "-v", "-z", "--", pathspec)
	if err != nil {
		return fmt.Errorf("check Git tracking for %s: %w", name, err)
	}
	// H denotes an ordinary tracked file. Lowercase and S denote index flags
	// that can hide worktree changes; other entries include unmerged files.
	if len(tracked) < 3 {
		return fmt.Errorf("file %s is not safely tracked by Git", name)
	}
	if tracked[0] != 'H' {
		return fmt.Errorf("file %s is not safely tracked by Git", name)
	}
	if bytes.Count(tracked, []byte{0}) != 1 {
		return fmt.Errorf("file %s is not safely tracked by Git", name)
	}
	status, err := git("status", "--porcelain=v1", "-z", "--untracked-files=all", "--", pathspec)
	if err != nil {
		return fmt.Errorf("check Git status for %s: %w", name, err)
	}
	if len(status) != 0 {
		return fmt.Errorf("file %s has Git changes", name)
	}
	return nil
}

func outputFixFile(name string, f *fixFile, cwd string, printDiff bool) error {
	if bytes.Equal(f.original, f.result) {
		return nil
	}
	if printDiff {
		if err := writeUnifiedDiff(os.Stdout, relativePath(cwd, name), f.original, f.result); err != nil {
			return fmt.Errorf("write diff for %s: %w", name, err)
		}
		return nil
	}
	if err := checkFixFile(name, f, "during fix application"); err != nil {
		return err
	}
	if err := os.WriteFile(name, f.result, 0); err != nil {
		return fmt.Errorf("write %s: %w", name, err)
	}
	return nil
}

// Emit a valid unified diff with one whole-file hunk per changed file.
func writeUnifiedDiff(w io.Writer, name string, old, new []byte) error {
	var output bytes.Buffer
	lines := func(b []byte) []string {
		if len(b) == 0 {
			return nil
		}
		return strings.SplitAfter(string(b), "\n")[:strings.Count(string(b), "\n")+boolInt(b[len(b)-1] != '\n')]
	}
	a, b := lines(old), lines(new)
	start := func(n int) int {
		if n == 0 {
			return 0
		}
		return 1
	}
	fmt.Fprintf(&output, "--- %s\n+++ %s\n@@ -%d,%d +%d,%d @@\n", name, name, start(len(a)), len(a), start(len(b)), len(b))
	for _, group := range []struct {
		prefix string
		lines  []string
	}{{"-", a}, {"+", b}} {
		for _, line := range group.lines {
			fmt.Fprint(&output, group.prefix, line)
			if !strings.HasSuffix(line, "\n") {
				fmt.Fprint(&output, "\n\\ No newline at end of file\n")
			}
		}
	}
	n, err := w.Write(output.Bytes())
	if err == nil {
		if n != output.Len() {
			return io.ErrShortWrite
		}
	}
	return err
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
