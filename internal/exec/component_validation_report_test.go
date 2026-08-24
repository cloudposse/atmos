package exec

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	m "github.com/cloudposse/atmos/pkg/merge"
)

func TestComponentValidationReport(t *testing.T) {
	t.Cleanup(ClearLastMergeContext)

	assert.Empty(t, ComponentValidationReport("api", nil).Diagnostics)

	context := m.NewMergeContext()
	context.EnableProvenance()
	context.RecordProvenance("components.terraform.api.vars.image", m.ProvenanceEntry{File: "stacks/api.yaml", Line: 12, Column: 9})
	SetLastMergeContext(context)

	report := ComponentValidationReport("api", errors.New(`invalid value: {"instanceLocation":"/vars/image"}`))
	require.Len(t, report.Diagnostics, 1)
	assert.Equal(t, "component", report.Diagnostics[0].Source)
	assert.Equal(t, "jsonschema", report.Diagnostics[0].RuleID)
	assert.Equal(t, "stacks/api.yaml", report.Diagnostics[0].File)
	assert.Equal(t, 12, report.Diagnostics[0].Line)

	fallback := ComponentValidationReport("api", errors.New("policy rejected component"))
	require.Len(t, fallback.Diagnostics, 1)
	assert.Equal(t, "component", fallback.Diagnostics[0].RuleID)
	assert.Empty(t, fallback.Diagnostics[0].File)
}

func TestComponentPointerPathAndProvenance(t *testing.T) {
	assert.Equal(t, "", componentPointerPath("/"))
	assert.Equal(t, "vars.a/b.~name", componentPointerPath("/vars/a~1b/~0name"))
	assert.Nil(t, componentProvenance(nil, "api", "vars.image"))
	assert.Nil(t, componentProvenance(m.NewMergeContext(), "api", "vars.image"))

	context := m.NewMergeContext()
	context.EnableProvenance()
	context.RecordProvenance("components.terraform.api.vars", m.ProvenanceEntry{File: "base.yaml", Line: 3, Column: 1})
	context.RecordProvenance("components.terraform.api.vars.image", m.ProvenanceEntry{File: "override.yaml", Line: 8, Column: 5})
	context.RecordProvenance("components.terraform.other.vars.image", m.ProvenanceEntry{File: "other.yaml", Line: 1})

	exact := componentProvenance(context, "api", "vars.image")
	require.NotNil(t, exact)
	assert.Equal(t, "override.yaml", exact.File)
	ancestor := componentProvenance(context, "api", "vars.image.tag")
	require.NotNil(t, ancestor)
	assert.Equal(t, "override.yaml", ancestor.File)
	assert.Nil(t, componentProvenance(context, "missing", "vars.image"))
}
