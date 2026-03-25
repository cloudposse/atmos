package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestVerifyPlanfile_StoredPlanFileDoesNotExist(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{}
	storedPlan := filepath.Join(t.TempDir(), "nonexistent.tfplan")

	err := VerifyPlanfile(info, storedPlan)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	assert.Contains(t, err.Error(), "stored planfile does not exist")
}

func TestVerifyPlanfile_StoredPlanFileExists_FailsOnStackProcessing(t *testing.T) {
	// Create a temporary planfile so we get past the existence check.
	tmpDir := t.TempDir()
	storedPlan := filepath.Join(tmpDir, "stored.plan.tfplan")
	err := os.WriteFile(storedPlan, []byte("fake plan data"), 0o644)
	require.NoError(t, err)

	info := &schema.ConfigAndStacksInfo{
		ComponentFromArg: "test-component",
		Stack:            "test-stack",
	}

	// This will fail because there's no valid atmos config, which is expected.
	// We're testing that the function gets past the planfile checks.
	err = VerifyPlanfile(info, storedPlan)
	require.Error(t, err)
	// Should NOT be ErrPlanVerificationFailed since it fails on config init, not verification.
	assert.NotErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
}
