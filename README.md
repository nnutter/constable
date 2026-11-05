# constable

constable is a static analysis tool for Go.

The first analyzer checks functions annotated with `//constable:nonmutating`.
An annotated function must not mutate caller-visible state through parameters or method receivers.

## Installation

Standard `go install` installation,

```sh
go install github.com/nnutter/constable/cmd/constable@latest
```

## Usage

Run `constable` on your Go packages,

```sh
constable ./...
```

constable skips files that carry the standard generated-code comment, `// Code generated ... DO NOT EDIT.`.
A diagnostic in such a file is not actionable because the next run of the generator overwrites the file.

## Example

This function is reported because it mutates through a pointer parameter,

```go
//constable:nonmutating
func F(p *int) {
	*p = 1
}
```

This function is allowed because it only reads through the pointer parameter,

```go
//constable:nonmutating
func F(p *int) int {
	return *p
}
```

This method is allowed because a scalar field assignment on a value receiver mutates only the receiver copy,

```go
type Count struct{ n int }

//constable:nonmutating
func (c Count) Increment() Count {
	c.n++
	return c
}
```

This method is reported because the slice field is mutated for the caller,

```go
type Buffer struct{ data []byte }

//constable:nonmutating
func (b Buffer) ClearFirstByte() {
	b.data[0] = 0
}
```

## Complexity

The complexity analyzer reports functions and methods whose cyclomatic complexity exceeds a limit.
Complexity starts at 1 and adds 1 for each `if`, `for`, `range`, `case`, `select` clause, `&&`, and `||`.
Closures fold into the enclosing function and default clauses count.

```sh
constable ./...
constable -complexity.limit=15 ./...
```

The limit defaults to 10 and is configured with `-complexity.limit`.

## Predicates

The predicates analyzer reports `&&` and `||` operands, except the first, that are not predicate-function calls.
The first operand always executes, so coverage observes short-circuiting through the rest.

```go
if a && b { // reported: use a && isB()
}

if a && isB() { // allowed
}
```

The check also covers `for` conditions and bool assignments such as `c := a && b`, including negated variables, field and map access, and comparisons.

## Fixes

`-fix` applies the fixes that the analyzers suggest, and `-fix -diff` prints the change instead of writing it.

```sh
constable -fix ./...
constable -fix -diff ./...
```

Only the methodical analyzer suggests fixes today.
It places every method of a reported type directly after the type declaration, in order, and a method that lives in another file moves into the file that declares its type.

A move fixes the import blocks of both files:

- The new file gains the imports that the moved method needs.
- The old file loses the imports that no declaration uses any more.

A move is conservative, because the analyzer must be sure that the result compiles and reads well:

- The fix skips the move when the new file already binds a needed name to another package, or when the import block that it would edit holds comments.
- The fix skips generated files, files with build constraints, Go directives, or a build suffix in the file name, files that use cgo, files with comments that no declaration owns, and declarations that do not start their own line.

A fix writes only files that Git tracks and that have no changes, so `git checkout` recovers a fix that went wrong.
Diagnostics that remain after the fixes are printed, and the command exits unsuccessfully.

## Embedded analyzers

constable runs four analyzers from other modules: `errcheck`, `sumtype`, `testifylint`, and `nilaway`.
Each keeps its upstream name, so its flags keep the upstream prefix, such as `-errcheck.blank`, `-sumtype.include-shared-interfaces`, and `-testifylint.disable-all`.
Run `constable -flags` for the complete list.
Each embedded analyzer skips files that carry the generated-code comment, the same as the constable analyzers.

### errcheck

Reports an error that the code discards.

### sumtype

Reports a type switch over a sum type that omits a variant.
Mark the interface with `//sumtype:decl` and the switch must cover every variant.

### testifylint

Reports a test that misuses testify.

### nilaway

Reports a nil value that flows to a dereference.
NilAway reads every dependency by default, which costs seconds and gigabytes.
Give it the module of the code that constable analyzes:

```sh
constable -nilaway_config.include-pkgs=$(go list -m) ./...
```

The `nilaway_config` prefix is the name of the analyzer that holds the configuration, not the name of the analyzer that reports the diagnostic.
The driver registers the flags of a required analyzer, so this flag reaches the analyzer that reads it.

## Development

Run all fixes,

```sh
mise run fix
```

Run all tests,

```sh
mise run test
```

Run all checks,

```sh
mise run check
```
