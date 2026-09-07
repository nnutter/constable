package directive_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/nnutter/constable/internal/directive"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		comment string
		want    []directive.Check
	}{
		{
			name:    "bare tag",
			comment: "//constable:nonmutating",
			want: []directive.Check{
				{Name: "nonmutating"},
			},
		},
		{
			name:    "not a constable tag",
			comment: "//go:noinline",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := directive.Parse(test.comment)
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestParseErrors(t *testing.T) {
	tests := []struct {
		name    string
		comment string
	}{
		{name: "missing name", comment: "//constable:"},
		{name: "missing quote", comment: "//constable:nonmutating:\"unterminated"},
		{name: "unquoted value", comment: "//constable:nonmutating:strict"},
		{name: "invalid check tag", comment: "//constable:othercheck"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := directive.Parse(test.comment)
			assert.Error(t, err)
		})
	}
}

func TestHasNonmutating(t *testing.T) {
	assert.True(t, directive.HasNonmutating("//constable:nonmutating"))
	assert.False(t, directive.HasNonmutating("//go:noinline"))
}
