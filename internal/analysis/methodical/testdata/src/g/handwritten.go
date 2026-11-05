package g

// Methods on a generated type cannot move next to the type, so the analyzer
// leaves them alone even though they are in a handwritten file.
func (g Gen) D() {}

func (g Gen) C() {}
