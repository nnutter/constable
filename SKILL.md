---
name: constable
description: Use constable to check that annotated Go functions and methods do not mutate caller-visible argument or receiver state.
---

# Go Constable

`constable` is a Go static analysis tool for opt-in non-mutation checks.

Use it when a function or method should read its inputs without changing state that the caller can observe.
The check does not guarantee that the function is pure or free of all side effects.

## Installation and invocation

Install the released command with:

```sh
go install github.com/nnutter/constable/cmd/constable@latest
```

Run it against the packages in a module:

```sh
constable ./...
```

The command reports diagnostics and exits unsuccessfully when an annotated function violates the contract.
An invocation with no diagnostics succeeds silently.

## Annotating Go code

Put the exact directive `//constable:nonmutating` immediately before the function or method declaration.
There must not be a space between `//` and `constable:`.

```go
//constable:nonmutating
func ReadValue(p *int) int {
 return *p
}
```

The directive applies to functions and methods only.
Do not add it to a function that is intentionally allowed to mutate caller-visible state.

## Mutations that must be avoided

The analyzer reports direct mutations through pointer, slice, and map parameters.
It also reports mutations through pointer receivers and through reference-like fields of value receivers.

```go
//constable:nonmutating
func WriteValue(p *int) {
 *p = 1 // reported
}

//constable:nonmutating
func WriteSlice(values []int) {
 values[0] = 1 // reported
}

//constable:nonmutating
func WriteMap(values map[string]int) {
 values["key"] = 1 // reported
}

//constable:nonmutating
func DeleteMapEntry(values map[string]int) {
 delete(values, "key") // reported
}

type Buffer struct {
 data []byte
}

//constable:nonmutating
func (b Buffer) ClearFirstByte() {
 b.data[0] = 0 // reported: b.data shares caller-visible backing storage
}
```

Aliases of parameters are also checked when the alias is assigned to a local variable.
Nested reference-like values, such as a slice of slices, are considered caller-visible when mutated.

## Code that is allowed

Reassigning a parameter does not mutate the value held by the caller.
Mutating a local variable or a pointer to a local variable is also allowed.
A value receiver may mutate its scalar fields because those fields belong to the receiver copy.

```go
//constable:nonmutating
func ReassignParameter(p *int) {
 p = nil // allowed
}

//constable:nonmutating
func MutateLocal() int {
 value := 0
 value++ // allowed
 return value
}

type Count struct {
 n int
}

//constable:nonmutating
func (c Count) Increment() Count {
 c.n++ // allowed: c is a value-receiver copy
 return c
}
```

## Predicates

The predicates analyzer reports `&&` and `||` operands, except the
first, that are not predicate-function calls. The first operand always
executes, so coverage observes short-circuiting through the rest.

```go
func F(a, b bool) {
 if a && b { // reported: use a && isB()
 }
}

func isB() bool { return false }

func G(a bool) {
 if a && isB() { // allowed
 }
}
```

The check covers `if` and `for` conditions and bool assignments such
as `c := a && b`, including `!a`, field and map access, and
comparisons. Operands that are already calls, compound expressions,
or `true`/`false`/`nil` are allowed.

## Agent workflow

1. Identify functions and methods whose API contract should not mutate caller-owned data.
2. Add `//constable:nonmutating` immediately before each eligible declaration.
3. Run `constable ./...` from the Go module root.
4. For every diagnostic, either remove the caller-visible mutation or remove the annotation if the contract is incorrect.
5. Re-run the analyzer and the package tests before finalizing the change.

Prefer returning a new value or making a defensive copy when the implementation needs to transform caller-provided data.
Do not silence a diagnostic by merely reassigning a parameter while continuing to mutate data reachable through it.

## Scope and limitations

The check is focused on direct, intra-procedural mutations represented by assignments, increments, slice or map indexing, and `delete`.
It is not a general immutability checker.
It does not by itself guarantee that a function has no I/O, global state changes, goroutines, reflection, unsafe operations, or other side effects.
Calls to helper functions, interface methods, function values, and other indirect mutation paths may not be modeled.
The analyzer currently recognizes only the `nonmutating` directive.

When behavior is uncertain, inspect the diagnostic and the implementation rather than assuming that a clean run proves complete immutability.
