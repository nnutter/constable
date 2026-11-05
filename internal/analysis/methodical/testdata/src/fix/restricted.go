package fix

import "fmt"

// Show stays here because the move would leave the fmt import unused.
func (Restricted) Show() { fmt.Print("x") } // want "method Show of type Restricted should be in same file"
