package l

func unusedHelper() int { return 1 } // want "unused unexported function unusedHelper is never used"

func usedHelper() int { return 2 }

func exportedEntry() int { return usedHelper() }

func init() {}

type unusedType struct{ n int } // want "unused unexported type unusedType is never used"

type usedType struct{ n int }

func makeUsedType() usedType { return usedType{n: 1} }

var unusedVar = 1 // want "unused unexported variable unusedVar is never used"

var usedVar = 2

const unusedConst = 1 // want "unused unexported constant unusedConst is never used"

const usedConst = 2

var keptByBlank = 3

var _ = keptByBlank

func ReadUsed() (int, int, usedType) {
	_ = exportedEntry()
	return usedVar, usedConst, makeUsedType()
}

var (
	unusedGroupedVar = 1 // want "unused unexported variable unusedGroupedVar is never used"
	usedGroupedVar   = 2
)

var _ = usedGroupedVar

const (
	unusedGroupedConst = 1 // want "unused unexported constant unusedGroupedConst is never used"
	usedGroupedConst   = 2
)

var _ = usedGroupedConst

type WithMethod struct{ n int }

func (w WithMethod) unusedMethod() {}

var _ = WithMethod{}

type WithField struct{ unusedField int }

var _ = WithField{unusedField: 1}

func unusedGeneric[T any](x T) T { return x } // want "unused unexported function unusedGeneric is never used"

func usedGeneric[T any](x T) T { return x }

var _ = usedGeneric[int]

type unusedBox[T any] struct{ v T } // want "unused unexported type unusedBox is never used"

type usedBox[T any] struct{ v T }

var _ = usedBox[int]{v: 1}

func Shadowed() int {
	unusedHelper := 1
	return unusedHelper
}

type code int

const (
	unusedCode code = iota
	usedCode
)

var _ = usedCode
