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

func TestVerifyPlanfile_EmptyPlanFile(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		PlanFile: "",
	}

	err := VerifyPlanfile(info)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	assert.Contains(t, err.Error(), "--verify-plan requires a planfile")
}

func TestVerifyPlanfile_PlanFileDoesNotExist(t *testing.T) {
	info := &schema.ConfigAndStacksInfo{
		PlanFile: filepath.Join(t.TempDir(), "nonexistent.tfplan"),
	}

	err := VerifyPlanfile(info)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
	assert.Contains(t, err.Error(), "planfile does not exist")
}

func TestVerifyPlanfile_PlanFileExists_FailsOnStackProcessing(t *testing.T) {
	// Create a temporary planfile so we get past the existence check.
	tmpDir := t.TempDir()
	planFile := filepath.Join(tmpDir, "test.tfplan")
	err := os.WriteFile(planFile, []byte("fake plan data"), 0o644)
	require.NoError(t, err)

	info := &schema.ConfigAndStacksInfo{
		PlanFile:         planFile,
		ComponentFromArg: "test-component",
		Stack:            "test-stack",
	}

	// This will fail because there's no valid atmos config, which is expected.
	// We're testing that the function gets past the planfile checks.
	err = VerifyPlanfile(info)
	require.Error(t, err)
	// Should NOT be ErrPlanVerificationFailed since it fails on config init, not verification.
	assert.NotErrorIs(t, err, errUtils.ErrPlanVerificationFailed)
}
