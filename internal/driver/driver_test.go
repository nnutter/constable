package driver_test

import (
	"go/token"
	"os"
	"path/filepath"
	"testing"

	"github.com/nnutter/constable/internal/driver"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRelativePosition(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "sub", "a.go")
	require.NoError(t, os.MkdirAll(filepath.Dir(file), 0o755))
	require.NoError(t, os.WriteFile(file, []byte("package sub\n"), 0o644))

	fset := token.NewFileSet()
	f := fset.AddFile(file, fset.Base(), len("package sub\n"))
	f.AddLine(0)

	position := driver.RelativePosition(dir, fset, f.Pos(0))
	assert.Equal(t, filepath.Join("sub", "a.go"), position.Filename)
}
