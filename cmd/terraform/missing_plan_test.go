package terraform

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// missingPlanConfig builds an AtmosConfiguration with planfile storage configured
// and an explicit verify mode + required pointer. An explicit required bypasses CI
// detection in IsPlanRequired, keeping these assertions deterministic regardless of
// whether the test binary itself runs inside CI.
func missingPlanConfig(verify schema.PlanfileVerifyMode, required *bool) *schema.AtmosConfiguration {
	c := &schema.AtmosConfiguration{}
	c.Components.Terraform.Planfiles = schema.PlanfilesConfig{
		Priority: []string{"github"},
		Verify:   verify,
		Required: required,
	}
	return c
}

func boolPtr(b bool) *bool { return &b }

func TestHandleMissingStoredPlan(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{ComponentFromArg: "mycomponent", Stack: "prod"}

	t.Run("required errors with ErrStoredPlanfileMissing", func(t *testing.T) {
		err := handleMissingStoredPlan(missingPlanConfig(schema.PlanfileVerifyFail, boolPtr(true)), info)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrStoredPlanfileMissing)
		assert.Contains(t, err.Error(), "mycomponent")
		assert.Contains(t, err.Error(), "prod")
	})

	t.Run("not required proceeds without error", func(t *testing.T) {
		assert.NoError(t, handleMissingStoredPlan(missingPlanConfig(schema.PlanfileVerifyFail, boolPtr(false)), info))
	})

	// verify=off short-circuits to not-required even when required:true is set.
	t.Run("verify off proceeds despite required true", func(t *testing.T) {
		assert.NoError(t, handleMissingStoredPlan(missingPlanConfig(schema.PlanfileVerifyOff, boolPtr(true)), info))
	})

	// Negative path: a config with no planfile storage and no CLI override must
	// never block the deploy, even though ci.IsCI() may report true in CI.
	t.Run("no storage never fails", func(t *testing.T) {
		assert.NoError(t, handleMissingStoredPlan(&schema.AtmosConfiguration{}, info))
	})
}
