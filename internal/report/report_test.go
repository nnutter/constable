package report_test

import (
	"testing"

	"github.com/nnutter/constable/internal/report"
	"github.com/stretchr/testify/assert"
)

func TestMessages(t *testing.T) {
	assert.Equal(t, "//constable:nonmutating function mutates pointer parameter p", report.MutatesPointerParameter("p"))
	assert.Equal(t, "//constable:nonmutating method mutates receiver c", report.MutatesReceiver("c"))
	assert.Equal(t, "//constable:nonmutating function mutates parameter s", report.MutatesParameter("s"))
	assert.Equal(t, "//constable:nonmutating function deletes from map parameter m", report.DeletesFromMapParameter("m"))
}
