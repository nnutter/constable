package m

type Good struct{}

func (g Good) A() {}

func (g Good) B() {}

type Split struct{}

func (s Split) A() {}

type Unsorted struct{}

func (u Unsorted) B() {}

func (u Unsorted) A() {} // want "method A of type Unsorted should be sorted before method B"

type Mixed struct{}

func (m Mixed) A() {}

func (m *Mixed) B() {}

type MixedUnsorted struct{}

func (m *MixedUnsorted) B() {}

func (m MixedUnsorted) A() {} // want "method A of type MixedUnsorted should be sorted before method B"

type Box[T any] struct {
	v T
}

func (b Box[T]) A() {}

func (b Box[T]) B() {}

type UnsortedBox[T any] struct {
	v T
}

func (b UnsortedBox[T]) B() {}

func (b UnsortedBox[T]) A() {} // want "method A of type UnsortedBox should be sorted before method B"
