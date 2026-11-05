package methodical

import (
	"bytes"
	"cmp"
	"go/ast"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/nnutter/constable/internal/generated"
)

type diagnosticReporter struct {
	pass    *analysis.Pass
	methods []methodInfo
	offered bool
}

func (r *diagnosticReporter) report(d analysis.Diagnostic, obj types.Object) {
	if !r.offered {
		if fix := suggestedFix(r.pass, r.methods, obj); fix != nil {
			d.SuggestedFixes = []analysis.SuggestedFix{*fix}
			r.offered = true
		}
	}
	r.pass.Report(d)
}

type fixFile struct {
	file   *ast.File
	token  *token.File
	source []byte
}

type fixType struct {
	file   *fixFile
	anchor token.Pos
	order  int
}

type fixMethod struct {
	method     methodInfo
	file       *fixFile
	start, end token.Pos
	text       []byte
	target     fixType
}

// Offer at most one receiver fix per analysis to prevent overlapping edits.
func suggestedFix(pass *analysis.Pass, methods []methodInfo, receiver types.Object) *analysis.SuggestedFix {
	files, typesByObject := collectFixFiles(pass)
	candidates, unsafe := collectFixMethods(pass, methods, receiver, files, typesByObject)
	return makeSuggestedFix(candidates, unsafe)
}

func collectFixFiles(pass *analysis.Pass) (map[string]*fixFile, map[types.Object]fixType) {
	files := make(map[string]*fixFile)
	typesByObject := make(map[types.Object]fixType)
	for _, file := range pass.Files {
		f := readFixFile(pass, file)
		if f == nil {
			continue
		}
		files[f.token.Name()] = f
		for _, decl := range file.Decls {
			obj, target := fixTypeForDecl(pass, f, decl)
			if obj != nil {
				typesByObject[obj] = target
			}
		}
	}
	return files, typesByObject
}

func readFixFile(pass *analysis.Pass, file *ast.File) *fixFile {
	tf := pass.Fset.File(file.Pos())
	if tf == nil {
		return nil
	}
	src, err := pass.ReadFile(tf.Name())
	if err != nil || generated.IsGenerated(file) || hasStandaloneComments(pass.Fset, file) {
		return nil
	}
	return &fixFile{file: file, token: tf, source: src}
}

// Moving a declaration out of a file can orphan an import, and moving one in
// can need an import, so each direction gets its own rule.
func (f *fixFile) moveSourceSafe() bool {
	return f.plain() && hasNoImports(f.file)
}

func (f *fixFile) moveTargetSafe(pass *analysis.Pass, decl *ast.FuncDecl) bool {
	if !f.plain() {
		return false
	}
	if isCgoFile(f.file) {
		return false
	}
	imports := fileImports(pass, f.file)
	for _, ref := range declImports(pass, decl) {
		if !hasImport(imports, ref) {
			return false
		}
	}
	return true
}

// A cross-file move must not cross build constraints or position directives,
// or the method could leave some builds. The file name can carry a constraint,
// and so can a directive inside the file.
func (f *fixFile) plain() bool {
	base := strings.TrimSuffix(filepath.Base(f.token.Name()), ".go")
	if strings.Contains(base, "_") {
		return false
	}
	return !containsDirective(f.source) && !bytes.Contains(f.source, []byte("// +build"))
}

// importRef names an import the way the code it serves names it.
type importRef struct {
	name string
	path string
}

// declImports lists the imports that a declaration needs to compile in a
// different file. A qualified identifier needs the name that qualifies it, and
// a bare identifier from another package needs a dot import.
func declImports(pass *analysis.Pass, decl *ast.FuncDecl) []importRef {
	var refs []importRef
	selectors := make(map[*ast.Ident]bool)
	seen := make(map[importRef]bool)
	ast.Inspect(decl, func(node ast.Node) bool {
		if sel, ok := node.(*ast.SelectorExpr); ok {
			selectors[sel.Sel] = true
		}
		id, ok := node.(*ast.Ident)
		if !ok {
			return true
		}
		if selectors[id] {
			return true
		}
		ref, ok := identImport(pass, id)
		if !ok {
			return true
		}
		if seen[ref] {
			return true
		}
		seen[ref] = true
		refs = append(refs, ref)
		return true
	})
	return refs
}

func identImport(pass *analysis.Pass, id *ast.Ident) (importRef, bool) {
	obj := pass.TypesInfo.Uses[id]
	if pkgName, ok := obj.(*types.PkgName); ok {
		return importRef{name: pkgName.Name(), path: pkgName.Imported().Path()}, true
	}
	// A bare, unqualified identifier with an object from another package comes
	// from a dot import.
	if obj == nil {
		return importRef{}, false
	}
	pkg := obj.Pkg()
	if pkg == nil {
		return importRef{}, false
	}
	if pkg == pass.Pkg {
		return importRef{}, false
	}
	return importRef{name: ".", path: pkg.Path()}, true
}

// fileImports maps each name that the file imports to the path it names.
func fileImports(pass *analysis.Pass, file *ast.File) map[string]string {
	imports := make(map[string]string)
	for _, spec := range file.Imports {
		pkgName := importPkgName(pass, spec)
		if pkgName == nil {
			continue
		}
		imports[pkgName.Name()] = pkgName.Imported().Path()
	}
	return imports
}

func importPkgName(pass *analysis.Pass, spec *ast.ImportSpec) *types.PkgName {
	if spec.Name != nil {
		if pkgName, ok := pass.TypesInfo.Defs[spec.Name].(*types.PkgName); ok {
			return pkgName
		}
		return nil
	}
	if pkgName, ok := pass.TypesInfo.Implicits[spec].(*types.PkgName); ok {
		return pkgName
	}
	return nil
}

func hasImport(imports map[string]string, ref importRef) bool {
	return imports[ref.name] == ref.path
}

func isCgoFile(file *ast.File) bool {
	return slices.ContainsFunc(file.Imports, isCgoImport)
}

func isCgoImport(spec *ast.ImportSpec) bool {
	path, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		return false
	}
	return path == "C"
}

func hasNoImports(file *ast.File) bool {
	return len(file.Imports) == 0
}

func containsDirective(src []byte) bool {
	return bytes.Contains(src, []byte("//go:")) || bytes.Contains(src, []byte("/*go:")) || bytes.Contains(src, []byte("//line ")) || bytes.Contains(src, []byte("/*line "))
}

func fixTypeForDecl(pass *analysis.Pass, file *fixFile, decl ast.Decl) (types.Object, fixType) {
	gen, ok := decl.(*ast.GenDecl)
	if !ok {
		return nil, fixType{}
	}
	if gen.Tok != token.TYPE {
		return nil, fixType{}
	}
	if len(gen.Specs) != 1 {
		return nil, fixType{}
	}
	end := file.token.Offset(gen.End())
	lineEnd := endOfLine(file.source, end)
	if !safeLineSuffix(file.source[end:lineEnd]) {
		return nil, fixType{}
	}
	ts, ok := gen.Specs[0].(*ast.TypeSpec)
	if !ok {
		return nil, fixType{}
	}
	return pass.TypesInfo.Defs[ts.Name], fixType{file: file, anchor: file.token.Pos(lineEnd)}
}

func endOfLine(src []byte, start int) int {
	if n := bytes.IndexByte(src[start:], '\n'); n >= 0 {
		return start + n
	}
	return len(src)
}

func safeLineSuffix(suffix []byte) bool {
	rest := strings.TrimSpace(string(suffix))
	return rest == "" || strings.HasPrefix(rest, "//")
}

func collectFixMethods(pass *analysis.Pass, methods []methodInfo, receiver types.Object, files map[string]*fixFile, typesByObject map[types.Object]fixType) ([]fixMethod, map[types.Object]bool) {
	var candidates []fixMethod
	unsafe := make(map[types.Object]bool)
	for _, m := range methods {
		if m.typeObj != receiver {
			continue
		}
		candidate, ok := fixMethodFor(pass, m, files[m.file], typesByObject[m.typeObj])
		if !ok {
			unsafe[m.typeObj] = true
			continue
		}
		candidates = append(candidates, candidate)
	}
	return candidates, unsafe
}

func fixMethodFor(pass *analysis.Pass, m methodInfo, srcFile *fixFile, target fixType) (fixMethod, bool) {
	if srcFile == nil {
		return fixMethod{}, false
	}
	if target.file == nil {
		return fixMethod{}, false
	}
	if srcFile != target.file {
		if !srcFile.moveSourceSafe() {
			return fixMethod{}, false
		}
		if !target.file.moveTargetSafe(pass, m.decl) {
			return fixMethod{}, false
		}
	}
	start := m.decl.Pos()
	if m.decl.Doc != nil {
		start = m.decl.Doc.Pos()
	}
	if pass.Fset.Position(start).Column != 1 {
		return fixMethod{}, false
	}
	end := srcFile.token.Offset(m.decl.End())
	lineEnd := endOfLine(srcFile.source, end)
	if !safeLineSuffix(srcFile.source[end:lineEnd]) {
		return fixMethod{}, false
	}
	text := srcFile.source[srcFile.token.Offset(start):lineEnd]
	if containsDirective(text) {
		return fixMethod{}, false
	}
	deleteEnd := afterBlankLines(srcFile.source, lineEnd)
	return fixMethod{method: m, file: srcFile, start: start, end: srcFile.token.Pos(deleteEnd), text: text, target: target}, true
}

func afterBlankLines(src []byte, lineEnd int) int {
	if lineEnd == len(src) {
		return lineEnd
	}
	end := lineEnd + 1
	for end < len(src) {
		next := endOfLine(src, end)
		if next == len(src) {
			break
		}
		if len(bytes.Trim(src[end:next], " \t\r")) != 0 {
			break
		}
		end = next + 1
	}
	return end
}

func makeSuggestedFix(candidates []fixMethod, unsafe map[types.Object]bool) *analysis.SuggestedFix {
	groups := make(map[token.Pos][]fixMethod)
	for _, m := range candidates {
		if !unsafe[m.method.typeObj] {
			groups[m.target.anchor] = append(groups[m.target.anchor], m)
		}
	}
	var edits []analysis.TextEdit
	for anchor, group := range groups {
		trimMovedTail(group)
		slices.SortFunc(group, func(a, b fixMethod) int {
			return cmp.Or(cmp.Compare(a.target.order, b.target.order), cmp.Compare(genericRank(a.method.decl), genericRank(b.method.decl)), strings.Compare(a.method.methodName, b.method.methodName))
		})
		var insertion []byte
		for _, m := range group {
			insertion = append(insertion, '\n', '\n')
			insertion = append(insertion, m.text...)
			edits = append(edits, analysis.TextEdit{Pos: m.start, End: m.end})
		}
		edits = append(edits, analysis.TextEdit{Pos: anchor, End: anchor, NewText: insertion})
	}
	if len(edits) == 0 {
		return nil
	}
	return &analysis.SuggestedFix{Message: "Move and order methods after their types", TextEdits: edits}
}

// trimMovedTail takes the blank line before the declarations that a move
// removes from the end of a file. They kept the blank line that separated them
// from the declaration above, which would end the file as a blank line.
func trimMovedTail(group []fixMethod) {
	slices.SortFunc(group, func(a, b fixMethod) int {
		return cmp.Compare(a.start, b.start)
	})
	last := len(group) - 1
	fix := group[last].file
	if fix.token.Offset(group[last].end) != len(fix.source) {
		return
	}
	first := last
	for isAdjacentToPrevious(group, first) {
		first--
	}
	limit := 0
	if first > 0 {
		limit = fix.token.Offset(group[first-1].end)
	}
	start := trimBlankTail(fix.source, fix.token.Offset(group[first].start))
	if start > limit {
		group[first].start = fix.token.Pos(start)
	}
}

// trimBlankTail moves an offset back over the blank lines before it and leaves
// the newline that ends the line above.
func trimBlankTail(src []byte, offset int) int {
	end := offset
	for end > 0 && isBlankByte(src[end-1]) {
		end--
	}
	if n := bytes.IndexByte(src[end:offset], '\n'); n >= 0 {
		return end + n + 1
	}
	return offset
}

func isBlankByte(b byte) bool {
	switch b {
	case '\n', '\r', ' ', '\t':
		return true
	default:
		return false
	}
}

// isAdjacentToPrevious reports whether a removal starts where the one before it
// ends, so both reach back to the same blank line.
func isAdjacentToPrevious(group []fixMethod, i int) bool {
	if i == 0 {
		return false
	}
	return group[i-1].end == group[i].start
}

func genericRank(decl *ast.FuncDecl) int {
	if decl.Type.TypeParams != nil {
		return 0
	}
	return 1
}

// Standalone comments can describe adjacent declarations or sections. Their
// intended ownership is not reliable enough for a source move.
func hasStandaloneComments(fset *token.FileSet, file *ast.File) bool {
	for _, comment := range file.Comments {
		if comment.End() < file.Package {
			continue
		}
		if !commentHasOwner(fset, file, comment) {
			return true
		}
	}
	return false
}

func commentHasOwner(fset *token.FileSet, file *ast.File, comment *ast.CommentGroup) bool {
	for _, decl := range file.Decls {
		if comment.Pos() >= decl.Pos() {
			if comment.End() <= decl.End() {
				return true
			}
		}
		if comment.Pos() >= decl.End() {
			if fset.Position(comment.Pos()).Line == fset.Position(decl.End()).Line {
				return true
			}
		}
		switch d := decl.(type) {
		case *ast.FuncDecl:
			if d.Doc == comment {
				return true
			}
		case *ast.GenDecl:
			if d.Doc == comment {
				return true
			}
		}
	}
	return false
}
