package t

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCompare(t *testing.T) {
	assert.True(t, 1 != 2) // want `compares: use assert.NotEqual`
}
