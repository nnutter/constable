package generic

type Box[T any] struct{}

// Regular documents a regular method of a generic receiver.
func (Box[T]) Regular() {}

// Generic documents a generic method.
func (Box[T]) Generic[U any](u U) {} // want "method Generic of type Box should be sorted before method Regular"
