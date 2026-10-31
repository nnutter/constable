package fix

// CrossMethod stays documented after the move.
func (Cross) CrossMethod() {} // want "method CrossMethod of type Cross should be in same file"
