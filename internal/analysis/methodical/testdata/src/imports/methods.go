package imports

// Alpha sorts before Trim.
func (Widget) Alpha() {} // want "method Alpha of type Widget should be in same file"
