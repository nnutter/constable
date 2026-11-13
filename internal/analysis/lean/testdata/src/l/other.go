package l

func ExportedOther() int { return usedHelperOther() }

func usedHelperOther() int { return 3 }

func unusedInOther() int { return 4 } // want "unused unexported function unusedInOther is never used"
