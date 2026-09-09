package c

func Simple() {}

func WithIf(x int) int {
	if x > 0 {
		return 1
	}
	return 0
}

func WithLoop(xs []int) int {
	total := 0
	for i := 0; i < len(xs); i++ {
		total += xs[i]
	}
	for _, x := range xs {
		total += x
	}
	return total
}

func WithLogic(a, b, c bool) bool {
	if a && b || c {
		return true
	}
	return false
}

func WithSwitch(x int) int {
	switch x {
	case 1:
		return 1
	case 2:
		return 2
	default:
		return 0
	}
}

func WithSelect(ch chan int) int {
	select {
	case v := <-ch:
		return v
	case v := <-ch:
		return v
	default:
		return 0
	}
}

func WithClosure(x int) int {
	f := func(y int) int {
		if y > 0 {
			return 1
		}
		return 0
	}
	return f(x)
}

func AllConstructs(x int, xs []int, ch chan int) int { // want "cyclomatic complexity 11 exceeds limit 10 \\(function AllConstructs\\)"
	n := 0
	if x > 0 { // +1 if
		n++
	}
	for i := 0; i < len(xs); i++ { // +1 for
		n += xs[i]
	}
	for _, v := range xs { // +1 range
		n += v
	}
	switch x {
	case 1: // +1 case
		n++
	case 2: // +1 case
		n++
	default: // +1 default
		n++
	}
	select {
	case v := <-ch: // +1 comm
		n += v
	}
	if x > 0 && x < 10 || x > 20 { // +1 if, +1 &&, +1 ||
		n++
	}
	return n
}

type T struct{}

func (T) SimpleMethod() {}

func AtLimit(x int) int {
	n := 0
	if x > 0 {
		n++
	}
	if x > 1 {
		n++
	}
	if x > 2 {
		n++
	}
	if x > 3 {
		n++
	}
	if x > 4 {
		n++
	}
	if x > 5 {
		n++
	}
	if x > 6 {
		n++
	}
	if x > 7 {
		n++
	}
	if x > 8 {
		n++
	}
	return n
}

func OverLimit(x int) int { // want "cyclomatic complexity 11 exceeds limit 10 \\(function OverLimit\\)"
	n := 0
	if x > 0 {
		n++
	}
	if x > 1 {
		n++
	}
	if x > 2 {
		n++
	}
	if x > 3 {
		n++
	}
	if x > 4 {
		n++
	}
	if x > 5 {
		n++
	}
	if x > 6 {
		n++
	}
	if x > 7 {
		n++
	}
	if x > 8 {
		n++
	}
	if x > 9 {
		n++
	}
	return n
}
