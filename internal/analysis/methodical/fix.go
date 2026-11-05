package methodical

import (
	"bytes"
	"cmp"
	"go/ast"
	"go/token"
	"go/types"
	"maps"
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
	candidates, imports, unsafe := collectFixMethods(pass, methods, receiver, files, typesByObject)
	return makeSuggestedFix(pass, files, candidates, imports, unsafe)
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
	if err != nil || generated.Skipped(file) || hasStandaloneComments(pass.Fset, file) {
		return nil
	}
	return &fixFile{file: file, token: tf, source: src}
}

func (f *fixFile) importDeclRemoval(gen *ast.GenDecl) (analysis.TextEdit, bool) {
	start := gen.Pos()
	if gen.Doc != nil {
		start = gen.Doc.Pos()
	}
	end := f.token.Offset(gen.End())
	lineEnd := endOfLine(f.source, end)
	if !safeLineSuffix(f.source[end:lineEnd]) {
		return analysis.TextEdit{}, false
	}
	return f.removal(f.token.Offset(start), afterBlankLines(f.source, lineEnd)), true
}

// importRemovals removes whole declarations when they lose every import, so a
// block that no longer holds anything does not stay behind.
func (f *fixFile) importRemovals(gen *ast.GenDecl, unused []*ast.ImportSpec) ([]analysis.TextEdit, bool) {
	if len(unused) == len(gen.Specs) {
		edit, ok := f.importDeclRemoval(gen)
		if !ok {
			return nil, false
		}
		return []analysis.TextEdit{edit}, true
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

// importSpecRemoval takes the blank line next to the import as well, so the
// remaining imports keep the grouping that gofmt leaves in place.
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

// A source file loses the declarations that move out of it, along with any
// import that only they used, so the move edits those imports too.
func (f *fixFile) moveSourceEdits(pass *analysis.Pass, moved []*ast.FuncDecl) ([]analysis.TextEdit, bool) {
	if !f.plain() {
		return nil, false
	}
	if isCgoFile(f.file) {
		return nil, false
	}
	used := usedImports(pass, f.file, moved)
	return f.removeImports(pass, used)
}

// A target file only gains declarations, so it must be one that the move may
// write to at all.
func (f *fixFile) moveTargetSafe() bool {
	if !f.plain() {
		return false
	}
	return !isCgoFile(f.file)
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

func (f *fixFile) removal(start, end int) analysis.TextEdit {
	return analysis.TextEdit{Pos: f.token.Pos(start), End: f.token.Pos(end)}
}

// removeImports deletes the imports that no declaration in the file uses any
// more. It reports false when an import cannot go without taking a comment with
// it or leaving a line that gofmt would rewrite.
func (f *fixFile) removeImports(pass *analysis.Pass, used map[importRef]bool) ([]analysis.TextEdit, bool) {
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

// tailRemoval takes the blank line that precedes removals that reach the end of
// the file, and leaves alone a blank line that has to separate what stays.
func (f *fixFile) tailRemoval(merged []fixRange) (analysis.TextEdit, bool) {
	if len(merged) == 0 {
		return analysis.TextEdit{}, false
	}
	last := merged[len(merged)-1]
	if last.end != len(f.source) {
		return analysis.TextEdit{}, false
	}
	limit := 0
	if len(merged) > 1 {
		limit = merged[len(merged)-2].end
	}
	start := trimBlankTail(f.source, last.start)
	start = max(start, limit)
	if start == last.start {
		return analysis.TextEdit{}, false
	}
	return analysis.TextEdit{Pos: f.token.Pos(start), End: f.token.Pos(last.start)}, true
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

func isBlankLineAt(src []byte, offset int) bool {
	if offset >= len(src) {
		return false
	}
	lineEnd := endOfLine(src, offset)
	return len(bytes.Trim(src[offset:lineEnd], " \t\r")) == 0
}

func isBlankLineBefore(src []byte, offset int) bool {
	if offset == 0 {
		return false
	}
	return isBlankLineAt(src, lineStart(src, offset-1))
}

func endOfLineInclusive(src []byte, offset int) int {
	end := endOfLine(src, offset)
	if end < len(src) {
		return end + 1
	}
	return end
}

// importRef names an import the way the code it serves names it.
type importRef struct {
	name string
	path string
}

// declImports lists the imports that a declaration needs to compile in a
// different file. A qualified identifier needs the name that qualifies it, and
// a bare identifier from another package needs a dot import.
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

// usedImports lists the imports that the declarations which stay in the file
// still reference. The moved declarations leave the file, so their uses do not
// keep an import alive.
func usedImports(pass *analysis.Pass, file *ast.File, moved []*ast.FuncDecl) map[importRef]bool {
	skipped := make(map[*ast.FuncDecl]bool, len(moved))
	for _, decl := range moved {
		skipped[decl] = true
	}
	used := make(map[importRef]bool)
	for _, decl := range file.Decls {
		if isMovedDecl(decl, skipped) {
			continue
		}
		for _, ref := range declImports(pass, decl) {
			used[ref] = true
		}
	}
	return used
}

func isMovedDecl(decl ast.Decl, skipped map[*ast.FuncDecl]bool) bool {
	funcDecl, ok := decl.(*ast.FuncDecl)
	if !ok {
		return false
	}
	return skipped[funcDecl]
}

func isImportUsed(pass *analysis.Pass, used map[importRef]bool, spec *ast.ImportSpec) bool {
	pkgName := importPkgName(pass, spec)
	if pkgName == nil {
		return false
	}
	// A blank import only runs package initialization, so a file cannot
	// orphan it.
	if isBlankName(pkgName.Name()) {
		return true
	}
	return used[importRef{name: pkgName.Name(), path: pkgName.Imported().Path()}]
}

func isBlankName(name string) bool {
	return name == "_"
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

func collectFixMethods(pass *analysis.Pass, methods []methodInfo, receiver types.Object, files map[string]*fixFile, typesByObject map[types.Object]fixType) ([]fixMethod, []analysis.TextEdit, map[types.Object]bool) {
	var candidates []fixMethod
	var importEdits []analysis.TextEdit
	unsafe := make(map[types.Object]bool)
	moved := make(map[*fixFile][]*ast.FuncDecl)
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
		if candidate.file != candidate.target.file {
			moved[candidate.file] = append(moved[candidate.file], candidate.method.decl)
		}
	}
	// A file can lose an import when several declarations leave it at once, so
	// judge each source file by everything that leaves it.
	for file, decls := range moved {
		removals, ok := file.moveSourceEdits(pass, decls)
		if !ok {
			unsafe[receiver] = true
			continue
		}
		importEdits = append(importEdits, removals...)
	}
	added, ok := collectImportEdits(pass, candidates)
	if !ok {
		unsafe[receiver] = true
	}
	importEdits = append(importEdits, added...)
	return candidates, importEdits, unsafe
}

// collectImportEdits adds the imports that the moved declarations need and the
// target files lack. It reports false when an import cannot be added safely.
func collectImportEdits(pass *analysis.Pass, candidates []fixMethod) ([]analysis.TextEdit, bool) {
	needed := make(map[*fixFile][]importRef)
	for _, m := range candidates {
		if m.file == m.target.file {
			continue
		}
		needed[m.target.file] = append(needed[m.target.file], declImports(pass, m.method.decl)...)
	}
	var edits []analysis.TextEdit
	for file, refs := range needed {
		fileEdits, ok := fileImportEdits(pass, file, refs)
		if !ok {
			return nil, false
		}
		edits = append(edits, fileEdits...)
	}
	return edits, true
}

func fileImportEdits(pass *analysis.Pass, file *fixFile, refs []importRef) ([]analysis.TextEdit, bool) {
	missing := missingImports(pass, file, refs)
	if len(missing) == 0 {
		return nil, true
	}
	if !canAddImports(pass, file, missing) {
		return nil, false
	}
	return addImportEdits(pass, file, missing), true
}

func missingImports(pass *analysis.Pass, file *fixFile, refs []importRef) []importRef {
	have := fileImports(pass, file.file)
	missing := make(map[importRef]bool)
	for _, ref := range refs {
		if hasImport(have, ref) {
			continue
		}
		missing[ref] = true
	}
	return slices.SortedFunc(maps.Keys(missing), compareImportRefs)
}

func compareImportRefs(a, b importRef) int {
	return cmp.Or(strings.Compare(a.path, b.path), strings.Compare(a.name, b.name))
}

// canAddImports reports whether the imports can join the file without changing
// what the names in that file mean. A name that the file already binds cannot
// name a second package, and a comment inside the import block would lose the
// import that it describes.
func canAddImports(pass *analysis.Pass, file *fixFile, missing []importRef) bool {
	have := fileImports(pass, file.file)
	for _, ref := range missing {
		if isNameTaken(have, ref.name) {
			return false
		}
	}
	block := importBlock(file.file)
	if block == nil {
		return true
	}
	return !hasCommentsIn(file, block)
}

// isNameTaken reports whether the file already binds the name to another
// package. Go forbids one name from naming both an import and a package-level
// declaration, so a name that the moved code already uses cannot clash with a
// package declaration, and only the names in the file need the check.
func isNameTaken(have map[string]string, name string) bool {
	_, ok := have[name]
	return ok
}

func importBlock(file *ast.File) *ast.GenDecl {
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		if gen.Tok != token.IMPORT {
			continue
		}
		if gen.Lparen.IsValid() {
			return gen
		}
	}
	return nil
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

func addImportEdits(pass *analysis.Pass, file *fixFile, missing []importRef) []analysis.TextEdit {
	block := importBlock(file.file)
	if block != nil {
		return blockImportEdits(pass, file, block, missing)
	}
	lines := standaloneImportLines(file)
	if len(lines) != 0 {
		return standaloneImportEdits(pass, file, lines, missing)
	}
	anchor := endOfLine(file.source, file.token.Offset(file.file.Name.End()))
	pos := file.token.Pos(anchor)
	return []analysis.TextEdit{{Pos: pos, End: pos, NewText: newImportDecl(pass, missing)}}
}

// newImportDecl writes imports for a file that has none, with the blank lines
// that separate a standalone declaration from the code around it.
func newImportDecl(pass *analysis.Pass, missing []importRef) []byte {
	if len(missing) == 1 {
		return []byte("\n\nimport " + importSpecText(pass, missing[0]))
	}
	var decl bytes.Buffer
	decl.WriteString("\n\nimport (\n")
	for _, ref := range missing {
		decl.WriteString("\t" + importSpecText(pass, ref) + "\n")
	}
	decl.WriteString(")")
	return decl.Bytes()
}

func standaloneImportLines(file *fixFile) []importLine {
	var lines []importLine
	for _, decl := range file.file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		if gen.Tok != token.IMPORT {
			continue
		}
		if gen.Lparen.IsValid() {
			continue
		}
		for _, spec := range gen.Specs {
			importSpec, ok := spec.(*ast.ImportSpec)
			if !ok {
				continue
			}
			lines = append(lines, importLine{spec: importSpec, path: importPath(importSpec)})
		}
	}
	return lines
}

// standaloneImportEdits keeps the imports of a file that spells each one as its
// own declaration together, instead of starting a second group of them.
func standaloneImportEdits(pass *analysis.Pass, file *fixFile, lines []importLine, missing []importRef) []analysis.TextEdit {
	after := make(map[int][]string)
	first := make(map[int][]string)
	for _, ref := range missing {
		line, ok := importNeighbour(lines, ref.path)
		if !ok {
			anchor := lineStart(file.source, file.token.Offset(lines[0].spec.Pos()))
			first[anchor] = append(first[anchor], "import "+importSpecText(pass, ref))
			continue
		}
		anchor := endOfLine(file.source, file.token.Offset(line.spec.End()))
		after[anchor] = append(after[anchor], "\nimport "+importSpecText(pass, ref))
	}
	return joinImportEdits(file, after, first)
}

// importSpecText writes one import spec the way the moved code names it.
func importSpecText(pass *analysis.Pass, ref importRef) string {
	quoted := strconv.Quote(ref.path)
	if ref.name == "." {
		return ". " + quoted
	}
	if ref.name == importedName(pass, ref.path) {
		return quoted
	}
	return ref.name + " " + quoted
}

func importedName(pass *analysis.Pass, path string) string {
	for _, pkg := range pass.Pkg.Imports() {
		if pkg.Path() == path {
			return pkg.Name()
		}
	}
	return ""
}

type importLine struct {
	spec *ast.ImportSpec
	path string
}

// blockImportEdits places each missing import right after the import that
// sorts before it, which is where gofmt leaves it. An import that sorts first
// joins the line of the import that follows it.
func blockImportEdits(pass *analysis.Pass, file *fixFile, block *ast.GenDecl, missing []importRef) []analysis.TextEdit {
	lines := blockImportLines(file, block)
	indent := specIndent(file, lines)
	after := make(map[int][]string)
	first := make(map[int][]string)
	for _, ref := range missing {
		line, ok := importNeighbour(lines, ref.path)
		if !ok {
			anchor := firstLineStart(file, block)
			first[anchor] = append(first[anchor], indent+importSpecText(pass, ref))
			continue
		}
		anchor := endOfLine(file.source, file.token.Offset(line.spec.End()))
		after[anchor] = append(after[anchor], "\n"+indent+importSpecText(pass, ref))
	}
	return joinImportEdits(file, after, first)
}

func joinImportEdits(file *fixFile, after, first map[int][]string) []analysis.TextEdit {
	var edits []analysis.TextEdit
	for anchor, specs := range after {
		pos := file.token.Pos(anchor)
		edits = append(edits, analysis.TextEdit{Pos: pos, End: pos, NewText: []byte(strings.Join(specs, ""))})
	}
	for anchor, specs := range first {
		pos := file.token.Pos(anchor)
		edits = append(edits, analysis.TextEdit{Pos: pos, End: pos, NewText: []byte(strings.Join(specs, "\n") + "\n")})
	}
	return edits
}

func blockImportLines(file *fixFile, block *ast.GenDecl) []importLine {
	lines := make([]importLine, 0, len(block.Specs))
	for _, spec := range block.Specs {
		importSpec, ok := spec.(*ast.ImportSpec)
		if !ok {
			continue
		}
		lines = append(lines, importLine{spec: importSpec, path: importPath(importSpec)})
	}
	return lines
}

func importPath(spec *ast.ImportSpec) string {
	path, err := strconv.Unquote(spec.Path.Value)
	if err != nil {
		return spec.Path.Value
	}
	return path
}

// importNeighbour finds the import that a new path follows in sort order.
func importNeighbour(lines []importLine, path string) (importLine, bool) {
	var neighbour importLine
	found := false
	for _, line := range lines {
		if !isBefore(line.path, path) {
			continue
		}
		if found && !isBefore(neighbour.path, line.path) {
			continue
		}
		neighbour, found = line, true
	}
	return neighbour, found
}

func firstLineStart(file *fixFile, block *ast.GenDecl) int {
	if len(block.Specs) == 0 {
		return endOfLine(file.source, file.token.Offset(block.Lparen))
	}
	return lineStart(file.source, file.token.Offset(block.Specs[0].Pos()))
}

func lineStart(src []byte, offset int) int {
	start := bytes.LastIndexByte(src[:offset], '\n')
	return start + 1
}

func specIndent(file *fixFile, lines []importLine) string {
	if len(lines) == 0 {
		return "\t"
	}
	pos := file.token.Offset(lines[0].spec.Pos())
	return string(file.source[lineStart(file.source, pos):pos])
}

func fixMethodFor(pass *analysis.Pass, m methodInfo, srcFile *fixFile, target fixType) (fixMethod, bool) {
	if srcFile == nil {
		return fixMethod{}, false
	}
	if target.file == nil {
		return fixMethod{}, false
	}
	if srcFile != target.file {
		if !target.file.moveTargetSafe() {
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

func makeSuggestedFix(pass *analysis.Pass, files map[string]*fixFile, candidates []fixMethod, imports []analysis.TextEdit, unsafe map[types.Object]bool) *analysis.SuggestedFix {
	groups := make(map[token.Pos][]fixMethod)
	for _, m := range candidates {
		if !unsafe[m.method.typeObj] {
			groups[m.target.anchor] = append(groups[m.target.anchor], m)
		}
	}
	var edits []analysis.TextEdit
	for anchor, group := range groups {
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
	edits = append(edits, imports...)
	return &analysis.SuggestedFix{Message: "Move and order methods after their types", TextEdits: trimRemovedTails(pass.Fset, files, edits)}
}

// fixRange is a span of a source file that a fix removes.
type fixRange struct {
	start, end int
}

// trimRemovedTails keeps a fix from ending a file with a blank line. Text that
// a fix removes keeps the blank line that separated it from the declaration
// above, so when the removals reach the end of a file, that blank line goes
// too.
func trimRemovedTails(fset *token.FileSet, files map[string]*fixFile, edits []analysis.TextEdit) []analysis.TextEdit {
	removals := make(map[*fixFile][]fixRange)
	kept := make([]analysis.TextEdit, 0, len(edits))
	for _, edit := range edits {
		fix := editFile(fset, files, edit)
		if edit.End == edit.Pos {
			kept = append(kept, edit)
			continue
		}
		if fix == nil {
			kept = append(kept, edit)
			continue
		}
		removals[fix] = append(removals[fix], fixRange{fix.token.Offset(edit.Pos), fix.token.Offset(edit.End)})
	}
	for fix, ranges := range removals {
		merged := mergeRanges(ranges)
		tail, ok := fix.tailRemoval(merged)
		if ok {
			kept = append(kept, tail)
		}
		for _, r := range merged {
			kept = append(kept, analysis.TextEdit{Pos: fix.token.Pos(r.start), End: fix.token.Pos(r.end)})
		}
	}
	return kept
}

func editFile(fset *token.FileSet, files map[string]*fixFile, edit analysis.TextEdit) *fixFile {
	file := fset.File(edit.Pos)
	if file == nil {
		return nil
	}
	if fset.File(edit.End) != file {
		return nil
	}
	return files[file.Name()]
}

func mergeRanges(ranges []fixRange) []fixRange {
	if len(ranges) == 0 {
		return nil
	}
	slices.SortFunc(ranges, func(a, b fixRange) int {
		return cmp.Compare(a.start, b.start)
	})
	merged := ranges[:1]
	for _, r := range ranges[1:] {
		last := &merged[len(merged)-1]
		if r.start <= last.end {
			last.end = max(last.end, r.end)
			continue
		}
		merged = append(merged, r)
	}
	return merged
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
