package keeps

import "fmt"

type Gadget struct{}

func (Gadget) Show() { fmt.Print("g") }

// Alpha keeps the fmt import alive through Show alone.
func (Widget) Alpha() {} // want "method Alpha of type Widget should be in same file"
