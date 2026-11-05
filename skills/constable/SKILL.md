---
name: constable
description: Use constable to check Go packages with its analyzers and to repair each diagnostic that it reports.
---

# Go Constable

`constable` is a static analysis tool for Go.
It runs a set of analyzers on each package and prints one diagnostic per line.
Use it to check a change, and to hold a repository rule in a hook or in CI.

## Run it

Run it from the module root,

```sh
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

An exit status `1` with `analysis skipped due to errors in package` means the package does not build.
Fix the build error first, then run constable again.

## Repair each diagnostic

### nonmutating

```text
//constable:nonmutating function mutates pointer parameter p
//constable:nonmutating function mutates parameter s
//constable:nonmutating function deletes from map parameter m
//constable:nonmutating method mutates receiver r
```

The statement writes to data that the caller owns.
The non-mutating contract section below lists the writes that count.
The message names the origin: `pointer parameter` for a write through a pointer, `parameter` for a slice, a map, or another reference, `receiver` for a method receiver, and `deletes` for `delete` on a map.
Repair it in this order:

1. Stop the write.
   Copy the data and write to the copy, or return a new value for the caller to assign.
1. Delete the directive when the function must change caller-visible state, because the contract is wrong for that function.

`p = nil` on its own reports nothing, but the caller's data still changes when the code also writes through `p`.
Do not hide a diagnostic by assigning a new value to the parameter.
Prefer the copy or the new value.

### methodical

```text
method Read of type Buffer should be in same file as type definition
method Append of type Buffer should be sorted before method Read
method Len of type Buffer should be grouped with method Read
```

Run the fix,

```sh
constable -fix -diff ./...   # read the change
constable -fix ./...         # write the change
```

The fix moves every method of the reported type directly after the type declaration.
It orders those methods by name, with a generic method after the non-generic methods.
It adds the imports that a moved method needs and removes the imports that the old file no longer uses.

Move a method by hand when the fix skips it.
The fix skips a generated file, a file with build constraints or Go directives, a file that uses cgo, a file whose import block holds comments, a file with comments that no declaration owns, and a declaration that does not start its own line.
The fix writes only a file that Git tracks and that has no changes, so `git checkout` recovers the file.

### predicates

```text
use predicate function instead of "b" in compound conditional
```

The quoted text is the operand to replace.
Move that operand into a function that returns `bool`, and call the function in its place,

```go
func isB() bool { return b }

if a && isB() { // was: if a && b {
```

A variable does not satisfy the analyzer.
Only a call does.

### testify

```text
use testify/assert instead of testing.Error
use testify/require instead of testing.Fatal
```

Replace the `testing` call with the testify call, and add the import.
`assert` does not stop the test, so it replaces `t.Error`, `t.Errorf`, and `t.Fail`. `require` stops the test, so it replaces `t.Fatal`, `t.Fatalf`, and `t.FailNow`.
Prefer a specific assertion, such as `assert.Equal` or `require.NoError`, because it prints the values that differ.

### complexity

```text
cyclomatic complexity 12 exceeds limit 10 (function Busy)
```

Extract branches into smaller functions until the count is at or below the limit.
Extract a branch when you can give the new function a name from the problem domain, such as `isOverdue` or `hasOpenInvoice`.
That name states the fact that the condition tests, so the reader sees the rule instead of the steps.
A name that only restates the steps, such as `checkFlagsAndReturnTwo`, does not help the reader.
When only that kind of name fits, the branch is not a good candidate for extraction.
Split the function somewhere else.
When the function is one coherent rule, raise the limit instead.
The count starts at 1 and adds 1 for each `if`, `for`, `range`, `case`, and `select` clause, `&&`, and `||`.
A closure adds to the function that holds it, and a `default` clause counts.
`-complexity.limit=15` raises the limit for the whole run.
Say why in the change when you use it.

### errcheck, sumtype, testifylint, and nilaway

These analyzers come from other modules.
Their messages, flags, and repairs are theirs.

| Analyzer | Message | Repair |
| --- | --- | --- |
| `errcheck` | `unchecked error` | Handle the error. Assign it to `_` only when the code discards it on purpose. |
| `sumtype` | `missing cases for Square` | Add the case for the variant, or add a `default` clause and set `-sumtype.default-signifies-exhaustive`. |
| `testifylint` | text that names a testify check | Follow the message. The check name states the misuse, such as `expected-actual`, `formatter`, or `require-error`. |
| `nilaway` | `Potential nil panic detected` | Remove the nil flow: check the value, return early, or initialize it where it is declared. |

The embedded analyzers have more flags than this document lists.
Their flags keep the analyzer name as a prefix, such as `-errcheck.blank` or `-testifylint.disable-all`.

NilAway reads every package that it loads by default, which costs seconds and gigabytes.
constable limits it to the main modules of the packages that it analyzes.
Set `-nilaway_config.include-pkgs=example.com/my/module` to choose the packages, or `-nilaway.include-all-pkgs` to analyze the dependencies and the standard library too.
Suppress a report that the code cannot fix with a `//nolint:nilaway` comment on the reported statement.

## The non-mutating contract

`//constable:nonmutating` states that the function or method does not change state that the caller can observe through its arguments or its receiver.
The check does not prove that the function is pure or free of side effects.

Put the exact directive immediately before the declaration, with no space between `//` and `constable:`.

```go
//constable:nonmutating
func ReadValue(p *int) int {
	return *p
}
```

The directive applies to functions and methods only.

### Reported mutations

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

### Allowed code

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

## Workflow

1. Identify the functions and methods whose contract must not change caller-owned data.
1. Add `//constable:nonmutating` immediately before each of them.
1. Run `constable ./...`.
1. Repair each diagnostic with the advice above.
1. Re-run constable and the package tests before you finalize the change.

## Scope and limits

The non-mutating check finds direct writes in one function body: assignment, `++`, `--`, index writes, field writes, and `delete`.
It does not model a call to a helper function, a write through `append`, or an indirect path such as an interface method or a function value.
It is not a general immutability checker, and it does not by itself guarantee that a function has no I/O, global state changes, goroutines, reflection, or unsafe operations.
The analyzer recognizes only the `nonmutating` directive.

When a run is clean, inspect the implementation before you claim that it is fully non-mutating.
