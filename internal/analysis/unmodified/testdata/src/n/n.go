package n

//nolint:unmodified
var suppressedDoc = 1

var suppressedLine = 2 //nolint:unmodified

var (
	suppressedBlockA = 4 //nolint:unmodified
	suppressedBlockB = 5
)

var reported = 3 //nolint:other // want "variable reported is never modified and should be a constant"

var (
	reportedBlockA = 6 // want "variable reportedBlockA is never modified and should be a constant"
	reportedBlockB = 7 // want "variable reportedBlockB is never modified and should be a constant"
)

//nolint:unmodified
var (
	suppressedWholeA = 8
	suppressedWholeB = 9
)

func Use() int {
	var suppressedLocal = 10 //nolint:unmodified
	var reportedLocal = 11   //nolint:nilaway,unmodifiedX // want "variable reportedLocal is never modified and should be a constant"
	var nakedBare = 12       //nolint // want "variable nakedBare is never modified and should be a constant"
	var reportedOther = 13   //nolint:other // want "variable reportedOther is never modified and should be a constant"
	return suppressedDoc + suppressedLine + suppressedBlockA + suppressedBlockB + reported + reportedBlockA + reportedBlockB + suppressedWholeA + suppressedWholeB + suppressedLocal + reportedLocal + nakedBare + reportedOther
}
