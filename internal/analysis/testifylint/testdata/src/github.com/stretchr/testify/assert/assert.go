// Package assert is a stub of github.com/stretchr/testify/assert that holds
// the calls the testdata makes. analysistest loads testdata in GOPATH mode, so
// the testdata cannot import the real module.
package assert

// TestingT is the part of testing.T that the assertions use.
type TestingT interface {
	Errorf(format string, args ...any)
}

// True asserts that value is true.
func True(t TestingT, value bool, msgAndArgs ...any) bool { return value }

// NotEqual asserts that expected and actual are not equal.
func NotEqual(t TestingT, expected, actual any, msgAndArgs ...any) bool { return true }
