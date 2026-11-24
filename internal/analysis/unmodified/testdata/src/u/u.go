package u

var unmodified = 1 // want "variable unmodified is never modified and should be a constant"

var typedValue float64 = 2.5 // want "variable typedValue is never modified and should be a constant"

const base = 3

var derived = base + 1 // want "variable derived is never modified and should be a constant"

var (
	blockA = 6 // want "variable blockA is never modified and should be a constant"
	blockB = 7 // want "variable blockB is never modified and should be a constant"
)

var Exported = 8

var mutated = 9

var addressTaken = 10

var computed = compute()

var list = []int{1}

var specA, specMutated = 11, 12 // want "variable specA is never modified and should be a constant"

type counter int

func (c *counter) Inc() { (*c)++ }

func (c counter) Value() int { return int(c) }

var counted counter = 16

var readOnlyCounter counter = 17 // want "variable readOnlyCounter is never modified and should be a constant"

var closureMutated = 18

var crossMutated = 20

func compute() int { return 13 }

func Mutate() {
	mutated = 99
	specMutated = 98
	ptr := &addressTaken
	*ptr = 97
	counted.Inc()
	fn := func() { closureMutated = 19 }
	fn()
}

func Use() int {
	var local = 14 // want "variable local is never modified and should be a constant"
	var localMutated = 15
	localMutated++
	return unmodified + int(typedValue) + derived + blockA + blockB + specA + local + localMutated + Exported + addressTaken + computed + list[0] + readOnlyCounter.Value() + int(counted) + crossMutated
}
