package fix

type Box[T any] struct{}

// Regular documents a regular method of a generic receiver.
func (Box[T]) Regular() {}

// Generic documents a generic method.
func (Box[T]) Generic[U any](u U) {} // want "method Generic of type Box should be sorted before method Regular"

type Plain struct{}

func (Plain) Z() {}

// A comment travels with its method.
func (Plain) A() {} // want "method A of type Plain should be sorted before method Z"
