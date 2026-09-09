package report_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/nnutter/constable/internal/report"
)

func TestMessages(t *testing.T) {
	assert.Equal(t, "//constable:nonmutating function mutates pointer parameter p", report.MutatesPointerParameter("p"))
	assert.Equal(t, "//constable:nonmutating method mutates receiver c", report.MutatesReceiver("c"))
	assert.Equal(t, "//constable:nonmutating function mutates parameter s", report.MutatesParameter("s"))
	assert.Equal(t, "//constable:nonmutating function deletes from map parameter m", report.DeletesFromMapParameter("m"))
	assert.Equal(t, "method B of type Split should be in same file as type definition", report.MethodShouldBeInSameFile("Split", "B"))
	assert.Equal(t, "method A of type Unsorted should be sorted before method B", report.MethodShouldBeSorted("Unsorted", "A", "B"))
	assert.Equal(t, "use testify/assert instead of testing.Error", report.UseTestifyInstead("Error", "assert"))
	assert.Equal(t, "use testify/require instead of testing.Fatal", report.UseTestifyInstead("Fatal", "require"))
	assert.Equal(t, "cyclomatic complexity 12 exceeds limit 10 (function F)", report.ComplexityExceedsLimit("F", 12, 10))
}
