package er

import "os"

func F() {
	os.Remove("x") // want `unchecked error`
}
