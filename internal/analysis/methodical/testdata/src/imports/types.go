package imports

import "strings"

type Widget struct{}

func (Widget) Trim(s string) string { return strings.TrimSpace(s) }
