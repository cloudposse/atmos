package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// missingPlanConfig builds an AtmosConfiguration with planfile storage configured
// and an explicit on_missing mode. An explicit on_missing bypasses CI detection in
// ResolveMissingMode, keeping these assertions deterministic regardless of whether
// the test binary itself runs inside CI.
func missingPlanConfig(onMissing schema.PlanfileVerifyMode) *schema.AtmosConfiguration {
	c := &schema.AtmosConfiguration{}
	c.Components.Terraform.Planfiles = schema.PlanfilesConfig{
		Priority:  []string{"github"},
		OnMissing: onMissing,
	}
	return c
}

func TestHandleMissingStoredPlan(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "mycomponent", Stack: "prod"}

	t.Run("fail errors with ErrStoredPlanfileMissing", func(t *testing.T) {
		err := handleMissingStoredPlan(missingPlanConfig(schema.PlanfileVerifyFail), info)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrStoredPlanfileMissing)
		assert.Contains(t, err.Error(), "mycomponent")
		assert.Contains(t, err.Error(), "prod")
	})

	t.Run("warn proceeds without error", func(t *testing.T) {
		assert.NoError(t, handleMissingStoredPlan(missingPlanConfig(schema.PlanfileVerifyWarn), info))
	})

	t.Run("off proceeds without error", func(t *testing.T) {
		assert.NoError(t, handleMissingStoredPlan(missingPlanConfig(schema.PlanfileVerifyOff), info))
	})

	// Negative path: a config with no planfile storage and no CLI override must
	// never block the deploy, even though ci.IsCI() may report true in CI.
	t.Run("no storage never fails", func(t *testing.T) {
		assert.NoError(t, handleMissingStoredPlan(&schema.AtmosConfiguration{}, info))
	})
}
