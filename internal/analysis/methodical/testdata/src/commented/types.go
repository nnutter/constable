package commented

import (
	// Keep this note beside its import.
	"strings"
)

type Widget struct{}

func (Widget) Trim(s string) string { return strings.TrimSpace(s) }
