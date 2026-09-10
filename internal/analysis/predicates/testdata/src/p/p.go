package p

func isA() bool { return true }

func isB() bool { return false }

func BothVars(a, b bool) {
	if a && b { // want "use predicate function instead of \"b\" in compound conditional"
		println("both")
	}
}

func BothPredicates() {
	if isA() && isB() {
		println("both")
	}
}

func MixedVarAndPredicate(a bool) {
	if a && isB() {
		println("mixed")
	}
}

func PredicateThenVar(a bool) {
	if isB() && a { // want "use predicate function instead of \"a\" in compound conditional"
		println("mixed")
	}
}

func NegatedVar(a, b bool) {
	if !a && b { // want "use predicate function instead of \"b\" in compound conditional"
		println("negated")
	}
}

func VarThenNegated(a, b bool) {
	if b && !a { // want "use predicate function instead of \"!a\" in compound conditional"
		println("negated")
	}
}

func NegatedPredicate(a bool) {
	if !isA() && a { // want "use predicate function instead of \"a\" in compound conditional"
		println("negated")
	}
}

func VarThenNegatedPredicate(a bool) {
	if a && !isB() {
		println("negated")
	}
}

func OrVars(a, b bool) {
	if a || b { // want "use predicate function instead of \"b\" in compound conditional"
		println("or")
	}
}

type T struct{ Active bool }

func FieldAccess(t T, b bool) {
	if b && t.Active { // want "use predicate function instead of \"t.Active\" in compound conditional"
		println("field")
	}
}

func MapAccess(m map[int]bool, b bool) {
	if b && m[0] { // want "use predicate function instead of \"m\\[0\\]\" in compound conditional"
		println("map")
	}
}

func Comparison(x int, b bool) {
	if b && x == 1 { // want "use predicate function instead of \"x == 1\" in compound conditional"
		println("comparison")
	}
}

func ForVars(a, b bool) {
	for a && b { // want "use predicate function instead of \"b\" in compound conditional"
		break
	}
}

func AssignedVars(a, b bool) {
	c := a && b // want "use predicate function instead of \"b\" in compound conditional"
	_ = c
}

func AssignedOr(a, b bool) bool {
	c := a || b // want "use predicate function instead of \"b\" in compound conditional"
	return c
}

func AssignedPredicates() bool {
	return isA() && isB()
}

func BoolLiteral(a bool) {
	if a && true {
		println("literal")
	}
}

func LiteralThenVar(a bool) {
	if true && a { // want "use predicate function instead of \"a\" in compound conditional"
		println("literal")
	}
}

func Nested(a, b, c bool) {
	if a && b && c { // want "use predicate function instead of \"b\" in compound conditional" "use predicate function instead of \"c\" in compound conditional"
		println("nested")
	}
}

func Parens(a, b bool) {
	if (a) && (b) { // want "use predicate function instead of \"b\" in compound conditional"
		println("parens")
	}
}

func SingleVar(a bool) {
	if a {
		println("single")
	}
}

func SingleAssign(a bool) {
	c := a
	_ = c
}
