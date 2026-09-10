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

The complexity analyzer reports functions and methods whose cyclomatic
complexity exceeds a limit. Complexity starts at 1 and adds 1 for each
`if`, `for`, `range`, `case`, `select` clause, `&&`, and `||`. Closures
fold into the enclosing function and default clauses count.

```sh
constable ./...
constable -complexity.limit=15 ./...
```

The limit defaults to 10 and is configured with `-complexity.limit`.

## Predicates

The predicates analyzer reports `&&` and `||` operands, except the
first, that are not predicate-function calls. The first operand always
executes, so coverage observes short-circuiting through the rest.

```go
if a && b { // reported: use a && isB()
}

if a && isB() { // allowed
}
```

The check also covers `for` conditions and bool assignments such as
`c := a && b`, including negated variables, field and map access,
and comparisons.

## Development

Run all fixers,

```sh
mise run fixers
```

Run all tests,

```sh
mise run tests
```

Run all linters,

```sh
mise run linters
```
