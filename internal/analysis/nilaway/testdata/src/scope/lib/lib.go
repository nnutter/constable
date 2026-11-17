package lib

// Identity's nonnil-to-nonnil contract is inferred from SSA.
func Identity(p *int) *int {
	return p
}

func Missing() *int {
	return nil
}

func Dereference() int {
	return *Missing()
}
