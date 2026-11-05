package commented

import "fmt"

type Gadget struct{}

func (Gadget) Show(s string) { fmt.Print(s) }

func (Widget) Alpha(s string) { fmt.Print(s) } // want "method Alpha of type Widget should be in same file"
