package t

import "testing"

func UsesError(t *testing.T) {
	t.Error("boom") // want "use testify/assert instead of testing.Error"
}

func UsesErrorf(t *testing.T) {
	t.Errorf("boom %d", 1) // want "use testify/assert instead of testing.Errorf"
}

func UsesFail(t *testing.T) {
	t.Fail() // want "use testify/assert instead of testing.Fail"
}

func UsesFailNow(t *testing.T) {
	t.FailNow() // want "use testify/require instead of testing.FailNow"
}

func UsesFatal(t *testing.T) {
	t.Fatal("boom") // want "use testify/require instead of testing.Fatal"
}

func UsesFatalf(t *testing.T) {
	t.Fatalf("boom %d", 1) // want "use testify/require instead of testing.Fatalf"
}

func UsesBenchmarkError(b *testing.B) {
	b.Error("boom") // want "use testify/assert instead of testing.Error"
	b.Fatal("boom") // want "use testify/require instead of testing.Fatal"
}

func UsesInterface(tb testing.TB) {
	tb.Error("boom")  // want "use testify/assert instead of testing.Error"
	tb.Fatalf("boom") // want "use testify/require instead of testing.Fatalf"
}

func AllowedCalls(t *testing.T) {
	t.Log("info")
	t.Helper()
	t.Skip("skip")
}

type fake struct{}

func (f fake) Error() {}

func (f fake) Fatal() {}

func FakeCalls(f fake) {
	f.Error()
	f.Fatal()
}
