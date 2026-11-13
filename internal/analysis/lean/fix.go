package lean

import (
	"bytes"
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

type reporter struct {
	offered bool
}

func newReporter(pass *analysis.Pass) *reporter {
	return &reporter{}
}

func readFixFile(pass *analysis.Pass, file *ast.File) *fixFile {
	tf := pass.Fset.File(file.Pos())
	if tf == nil {
		return nil
	}
	src, err := pass.ReadFile(tf.Name())
	if err != nil || generated.Skipped(file) || hasStandaloneComments(pass.Fset, file) {
		return nil
	}
	return &fixFile{file: file, token: tf, source: src}
}

func containsDirective(src []byte) bool {
	return bytes.Contains(src, []byte("//go:")) || bytes.Contains(src, []byte("/*go:")) || bytes.Contains(src, []byte("//line ")) || bytes.Contains(src, []byte("/*line "))
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
		if hasOwner(fset, decl, comment) {
			return true
		}
	}
	return false
}

func hasOwner(fset *token.FileSet, decl ast.Decl, comment *ast.CommentGroup) bool {
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
	return false
}

func candidateDecl(pass *analysis.Pass, sym symbol) (ast.Decl, *ast.GenDecl, ast.Spec) {
	if funcDecl, ok := sym.decl.(*ast.FuncDecl); ok {
		return funcDecl, nil, nil
	}
	gen, spec := containingGen(pass, sym)
	return nil, gen, spec
}

func specMatches(spec ast.Spec, sym symbol) bool {
	sx, ok := sym.decl.(ast.Spec)
	if !ok {
		return false
	}
	return sx == spec
}

func containingGen(pass *analysis.Pass, sym symbol) (*ast.GenDecl, ast.Spec) {
	for _, decl := range sym.file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			if specMatches(spec, sym) {
				return gen, spec
			}
		}
	}
	return nil, nil
}

func fixTarget(pass *analysis.Pass, sym symbol) (start, end token.Pos, whole bool, ok bool) {
	decl, gen, spec := candidateDecl(pass, sym)
	if decl != nil {
		return declStart(decl), decl.End(), true, true
	}
	if gen != nil {
		return specTarget(pass, sym, gen, spec)
	}
	return 0, 0, false, false
}

func declStart(node ast.Node) token.Pos {
	switch node := node.(type) {
	case *ast.FuncDecl:
		if node.Doc != nil {
			return node.Doc.Pos()
		}
		return node.Pos()
	case *ast.GenDecl:
		if node.Doc != nil {
			return node.Doc.Pos()
		}
		return node.Pos()
	}
	return node.Pos()
}

func specTarget(pass *analysis.Pass, sym symbol, gen *ast.GenDecl, spec ast.Spec) (token.Pos, token.Pos, bool, bool) {
	if spec == nil {
		return 0, 0, false, false
	}
	if len(gen.Specs) == 1 {
		return declStart(gen), gen.End(), true, true
	}
	if gen.Tok == token.CONST {
		return 0, 0, false, false
	}
	return specStart(spec), spec.End(), false, true
}

func specStart(spec ast.Spec) token.Pos {
	switch spec := spec.(type) {
	case *ast.TypeSpec:
		if spec.Doc != nil {
			return spec.Doc.Pos()
		}
		return spec.Pos()
	case *ast.ValueSpec:
		if spec.Doc != nil {
			return spec.Doc.Pos()
		}
		return spec.Pos()
	}
	return spec.Pos()
}

func suggestedFix(pass *analysis.Pass, sym symbol) *analysis.SuggestedFix {
	file := readFixFile(pass, sym.file)
	if file == nil {
		return nil
	}
	if !file.plain() || isCgoFile(file.file) {
		return nil
	}
	start, end, whole, ok := fixTarget(pass, sym)
	if !ok {
		return nil
	}
	edits, ok := file.removalEdits(pass, sym, start, end, whole)
	if !ok {
		return nil
	}
	return &analysis.SuggestedFix{
		Message:   "Remove unused unexported declaration " + sym.name,
		TextEdits: edits,
	}
}

func endOfLine(src []byte, start int) int {
	if n := bytes.IndexByte(src[start:], '\n'); n >= 0 {
		return start + n
	}
	return len(src)
}

func lineStart(src []byte, offset int) int {
	for start := offset; start > 0; start-- {
		if src[start-1] == '\n' {
			return start
		}
	}
	return 0
}

func safeLineSuffix(suffix []byte) bool {
	rest := strings.TrimSpace(string(suffix))
	return rest == "" || strings.HasPrefix(rest, "//")
}

func afterBlankLines(src []byte, lineEnd int) int {
	end := lineEnd
	for end < len(src) {
		lineEnd = endOfLine(src, end)
		if !isBlankLine(src[end:lineEnd]) {
			break
		}
		end = lineEnd + 1
		if end > len(src) {
			break
		}
	}
	return end
}

func isBlankLine(line []byte) bool {
	return len(bytes.Trim(line, " \t\r")) == 0
}

type importRef struct {
	name string
	path string
}

func hasCommentsIn(file *fixFile, node ast.Node) bool {
	for _, comment := range file.file.Comments {
		if comment.Pos() < node.Pos() {
			continue
		}
		if comment.End() > node.End() {
			continue
		}
		return true
	}
	return false
}

func endOfLineInclusive(src []byte, offset int) int {
	end := endOfLine(src, offset)
	if end < len(src) {
		return end + 1
	}
	return end
}

func isBlankLineAt(src []byte, offset int) bool {
	if offset >= len(src) {
		return false
	}
	lineEnd := endOfLine(src, offset)
	return isBlankLine(src[offset:lineEnd])
}

func isBlankLineBefore(src []byte, offset int) bool {
	if offset == 0 {
		return false
	}
	return isBlankLineAt(src, lineStart(src, offset-1))
}

func unusedImports(pass *analysis.Pass, used map[importRef]bool, gen *ast.GenDecl) []*ast.ImportSpec {
	var unused []*ast.ImportSpec
	for _, spec := range gen.Specs {
		importSpec, ok := spec.(*ast.ImportSpec)
		if !ok {
			continue
		}
		if isImportUsed(pass, used, importSpec) {
			continue
		}
		unused = append(unused, importSpec)
	}
	return unused
}

func isImportUsed(pass *analysis.Pass, used map[importRef]bool, spec *ast.ImportSpec) bool {
	pkgName := importPkgName(pass, spec)
	if pkgName == nil {
		return false
	}
	if pkgName.Name() == "_" {
		return true
	}
	return used[importRef{name: pkgName.Name(), path: pkgName.Imported().Path()}]
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

func usedImports(pass *analysis.Pass, file *ast.File, sym symbol, whole bool) map[importRef]bool {
	used := make(map[importRef]bool)
	for _, decl := range file.Decls {
		if isImportDecl(decl) {
			continue
		}
		for _, ref := range remainingImports(pass, decl, sym, whole) {
			used[ref] = true
		}
	}
	return used
}

func remainingImports(pass *analysis.Pass, decl ast.Decl, sym symbol, whole bool) []importRef {
	if isRemovedDecl(decl, sym, whole) {
		return nil
	}
	gen, ok := decl.(*ast.GenDecl)
	if !ok {
		return declImports(pass, decl)
	}
	return groupRemainingImports(pass, gen, sym, whole)
}

func isRemovedDecl(decl ast.Decl, sym symbol, whole bool) bool {
	if !whole {
		return false
	}
	if decl == sym.decl {
		return true
	}
	gen, ok := decl.(*ast.GenDecl)
	if !ok {
		return false
	}
	for _, spec := range gen.Specs {
		if specMatches(spec, sym) {
			return true
		}
	}
	return false
}

func groupRemainingImports(pass *analysis.Pass, gen *ast.GenDecl, sym symbol, whole bool) []importRef {
	var refs []importRef
	seen := make(map[importRef]bool)
	for _, spec := range gen.Specs {
		if specMatches(spec, sym) {
			continue
		}
		for _, ref := range declImports(pass, spec) {
			if seen[ref] {
				continue
			}
			seen[ref] = true
			refs = append(refs, ref)
		}
	}
	return refs
}

func isImportDecl(decl ast.Decl) bool {
	gen, ok := decl.(*ast.GenDecl)
	if !ok {
		return false
	}
	return gen.Tok == token.IMPORT
}

func declImports(pass *analysis.Pass, decl ast.Node) []importRef {
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

func (f *fixFile) importDeclRemoval(gen *ast.GenDecl) ([]analysis.TextEdit, bool) {
	start := gen.Pos()
	if gen.Doc != nil {
		start = gen.Doc.Pos()
	}
	end := f.token.Offset(gen.End())
	lineEnd := endOfLine(f.source, end)
	if !safeLineSuffix(f.source[end:lineEnd]) {
		return nil, false
	}
	return []analysis.TextEdit{f.removal(f.token.Offset(start), afterBlankLines(f.source, lineEnd))}, true
}

func (f *fixFile) importRemovals(gen *ast.GenDecl, unused []*ast.ImportSpec) ([]analysis.TextEdit, bool) {
	if len(unused) == len(gen.Specs) {
		return f.importDeclRemoval(gen)
	}
	var edits []analysis.TextEdit
	for _, spec := range unused {
		edit, ok := f.importSpecRemoval(spec)
		if !ok {
			return nil, false
		}
		edits = append(edits, edit)
	}
	return edits, true
}

func (f *fixFile) importRemovalsFor(pass *analysis.Pass, sym symbol, whole bool) ([]analysis.TextEdit, bool) {
	used := usedImports(pass, f.file, sym, whole)
	var edits []analysis.TextEdit
	for _, decl := range f.file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		if gen.Tok != token.IMPORT {
			continue
		}
		unused := unusedImports(pass, used, gen)
		if len(unused) == 0 {
			continue
		}
		if hasCommentsIn(f, gen) {
			return nil, false
		}
		removals, ok := f.importRemovals(gen, unused)
		if !ok {
			return nil, false
		}
		edits = append(edits, removals...)
	}
	return edits, true
}

func (f *fixFile) importSpecRemoval(spec *ast.ImportSpec) (analysis.TextEdit, bool) {
	start := lineStart(f.source, f.token.Offset(spec.Pos()))
	end := f.token.Offset(spec.End())
	lineEnd := endOfLine(f.source, end)
	if !safeLineSuffix(f.source[end:lineEnd]) {
		return analysis.TextEdit{}, false
	}
	if lineEnd == len(f.source) {
		return analysis.TextEdit{}, false
	}
	next := lineEnd + 1
	if isBlankLineAt(f.source, next) {
		return f.removal(start, endOfLineInclusive(f.source, next)), true
	}
	if isBlankLineBefore(f.source, start) {
		start = lineStart(f.source, start-1)
	}
	return f.removal(start, next), true
}

func (f *fixFile) plain() bool {
	base := strings.TrimSuffix(filepath.Base(f.token.Name()), ".go")
	if strings.Contains(base, "_") {
		return false
	}
	return !containsDirective(f.source) && !bytes.Contains(f.source, []byte("// +build"))
}

func (f *fixFile) removal(start, end int) analysis.TextEdit {
	return analysis.TextEdit{Pos: f.token.Pos(start), End: f.token.Pos(end)}
}

func (f *fixFile) removalEdits(pass *analysis.Pass, sym symbol, start, end token.Pos, whole bool) ([]analysis.TextEdit, bool) {
	if !f.startsOwnLine(start) {
		return nil, false
	}
	startOff := f.token.Offset(start)
	endOff := f.token.Offset(end)
	lineEnd := endOfLine(f.source, endOff)
	if !safeLineSuffix(f.source[endOff:lineEnd]) {
		return nil, false
	}
	var edits []analysis.TextEdit
	edits = append(edits, f.removal(startOff, afterBlankLines(f.source, lineEnd)))
	removals, ok := f.importRemovalsFor(pass, sym, whole)
	if !ok {
		return nil, false
	}
	return append(edits, removals...), true
}

func (f *fixFile) startsOwnLine(pos token.Pos) bool {
	off := f.token.Offset(pos)
	start := lineStart(f.source, off)
	return len(bytes.Trim(f.source[start:off], " \t\r")) == 0
}

func (r *reporter) report(pass *analysis.Pass, sym symbol) {
	diag := analysis.Diagnostic{
		Pos:     sym.pos,
		Message: messageFor(sym),
	}
	if !r.offered {
		if fix := suggestedFix(pass, sym); fix != nil {
			diag.SuggestedFixes = []analysis.SuggestedFix{*fix}
			r.offered = true
		}
	}
	pass.Report(diag)
}

type fixFile struct {
	file   *ast.File
	token  *token.File
	source []byte
}
