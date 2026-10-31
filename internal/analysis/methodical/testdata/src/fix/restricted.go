package fix

import "fmt"

func (Restricted) Show() { fmt.Print("x") } // want "method Show of type Restricted should be in same file"
