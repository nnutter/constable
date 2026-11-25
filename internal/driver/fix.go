package driver

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
	"golang.org/x/tools/go/analysis/checker"
	"golang.org/x/tools/imports"
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

// collectFixes gathers the suggested-fix edits of every root action, along
// with the name of the analyzer that suggested each edit, so -fix-commits
// can group the commits by analyzer.
func collectFixes(graph *checker.Graph, sources map[*token.File][]byte) (map[string]*fixFile, map[string]map[string]bool, error) {
	files := make(map[string]*fixFile)
	analyzersByFile := make(map[string]map[string]bool)
	for act := range graph.All() {
		if act.Err != nil {
			return nil, nil, fmt.Errorf("analysis failed; suggested fixes not applied")
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
				name, err := collectFixEdit(files, sources, act.Package.Fset, edit, diagnostic.Message)
				if err != nil {
					return nil, nil, err
				}
				recordFixAnalyzer(analyzersByFile, name, act.Analyzer.Name)
			}
		}
	}
	return files, analyzersByFile, nil
}

func recordFixAnalyzer(analyzersByFile map[string]map[string]bool, name, analyzer string) {
	names := analyzersByFile[name]
	if names == nil {
		names = make(map[string]bool)
		analyzersByFile[name] = names
	}
	names[analyzer] = true
}

func mergeFixAnalyzers(dst, src map[string]map[string]bool) {
	for name, analyzers := range src {
		for analyzer := range analyzers {
			recordFixAnalyzer(dst, name, analyzer)
		}
	}
}

func collectFixEdit(files map[string]*fixFile, sources map[*token.File][]byte, fset *token.FileSet, edit analysis.TextEdit, message string) (string, error) {
	file := fset.File(edit.Pos)
	if file == nil {
		return "", fmt.Errorf("invalid suggested fix for %s", message)
	}
	if fset.File(edit.End) != file {
		return "", fmt.Errorf("invalid suggested fix for %s", message)
	}
	if edit.End < edit.Pos {
		return "", fmt.Errorf("invalid suggested fix for %s", message)
	}
	name := file.Name()
	source, ok := sources[file]
	if !ok {
		return "", fmt.Errorf("no analyzed source for %s", name)
	}
	f := files[name]
	if f == nil {
		f = &fixFile{original: source}
		files[name] = f
	} else if !bytes.Equal(f.original, source) {
		return "", fmt.Errorf("inconsistent analyzed source for %s", name)
	}
	f.edits = append(f.edits, fixEdit{file.Offset(edit.Pos), file.Offset(edit.End), string(edit.NewText)})
	return name, nil
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
		// -C selects the file's repository. GIT_DIR from git rebase --exec would override it.
		cmd.Env = gitEnvWithoutRepository(os.Environ())
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

// git rebase --exec exports GIT_DIR for the repository being rebased and an
// empty GIT_PREFIX. Git treats both as stronger than -C.
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

func validateFixCommits(opts Options) error {
	if !opts.FixCommits {
		return nil
	}
	if !opts.Fix {
		return fmt.Errorf("-fix-commits requires -fix")
	}
	if opts.Diff {
		return fmt.Errorf("-fix-commits cannot be used with -diff")
	}
	return nil
}

// fixCommitGroup is one -fix-commits commit: the changed files that one
// analyzer (or one combination of analyzers) fixed.
type fixCommitGroup struct {
	message string
	names   []string
}

func commitFinalFixes(original, state map[string][]byte, analyzersByFile map[string]map[string]bool, cwd string, enabled bool) error {
	if !enabled {
		return nil
	}
	for _, group := range fixCommitGroups(changedFixFiles(original, state), analyzersByFile, cwd) {
		if err := commitFixGroup(group, cwd); err != nil {
			return err
		}
	}
	return nil
}

// repairImports fixes the import block of each file a fix pass changed, in
// place in next. A suggested fix can remove the last use of an import
// without removing the import itself (testifylint's error-is-as and
// contains fixes do this), and the next pass cannot load a package whose
// files import a package they no longer use. Repairing the imports between
// passes is the same cleanup goimports runs between tools in a fix
// pipeline. Only the import lines change: the rest of the file keeps the
// exact bytes the suggested fixes produced. A file whose imports cannot be
// repaired keeps its bytes, so the next load reports the real problem
// instead.
func repairImports(next map[string][]byte, files map[string]*fixFile) {
	for name := range files {
		src, ok := next[name]
		if !ok {
			continue
		}
		if repaired, ok := repairFileImports(name, src); ok {
			next[name] = repaired
		}
	}
}

// repairFileImports asks goimports which imports the file should have, then
// applies only the import-line additions and removals to src. It reports
// false when the file needs no import change or its imports cannot be
// edited safely.
func repairFileImports(name string, src []byte) ([]byte, bool) {
	processed, err := imports.Process(name, src, &imports.Options{Comments: true, TabIndent: true, TabWidth: 8})
	if err != nil {
		return nil, false
	}
	removed, added, ok := importSetDiff(src, processed)
	if !ok {
		return nil, false
	}
	if noImportChange(removed, added) {
		return nil, false
	}
	return applyImportEdits(name, src, removed, added)
}

func noImportChange(removed map[string]bool, added []string) bool {
	if len(removed) > 0 {
		return false
	}
	return len(added) == 0
}

// importSetDiff compares the import paths of two versions of one file.
func importSetDiff(src, processed []byte) (map[string]bool, []string, bool) {
	before, ok := importPaths(src)
	if !ok {
		return nil, nil, false
	}
	after, ok := importPaths(processed)
	if !ok {
		return nil, nil, false
	}
	removed := make(map[string]bool)
	for path := range before {
		if !after[path] {
			removed[path] = true
		}
	}
	var added []string
	for path := range after {
		if !before[path] {
			added = append(added, path)
		}
	}
	slices.Sort(added)
	return removed, added, true
}

func importPaths(src []byte) (map[string]bool, bool) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", src, 0)
	if err != nil {
		return nil, false
	}
	paths := make(map[string]bool)
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, false
		}
		paths[path] = true
	}
	return paths, true
}

// applyImportEdits removes the import lines for removed and inserts lines
// for added. It refuses files whose import block holds comments, mirroring
// the analyzers' own import fixes: a comment could describe an import that
// the edit would orphan.
func applyImportEdits(name string, src []byte, removed map[string]bool, added []string) ([]byte, bool) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, name, src, parser.ParseComments)
	if err != nil {
		return nil, false
	}
	tokFile := fset.File(file.Pos())
	if tokFile == nil {
		return nil, false
	}
	offset := func(pos token.Pos) int { return tokFile.Offset(pos) }

	importDecls := collectImportDecls(file)
	if !importDeclsCommentFree(file, importDecls) {
		return nil, false
	}
	edits, ok := collectImportEdits(src, offset, file, importDecls, removed, added)
	if !ok {
		return nil, false
	}
	if len(edits) == 0 {
		return nil, false
	}
	return applyFixEdits(src, edits)
}

func isImportGenDecl(gen *ast.GenDecl) bool {
	return gen.Tok == token.IMPORT
}

func collectImportDecls(file *ast.File) []*ast.GenDecl {
	var decls []*ast.GenDecl
	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok {
			if isImportGenDecl(gen) {
				decls = append(decls, gen)
			}
		}
	}
	return decls
}

func importDeclsCommentFree(file *ast.File, decls []*ast.GenDecl) bool {
	for _, gen := range decls {
		if gen.Doc != nil {
			return false
		}
		if importDeclHasComment(file, gen) {
			return false
		}
	}
	return true
}

// collectImportEdits builds the removal and addition edits. When every
// import goes away and new ones arrive in the same pass, the first
// declaration is rewritten in place instead of removed, so the removal and
// addition edits cannot overlap.
func collectImportEdits(src []byte, offset func(token.Pos) int, file *ast.File, importDecls []*ast.GenDecl, removed map[string]bool, added []string) ([]fixEdit, bool) {
	replaceFirst, ok := replacesFirstDecl(importDecls, removed, added)
	if !ok {
		return nil, false
	}
	var edits []fixEdit
	for i, gen := range importDecls {
		if replaceFirst {
			if i == 0 {
				edit, ok := importReplacementEdit(src, offset, gen, added)
				if !ok {
					return nil, false
				}
				edits = append(edits, edit)
				continue
			}
		}
		genEdits, ok := importRemovalEdits(src, offset, gen, removed)
		if !ok {
			return nil, false
		}
		edits = append(edits, genEdits...)
	}
	if len(added) > 0 {
		if !replaceFirst {
			edit, ok := importAdditionEdit(src, offset, file, importDecls, added)
			if !ok {
				return nil, false
			}
			edits = append(edits, edit)
		}
	}
	return edits, true
}

// replacesFirstDecl reports whether added replaces every existing import,
// which only holds when there is at least one declaration to rewrite.
func replacesFirstDecl(importDecls []*ast.GenDecl, removed map[string]bool, added []string) (bool, bool) {
	if len(added) == 0 {
		return false, true
	}
	if len(importDecls) == 0 {
		return false, true
	}
	for _, gen := range importDecls {
		for _, spec := range gen.Specs {
			importSpec, ok := spec.(*ast.ImportSpec)
			if !ok {
				return false, false
			}
			path, err := strconv.Unquote(importSpec.Path.Value)
			if err != nil {
				return false, false
			}
			if !removed[path] {
				return false, true
			}
		}
	}
	return true, true
}

func commentWithinGen(comment *ast.CommentGroup, gen *ast.GenDecl) bool {
	if comment.Pos() < gen.Pos() {
		return false
	}
	return comment.End() <= gen.End()
}

func importDeclHasComment(file *ast.File, gen *ast.GenDecl) bool {
	for _, comment := range file.Comments {
		if commentWithinGen(comment, gen) {
			return true
		}
	}
	return false
}

func doomedSpecs(specs []*ast.ImportSpec, removed map[string]bool) ([]*ast.ImportSpec, bool) {
	var doomed []*ast.ImportSpec
	for _, spec := range specs {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			return nil, false
		}
		if removed[path] {
			doomed = append(doomed, spec)
		}
	}
	return doomed, true
}

func importRemovalEdits(src []byte, offset func(token.Pos) int, gen *ast.GenDecl, removed map[string]bool) ([]fixEdit, bool) {
	var specs []*ast.ImportSpec
	for _, spec := range gen.Specs {
		importSpec, ok := spec.(*ast.ImportSpec)
		if !ok {
			return nil, false
		}
		specs = append(specs, importSpec)
	}
	doomed, ok := doomedSpecs(specs, removed)
	if !ok {
		return nil, false
	}
	if len(doomed) == 0 {
		return nil, true
	}
	if len(doomed) == len(specs) {
		start := importLineStart(src, offset(gen.Pos()))
		end := importLineEnd(src, offset(gen.End()))
		if !safeImportSuffix(src[offset(gen.End()):end]) {
			return nil, false
		}
		return []fixEdit{{start: start, end: afterImportBlankLines(src, end)}}, true
	}
	var edits []fixEdit
	for _, spec := range doomed {
		start := importLineStart(src, offset(spec.Pos()))
		end := importLineEnd(src, offset(spec.End()))
		if !safeImportSuffix(src[offset(spec.End()):end]) {
			return nil, false
		}
		end = afterImportBlankLines(src, end)
		edits = append(edits, fixEdit{start: start, end: end})
	}
	return edits, true
}

// importReplacementEdit rewrites a declaration that loses every import
// into a declaration holding the added imports.
func importReplacementEdit(src []byte, offset func(token.Pos) int, gen *ast.GenDecl, added []string) (fixEdit, bool) {
	if !safeImportSuffix(src[offset(gen.End()):importLineEnd(src, offset(gen.End()))]) {
		return fixEdit{}, false
	}
	var text strings.Builder
	text.WriteString("import (\n")
	for _, path := range added {
		fmt.Fprintf(&text, "\t%q\n", path)
	}
	text.WriteString(")")
	return fixEdit{start: offset(gen.Pos()), end: offset(gen.End()), text: text.String()}, true
}

func importAdditionEdit(src []byte, offset func(token.Pos) int, file *ast.File, importDecls []*ast.GenDecl, added []string) (fixEdit, bool) {
	var lines strings.Builder
	for _, path := range added {
		fmt.Fprintf(&lines, "\t%q\n", path)
	}
	for _, gen := range importDecls {
		if gen.Lparen.IsValid() {
			// Insert before the line holding the closing parenthesis.
			return fixEdit{start: importLineStart(src, offset(gen.Rparen)), end: importLineStart(src, offset(gen.Rparen)), text: lines.String()}, true
		}
	}
	if len(importDecls) > 0 {
		gen := importDecls[0]
		end := importLineEnd(src, offset(gen.End()))
		var text strings.Builder
		for _, path := range added {
			fmt.Fprintf(&text, "\nimport %q", path)
		}
		return fixEdit{start: end, end: end, text: text.String()}, true
	}
	// No import declaration yet: add one after the package clause.
	end := importLineEnd(src, offset(file.Name.End()))
	return fixEdit{start: end, end: end, text: "\n\nimport (\n" + lines.String() + ")"}, true
}

// applyFixEdits splices edits into src. The edits must not overlap.
func applyFixEdits(src []byte, edits []fixEdit) ([]byte, bool) {
	slices.SortFunc(edits, func(a, b fixEdit) int {
		if a.start != b.start {
			return a.start - b.start
		}
		return a.end - b.end
	})
	var result bytes.Buffer
	end := 0
	for _, edit := range edits {
		if editOutOfOrder(edit, end) {
			return nil, false
		}
		result.Write(src[end:edit.start])
		result.WriteString(edit.text)
		end = edit.end
	}
	result.Write(src[end:])
	return result.Bytes(), true
}

func editOutOfOrder(edit fixEdit, end int) bool {
	if edit.start < end {
		return true
	}
	return edit.end < edit.start
}

func importLineStart(src []byte, offset int) int {
	for offset > 0 {
		if src[offset-1] == '\n' {
			break
		}
		offset--
	}
	return offset
}

// importLineEnd returns the offset of the newline ending the line, or the
// end of src. A removal edit ends one past it to take the newline along.
func importLineEnd(src []byte, offset int) int {
	if n := bytes.IndexByte(src[offset:], '\n'); n >= 0 {
		return offset + n + 1
	}
	return len(src)
}

// afterImportBlankLines extends an offset at a line start past any blank
// lines, so removing a declaration or spec takes its separating blank line.
func afterImportBlankLines(src []byte, offset int) int {
	for offset < len(src) {
		end := importLineEnd(src, offset)
		if len(bytes.Trim(src[offset:end], " \t\r\n")) != 0 {
			break
		}
		offset = end
	}
	return offset
}

func safeImportSuffix(suffix []byte) bool {
	rest := strings.TrimSpace(string(suffix))
	return rest == "" || strings.HasPrefix(rest, "//")
}

// fixCommitGroups groups changed files by the analyzers that fixed them,
// in analyzer order, so each commit holds one analyzer's fixes.
func fixCommitGroups(names []string, analyzersByFile map[string]map[string]bool, cwd string) []fixCommitGroup {
	namesByKey := make(map[string][]string)
	messageByKey := make(map[string]string)
	var keys []string
	for _, name := range names {
		analyzers := sortedFixAnalyzers(analyzersByFile[name])
		key := strings.Join(analyzers, "+")
		if _, ok := namesByKey[key]; !ok {
			messageByKey[key] = fixCommitMessage(analyzers, name, cwd)
			keys = append(keys, key)
		}
		namesByKey[key] = append(namesByKey[key], name)
	}
	slices.Sort(keys)
	groups := make([]fixCommitGroup, 0, len(keys))
	for _, key := range keys {
		groups = append(groups, fixCommitGroup{message: messageByKey[key], names: namesByKey[key]})
	}
	return groups
}

func sortedFixAnalyzers(analyzers map[string]bool) []string {
	names := make([]string, 0, len(analyzers))
	for name := range analyzers {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func fixCommitMessage(analyzers []string, name, cwd string) string {
	if len(analyzers) == 0 {
		return "Refactor: Apply constable fixes to " + relativePath(cwd, name)
	}
	return "Refactor: Apply constable's " + strings.Join(analyzers, " + ") + " fixes"
}

func changedFixFiles(original, state map[string][]byte) []string {
	var names []string
	for name, result := range state {
		if !bytes.Equal(original[name], result) {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

func commitFixGroup(group fixCommitGroup, cwd string) error {
	top, err := fixRepoTop(cwd)
	if err != nil {
		return err
	}
	specs, err := fixGroupPathspecs(group.names, top)
	if err != nil {
		return err
	}
	if _, err := runGitCommand(cwd, append([]string{"add", "--"}, specs...)...); err != nil {
		return fmt.Errorf("stage fixes for %q: %w", group.message, err)
	}
	commitArgs := append([]string{"commit", "-m", group.message, "--"}, specs...)
	if _, err := runGitCommand(cwd, commitArgs...); err != nil {
		return fmt.Errorf("commit fixes for %q: %w", group.message, err)
	}
	return nil
}

func fixRepoTop(cwd string) (string, error) {
	out, err := runGitCommand(cwd, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("find repository for fixes: %w", err)
	}
	return strings.TrimSpace(string(out)), nil
}

// fixGroupPathspecs names each file relative to the repository top, as a
// literal pathspec, so a group spanning directories commits exactly its
// files and no other staged change joins it.
func fixGroupPathspecs(names []string, top string) ([]string, error) {
	specs := make([]string, 0, len(names))
	for _, name := range names {
		abs, err := filepath.Abs(name)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", name, err)
		}
		rel, err := filepath.Rel(top, abs)
		if err != nil {
			return nil, fmt.Errorf("resolve %s: %w", name, err)
		}
		specs = append(specs, ":(literal)"+filepath.ToSlash(rel))
	}
	return specs, nil
}

func runGitCommand(dir string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"--no-optional-locks", "-C", dir}, args...)...) // #nosec G204 -- Args are fixed Git verbs, a fixed message, and a literal pathspec for an analyzed file.
	cmd.Env = gitEnvWithoutRepository(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return out, nil
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
