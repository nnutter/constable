# Constable Plan

Constable is a static analysis tool for Go.
Its first goal is to provide opt-in semantic checks that help developers state and enforce code contracts that Go does not currently express in the type system.
The long-term goal is for these checks to be usable from the command line, test suites, CI, and `gopls`.

The initial check verifies a function is nonmutating,

```go
//constable:nonmutating
func Example(arg *Config) Result {
	// The function must not mutate caller-visible state through its arguments.
}
```

For this project, "nonmutating" means that a function does not mutate any arguments from the caller's perspective.
That includes the receiver for receiver functions (methods).
The name, `nonmutating`, is inspired by the `mutating` keyword in Swift; see [Modifying Value Types from Within Instance Methods](https://docs.swift.org/swift-book/documentation/the-swift-programming-language/methods/#Modifying-Value-Types-from-Within-Instance-Methods).
The definition of "nonmutating" does not imply more general immutability nor does it imply lack of side effects.

## Goals

- Build analyzers on top of `golang.org/x/tools/go/analysis`.
- Keep checks independently usable, testable, and composable.
- Design analyzer output so it can be consumed by `gopls` diagnostics.
- Start with one focused check: `//constable:nonmutating` on functions and methods.
- Use Go SSA where it provides better semantic information than AST-only inspection.
- Prefer precise, explainable diagnostics over broad but noisy warnings.

## Non-Goals For The Initial Version

- Proving broader side-effect freedom beyond caller-visible argument mutation.
- Detecting every possible mutation through aliases.
- Enforcing nonmutation across goroutine, reflection, unsafe, cgo, or assembly boundaries with complete precision.

## Architecture

The project should follow the shape used by common Go analysis tools such as Staticcheck: small analyzers, a shared internal framework where needed, and thin entry points for command-line or editor integration.

Proposed package layout:

```text
cmd/constable/
    Main command-line entry point.

internal/analysis/nonmutating/
    The initial //constable:nonmutating analyzer.

internal/directive/
    Parsing and validation for //constable: directives.

internal/report/
    Shared diagnostic helpers if multiple analyzers later need them.

internal/testdata/
    Analyzer test fixtures following go/analysis analysistest conventions.
```

The first implementation can be smaller than this layout if some packages are not yet useful.
The important boundary is that analyzer logic should not be coupled to the command-line entry point.

## Analyzer Framework

Each check should be implemented as a `*analysis.Analyzer`.
The command-line tool can use `singlechecker` while there is only one analyzer and move to `multichecker` when more checks are added.

Expected flow:

1. Load packages through the standard analysis driver.
2. Parse `//constable:` directives from function and method declarations.
3. For each recognized directive, run the corresponding analyzer logic.
4. Emit diagnostics through `analysis.Pass.Report` with stable, actionable messages.

The analyzer should avoid defining its own package loading, type checking, or diagnostic protocol unless the standard `go/analysis` path proves insufficient.

## Directive Syntax

Directive comments use the `constable` namespace:

```go
//constable:nonmutating
func F() {}
```

Initial parsing rules:

- A directive applies to the immediately following function or method declaration.
- A directive begins with `//constable:`.
- Use Go struct tag syntax after `//constable:` for directive options; see [reflect.StructTag](https://pkg.go.dev/reflect#StructTag).
  The sturct tag key refers to the check name, e.g., `nonmutating`.
  The value can be parsed specific to the check's options (if any) but typically is a simple comma-separated list of options.
  Ideally, we should tolerate omitting the `:""` part of the struct tag if there are no options.
- The first directive name is `nonmutating`.
- Unknown directives should produce a diagnostic once directive parsing exists.

The proposed syntax leaves room for new, future checks:

```go
//constable:nonmutating othercheck:"safe,strict"
```

## Initial Check: `//constable:nonmutating`

The `nonmutating` analyzer verifies that annotated functions do not mutate state that is visible to the caller through parameters or method receivers.

Examples that should be reported:

```go
//constable:nonmutating
func MutatesPointer(p *int) {
	*p = 1
}

//constable:nonmutating
func MutatesSlice(s []int) {
	s[0] = 1
}

//constable:nonmutating
func MutatesMap(m map[string]int) {
	m["x"] = 1
}

type Counter struct{ n int }

//constable:nonmutating
func (c *Counter) Inc() {
	c.n++
}

type Buffer struct{ data []byte }

//constable:nonmutating
func (b Buffer) ClearFirstByte() {
	b.data[0] = 0
}
```

Examples that should be allowed:

```go
//constable:nonmutating
func ReadsPointer(p *int) int {
	return *p
}

//constable:nonmutating
func MutatesLocal() int {
	x := 0
	x++
	return x
}

//constable:nonmutating
func ReassignsParameter(p *int) {
	p = nil // Does not mutate the caller's pointed-to value.
}

type Count struct{ n int }

//constable:nonmutating
func (c Count) Increment() Count {
	c.n++ // Mutates only the receiver copy.
	return c
}
```

The check should initially focus on direct caller-visible mutations:

- Assigning through a pointer parameter or pointer receiver.
- Assigning to fields reachable through a pointer parameter or pointer receiver.
- Assigning to slice elements reachable from a parameter.
- Assigning to map entries reachable from a parameter.
- Calling built-ins that mutate parameter-backed values, such as `delete` on a map parameter.
- Passing parameter-reachable mutable state to known-mutating operations when they can be identified with confidence.

The check should be conservative about uncertain cases.
Depending on the final policy, uncertain cases may either be reported as unsupported or ignored until a more precise model exists.

## SSA Strategy

SSA is likely the right representation for the core mutation analysis because it makes data flow and memory operations more explicit than raw AST traversal.
`golang.org/x/tools/go/analysis/passes/buildssa` can provide SSA for the current package to analyzers.

The `nonmutating` analyzer should depend on `buildssa.Analyzer` and inspect the SSA function corresponding to each annotated declaration.

Likely implementation outline:

1. Find annotated `*ast.FuncDecl` nodes.
2. Resolve each declaration to its `*types.Func` object.
3. Find the corresponding `*ssa.Function` from the `buildssa` result.
4. Mark SSA values that originate from parameters and receivers.
5. Track aliases derived from those values where practical.
6. Report stores, map updates, field updates, slice element updates, and relevant built-ins when their target is parameter-derived caller-visible state.

SSA concepts likely to matter:

- `*ssa.Parameter` identifies function parameters and receivers.
- `*ssa.Store` represents memory writes.
- `*ssa.FieldAddr`, `*ssa.IndexAddr`, and related address-producing instructions can identify writes into fields, arrays, and slices.
- `*ssa.MapUpdate` represents map entry mutation.
- `*ssa.Call` may need summary information to understand whether a callee can mutate its arguments.
- `*ssa.Alloc` helps distinguish local allocations from caller-provided memory.

The first version should not attempt to reproduce Go's escape analysis.
Escape analysis answers allocation-placement questions; this analyzer answers whether caller-visible state is mutated.
Escape analysis and SSA alias information can inspire the implementation, but the check should remain independent and explainable.

## Interprocedural Analysis

The initial version can be intraprocedural with limited call handling.
That means it analyzes the body of the annotated function directly and reports obvious mutations in that body.

Future versions can add summaries for functions:

- Which parameters may be mutated.
- Whether a function is annotated `//constable:nonmutating` and already checked.
- Whether a known standard-library function mutates an argument.
- Whether calls through interfaces or function values are unknown.

This summary model will matter for code such as:

```go
//constable:nonmutating
func F(p *int) {
	G(p)
}

func G(p *int) {
	*p = 1
}
```

The first version should decide whether to ignore this, report it as an unknown mutation risk, or only report it when `G` is available and analyzable.

## Diagnostics

Diagnostics should be specific and located at the mutation site when possible.

Example messages:

- `//constable:nonmutating function mutates pointer parameter p`
- `//constable:nonmutating method mutates receiver c`
- `//constable:nonmutating function mutates map parameter m`
- `//constable:nonmutating function deletes from map parameter m`

Diagnostics should avoid claiming more than the analyzer has proven.
If a case is uncertain, the message should say so explicitly or the analyzer should remain silent until the policy is clear.

## Testing Strategy

Use `golang.org/x/tools/go/analysis/analysistest` for analyzer tests.
Test data should include positive and negative examples with `// want` comments.

Initial test categories:

- Directive parsing and attachment to function declarations.
- Direct pointer mutation.
- Receiver mutation.
- Struct field mutation through pointer parameters.
- Slice element mutation.
- Map update and delete.
- Local-only mutation that should not be reported.
- Parameter reassignment that should not be reported.
- Unsupported or uncertain cases, once policy is decided.

As the analyzer grows, tests should separate behavior-preserving refactors from new detection behavior so review stays manageable.

## Command-Line Tool

The first command should run the analyzer over packages in the same style as other Go analysis tools:

```sh
constable ./...
```

While there is only one analyzer, `singlechecker.Main(nonmutating.Analyzer)` is enough.
When additional checks exist, switch to `multichecker.Main(...)`.

The command-line tool should be thin.
Analyzer configuration, directive parsing, and diagnostic behavior should live outside `cmd/`.

## Future `gopls` Integration

`gopls` can consume analyzers built on `go/analysis`, which is the main reason to use that package from the beginning.
To keep that path open:

- Keep analyzers deterministic and package-local unless explicitly designed otherwise.
- Avoid depending on command-line-only behavior.
- Keep diagnostics stable and precise.
- Avoid expensive whole-program work in the default analyzer path.
- Make any future configuration explicit and serializable.

The first integration milestone should be a normal analyzer that can run under standard Go analysis drivers.
Direct `gopls` integration can wait until the check is useful and well tested.

## Open Questions

These questions should be resolved incrementally as implementation begins:

1. Should `//constable:` options use struct-tag syntax, Go directive-style syntax, or a simpler custom syntax?
2. Should unknown `//constable:` directives be diagnostics, ignored comments, or reserved for future tools?
3. Should `//constable:nonmutating` forbid mutation of package globals, or only mutation of caller-visible argument and receiver state?
4. How should calls to unanalyzed functions be handled: ignored, reported as uncertain, or controlled by a strict mode?
5. Should calls to methods on parameter-derived values be assumed mutating when the receiver is mutable?
6. How should interface calls and function values be modeled?
7. Should mutation through `unsafe`, reflection, cgo, or assembly be reported as unsupported in annotated functions?
8. Should appending to a slice parameter be considered mutation only when it writes to the existing backing array, or always treated as caller-visible?
9. Should the analyzer permit mutations to data reachable from parameters if the function can prove the data was freshly allocated inside the function?
10. The project name and module path are `constable` and `github.com/nnutter/constable`.

## First Implementation Milestone

The first milestone is a minimal but real analyzer:

1. Create a Go module.
2. Add a `nonmutating` analyzer using `go/analysis` and `buildssa`.
3. Parse `//constable:nonmutating` comments on function and method declarations.
4. Detect direct pointer, receiver, slice, and map mutations in annotated functions.
5. Add analyzer tests for the supported cases and explicit non-cases.
6. Add a thin command-line entry point using `singlechecker`.

This milestone should prioritize a small, reviewable implementation over a broad nonmutation model.
