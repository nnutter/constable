package methodical

import (
	"cmp"
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"strings"

	"golang.org/x/tools/go/analysis"

	"github.com/nnutter/constable/internal/report"
)

var Analyzer = &analysis.Analyzer{
	Name: "methodical",
	Doc:  "reports methods not in same file as type, not grouped together, or not sorted alphabetically",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	typeFiles := collectTypeFiles(pass)
	methods := collectMethods(pass, typeFiles)
	reporter := &diagnosticReporter{pass: pass, methods: methods}
	for _, m := range methods {
		checkSameFile(reporter, typeFiles, m)
	}
	checkSorted(reporter, methods)
	checkGrouped(reporter, typeFiles)

	return nil, nil
}

func succeeded(ok bool) bool {
	return ok
}

func isNilTypeName(t *types.TypeName) bool {
	return t == nil
}

func isNilRecv(recv *ast.FieldList) bool {
	return recv == nil
}

func isNonEmpty(s string) bool {
	return s != ""
}

func isBefore(a, b string) bool {
	return strings.Compare(a, b) < 0
}

func isNilFunc(f *types.Func) bool {
	return f == nil
}

func isNilVar(v *types.Var) bool {
	return v == nil
}

func collectTypeFiles(pass *analysis.Pass) map[types.Object]string {
	typeFiles := make(map[types.Object]string)
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			genDecl, ok := decl.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range genDecl.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				obj, ok := pass.TypesInfo.Defs[typeSpec.Name].(*types.TypeName)
				if !succeeded(ok) || isNilTypeName(obj) {
					continue
				}
				typeFiles[obj] = pass.Fset.Position(typeSpec.Pos()).Filename
			}
		}
	}

	return typeFiles
}

type methodInfo struct {
	typeObj    types.Object
	typeName   string
	methodName string
	file       string
	pos        token.Pos
	decl       *ast.FuncDecl
}

type groupKey struct {
	typeObj types.Object
	file    string
}

func collectMethods(pass *analysis.Pass, typeFiles map[types.Object]string) []methodInfo {
	var methods []methodInfo
	for _, file := range pass.Files {
		for _, decl := range file.Decls {
			funcDecl, ok := decl.(*ast.FuncDecl)
			if !succeeded(ok) || isNilRecv(funcDecl.Recv) {
				continue
			}
			typeObj, methodName := receiverType(pass, funcDecl)
			if typeObj == nil {
				continue
			}
			if _, ok := typeFiles[typeObj]; !ok {
				continue
			}
			methods = append(methods, methodInfo{
				typeObj:    typeObj,
				typeName:   typeObj.Name(),
				methodName: methodName,
				file:       pass.Fset.Position(funcDecl.Name.Pos()).Filename,
				pos:        funcDecl.Name.Pos(),
				decl:       funcDecl,
			})
		}
	}

	return methods
}

func checkSameFile(reporter *diagnosticReporter, typeFiles map[types.Object]string, m methodInfo) {
	typeFile := typeFiles[m.typeObj]
	if m.file == typeFile {
		return
	}

	reporter.report(analysis.Diagnostic{
		Pos:     m.decl.Name.Pos(),
		Message: report.MethodShouldBeInSameFile(m.typeName, m.methodName),
	}, m.typeObj)
}

func checkSorted(reporter *diagnosticReporter, methods []methodInfo) {
	groups := make(map[groupKey][]methodInfo)
	for _, m := range methods {
		key := groupKey{typeObj: m.typeObj, file: m.file}
		groups[key] = append(groups[key], m)
	}

	keys := make([]groupKey, 0, len(groups))
	for key := range groups {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b groupKey) int {
		return cmp.Or(
			strings.Compare(a.file, b.file),
			strings.Compare(a.typeObj.Name(), b.typeObj.Name()),
		)
	})

	for _, key := range keys {
		checkGroupSorted(reporter, groups[key])
	}
}

func isGenericAfterRegular(generic, seenRegular bool) bool {
	if !generic {
		return false
	}
	return seenRegular
}

func checkGroupSorted(reporter *diagnosticReporter, group []methodInfo) {
	slices.SortFunc(group, func(a, b methodInfo) int {
		return cmp.Compare(a.pos, b.pos)
	})

	var maxGeneric, maxRegular string
	seenRegular := false
	for _, m := range group {
		generic := m.decl.Type.TypeParams != nil
		max := maxRegular
		if generic {
			max = maxGeneric
		}
		if isGenericAfterRegular(generic, seenRegular) {
			passName := maxRegular
			reporter.report(analysis.Diagnostic{
				Pos:     m.decl.Name.Pos(),
				Message: report.MethodShouldBeSorted(m.typeName, m.methodName, passName),
			}, m.typeObj)
		} else if isNonEmpty(max) && isBefore(m.methodName, max) {
			reporter.report(analysis.Diagnostic{
				Pos:     m.decl.Name.Pos(),
				Message: report.MethodShouldBeSorted(m.typeName, m.methodName, max),
			}, m.typeObj)
		}
		if generic {
			if m.methodName > maxGeneric {
				maxGeneric = m.methodName
			}
		} else {
			seenRegular = true
			if m.methodName > maxRegular {
				maxRegular = m.methodName
			}
		}
	}
}

type groupState struct {
	lastName    string
	interrupted bool
}

func isInterrupted(state *groupState) bool {
	return state.interrupted
}

func isKnownReceiver(typeObj types.Object, typeFiles map[types.Object]string) bool {
	_, ok := typeFiles[typeObj]
	return ok
}

func isOtherType(obj, current types.Object) bool {
	return obj != current
}

func checkGrouped(reporter *diagnosticReporter, typeFiles map[types.Object]string) {
	for _, file := range reporter.pass.Files {
		checkFileGrouped(reporter, file, typeFiles)
	}
}

func checkFileGrouped(reporter *diagnosticReporter, file *ast.File, typeFiles map[types.Object]string) {
	states := make(map[types.Object]*groupState)
	for _, decl := range file.Decls {
		funcDecl, ok := decl.(*ast.FuncDecl)
		if !succeeded(ok) {
			continue
		}
		typeObj, methodName := receiverType(reporter.pass, funcDecl)
		if typeObj == nil {
			interruptAllGroups(states)
			continue
		}
		if !isKnownReceiver(typeObj, typeFiles) {
			interruptAllGroups(states)
			continue
		}
		state := states[typeObj]
		if state != nil && isInterrupted(state) {
			reporter.report(analysis.Diagnostic{
				Pos:     funcDecl.Name.Pos(),
				Message: report.MethodShouldBeGrouped(typeObj.Name(), methodName, state.lastName),
			}, typeObj)
		}
		if state == nil {
			state = &groupState{}
			states[typeObj] = state
		}
		state.lastName = methodName
		state.interrupted = false
		interruptOtherGroups(states, typeObj)
	}
}

func interruptAllGroups(states map[types.Object]*groupState) {
	for _, state := range states {
		state.interrupted = true
	}
}

func interruptOtherGroups(states map[types.Object]*groupState, current types.Object) {
	for obj, state := range states {
		if isOtherType(obj, current) {
			state.interrupted = true
		}
	}
}

func receiverType(pass *analysis.Pass, funcDecl *ast.FuncDecl) (types.Object, string) {
	funcObj, ok := pass.TypesInfo.Defs[funcDecl.Name].(*types.Func)
	if !succeeded(ok) || isNilFunc(funcObj) {
		return nil, ""
	}
	sig, ok := funcObj.Type().(*types.Signature)
	if !succeeded(ok) || isNilVar(sig.Recv()) {
		return nil, ""
	}
	recvType := sig.Recv().Type()
	if ptr, ok := recvType.(*types.Pointer); ok {
		recvType = ptr.Elem()
	}
	named, ok := recvType.(*types.Named)
	if !ok {
		return nil, ""
	}
	if named.Obj() == nil {
		return nil, ""
	}

	return named.Obj(), funcObj.Name()
}
