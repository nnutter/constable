---
name: constable
description: Use constable to check that annotated Go functions and methods do not mutate caller-visible argument or receiver state.
---

# Go Constable

`constable` is a Go static analysis tool for opt-in non-mutation checks.
Use it when a function or method must read its inputs without changing state that the caller can observe.
The check does not guarantee that the function is pure or free of all side effects.

## Installation and invocation

Install the released command, then run it from the module root,

```sh
go install github.com/nnutter/constable/cmd/constable@latest
constable ./...
```

Each diagnostic has the form `file:line:col: message` and goes to stderr.
Exit status `0` means no diagnostic, `3` means at least one diagnostic, and `1` means constable failed.
An invocation that finds no diagnostic succeeds silently.

Further flags:

- `-json` prints JSON and always exits `0`.
- `-c N` prints N lines of source around each reported line.
- `-fix` writes the fixes, and `-fix -diff` prints them as a diff and writes nothing.
- `-<analyzer>=false` turns off one analyzer, for example `-complexity=false`.
- `-flags` lists every flag, and `help <analyzer>` describes one analyzer.

A file that carries the generated-code comment `// Code generated ... DO NOT EDIT.` is skipped, because the next run of the generator overwrites it.
Change such a file through its generator.
`-include-generated` reports in it anyway.

## Annotating Go code

Put the exact directive `//constable:nonmutating` immediately before the function or method declaration.
Do not put a space between `//` and `constable:`.

```go
//constable:nonmutating
func ReadValue(p *int) int {
	return *p
}
```

The directive applies to functions and methods only.
Do not add it to a function that must change caller-visible state.

## Reported mutations

The analyzer reports these writes:

- A write through a pointer parameter or a pointer receiver, such as `*p = 1`.
- A write to a slice element, a map entry, or a field that the parameter reaches.
- `delete` on a map parameter.
- A write to a reference-like field of a value receiver, such as a slice field.
- The same writes through an alias that a local variable holds.

```go
type Buffer struct{ data []byte }

//constable:nonmutating
func (b Buffer) ClearFirstByte() {
	b.data[0] = 0 // b.data shares the caller's backing array
}
```

Nested references count too, such as a write to an element of a slice of slices.

## Allowed code

- A new value for the parameter itself, such as `p = nil`.
- A write to a local variable, or to memory that the function allocates.
- A scalar field write on a value receiver, because that field belongs to the receiver copy.

```go
type Count struct{ n int }

//constable:nonmutating
func (c Count) Increment() Count {
	c.n++ // c is the receiver copy
	return c
}
```

## Predicates

The predicates analyzer reports `&&` and `||` operands, except the first, that are not calls to a predicate function.
The first operand always runs, so coverage observes short-circuiting through the operands that follow.

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

The check covers `if` and `for` conditions and bool assignments such as `c := a && b`.
It accepts an operand that is a call, a compound expression, a negated call, or `true`, `false`, or `nil`.

## Workflow

1. Identify the functions and methods whose contract must not change caller-owned data.
1. Add `//constable:nonmutating` immediately before each of them.
1. Run `constable ./...`.
1. For each diagnostic, remove the caller-visible mutation, or delete the directive when the contract is wrong.
1. Re-run constable and the package tests before you finalize the change.

Prefer a new return value or a defensive copy when the implementation must transform caller-provided data.
Do not hide a diagnostic by assigning a new value to the parameter while the code still writes through it.

## Scope and limits

The check finds direct writes in one function body: assignment, `++`, `--`, index writes, field writes, and `delete`.
It does not model a call to a helper function, a write through `append`, or an indirect path such as an interface method or a function value.
The analyzer recognizes only the `nonmutating` directive, and it is not a general immutability checker.
It does not by itself guarantee that a function has no I/O, global state changes, goroutines, reflection, or unsafe operations.

When a run is clean, inspect the implementation before you claim that it is fully non-mutating.
