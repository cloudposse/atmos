package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestTerraformPlanDiffValidate(t *testing.T) {
	t.Parallel()
	assert.NoError(t, (TerraformPlanDiff{}).Validate())
	assert.NoError(t, (TerraformPlanDiff{IgnoreChanges: []string{
		"aws_db_instance.latest_restorable_time", "custom_resource.custom_attribute",
	}}).Validate())
	for _, entry := range []string{
		"", "latest_restorable_time", ".attribute", "resource.", "resource.attribute.nested",
		"*.attribute", "resource.*", "resource.attributes[0]", " resource.attribute", "resource.attribute ",
		"resource .attribute", "resource.0", "0resource.attribute",
	} {
		t.Run(entry, func(t *testing.T) {
			t.Parallel()
			err := (TerraformPlanDiff{IgnoreChanges: []string{entry}}).Validate()
			assert.ErrorIs(t, err, ErrInvalidPlanDiffIgnoreChange)
			assert.ErrorContains(t, err, "components.terraform.plan_diff.ignore_changes")
		})
	}
}
