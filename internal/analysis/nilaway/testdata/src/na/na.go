package na

type T struct{ X int }

func F() {
	var t *T
	_ = t.X // want `Potential nil panic detected`
}
