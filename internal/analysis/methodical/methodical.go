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
	Doc:  "reports methods not in same file as type or not sorted alphabetically",
	Run:  run,
}

func run(pass *analysis.Pass) (any, error) {
	typeFiles := collectTypeFiles(pass)
	methods := collectMethods(pass, typeFiles)
	for _, m := range methods {
		checkSameFile(pass, typeFiles, m)
	}
	checkSorted(pass, methods)

	return nil, nil
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
				if !ok || obj == nil {
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
			if !ok || funcDecl.Recv == nil {
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

func checkSameFile(pass *analysis.Pass, typeFiles map[types.Object]string, m methodInfo) {
	typeFile := typeFiles[m.typeObj]
	if m.file == typeFile {
		return
	}

	pass.Report(analysis.Diagnostic{
		Pos:     m.decl.Name.Pos(),
		Message: report.MethodShouldBeInSameFile(m.typeName, m.methodName),
	})
}

func checkSorted(pass *analysis.Pass, methods []methodInfo) {
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
		group := groups[key]
		slices.SortFunc(group, func(a, b methodInfo) int {
			return cmp.Compare(a.pos, b.pos)
		})

		max := ""
		for _, m := range group {
			if max != "" && strings.Compare(m.methodName, max) < 0 {
				pass.Report(analysis.Diagnostic{
					Pos:     m.decl.Name.Pos(),
					Message: report.MethodShouldBeSorted(m.typeName, m.methodName, max),
				})
				continue
			}
			if strings.Compare(m.methodName, max) > 0 {
				max = m.methodName
			}
		}
	}
}

func receiverType(pass *analysis.Pass, funcDecl *ast.FuncDecl) (types.Object, string) {
	funcObj, ok := pass.TypesInfo.Defs[funcDecl.Name].(*types.Func)
	if !ok || funcObj == nil {
		return nil, ""
	}
	sig, ok := funcObj.Type().(*types.Signature)
	if !ok || sig.Recv() == nil {
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
