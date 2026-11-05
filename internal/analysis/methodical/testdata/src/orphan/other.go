package orphan

import "fmt"

// Alpha leaves this file without the fmt import that only it uses.
func (Widget) Alpha(s string) { fmt.Print(s) } // want "method Alpha of type Widget should be in same file"
