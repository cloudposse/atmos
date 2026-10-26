package validate

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/validation"
)

func TestWriteTextSortsAndIncludesColumns(t *testing.T) {
	report := validation.Report{Diagnostics: []validation.Diagnostic{
		{File: "b.yml", Line: 2, Column: 4, RuleID: "second", Message: "second error"},
		{File: "a.yml", Line: 3, Column: 1, RuleID: "first", Message: "first error"},
	}}
	var output bytes.Buffer

	require.NoError(t, WriteText(&output, report))
	assert.Equal(t, "a.yml:3:1: first error [first]\nb.yml:2:4: second error [second]\n", output.String())
}
