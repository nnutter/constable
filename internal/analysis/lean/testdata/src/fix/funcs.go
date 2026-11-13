package fix

import "fmt"

// unused prints and nothing reads it back.
func unused() { // want "unused unexported function unused is never used"
	fmt.Println("x")
}

func helper() {}

// Keep exercises the used helper.
func Keep() {
	helper()
}
