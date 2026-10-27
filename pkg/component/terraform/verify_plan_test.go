package terraform

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestFinalizeVerification(t *testing.T) {
	t.Parallel()

	freshPlan := filepath.Join(t.TempDir(), "fresh.tfplan")

	t.Run("drift + fail returns verification error", func(t *testing.T) {
		t.Parallel()
		info := &schema.ConfigAndStacksInfo{}
		err := finalizeVerification(info, freshPlan, "  ~ resource changed", true, schema.PlanfileVerifyFail)
		require.Error(t, err)
		assert.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
		assert.Contains(t, err.Error(), "resource changed")
		// On failure the fresh plan is NOT selected for apply.
		assert.Empty(t, info.PlanFile)
		assert.False(t, info.UseTerraformPlan)
	})

	t.Run("drift + warn proceeds with the fresh plan", func(t *testing.T) {
		t.Parallel()
		info := &schema.ConfigAndStacksInfo{}
		err := finalizeVerification(info, freshPlan, "  ~ resource changed", true, schema.PlanfileVerifyWarn)
		require.NoError(t, err)
		assert.Equal(t, freshPlan, info.PlanFile)
		assert.True(t, info.UseTerraformPlan)
	})

	t.Run("no drift selects the fresh plan", func(t *testing.T) {
		t.Parallel()
		info := &schema.ConfigAndStacksInfo{}
		err := finalizeVerification(info, freshPlan, "", false, schema.PlanfileVerifyFail)
		require.NoError(t, err)
		assert.Equal(t, freshPlan, info.PlanFile)
		assert.True(t, info.UseTerraformPlan)
	})
}
