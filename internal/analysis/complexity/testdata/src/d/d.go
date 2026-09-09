package d

func Simple() {}

func TwoBranches(x int) int { // want "cyclomatic complexity 3 exceeds limit 2 \\(function TwoBranches\\)"
	if x > 0 {
		return 1
	}
	if x > 1 {
		return 2
	}
	return 0
}
