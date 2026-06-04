package hooks

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestStoreCommand_CustomComponentDispatch confirms that getOutputValue routes
// to the custom outputter (file-based) when ComponentType is non-terraform,
// and to the terraform outputter when it's terraform or unset.
func TestStoreCommand_CustomComponentDispatch(t *testing.T) {
	tests := []struct {
		name                string
		componentType       string
		expectCustomCall    bool
		expectTerraformCall bool
	}{
		{
			name:                "custom component type routes to custom outputter",
			componentType:       "agent",
			expectCustomCall:    true,
			expectTerraformCall: false,
		},
		{
			name:                "terraform component routes to terraform outputter",
			componentType:       "terraform",
			expectCustomCall:    false,
			expectTerraformCall: true,
		},
		{
			name:                "empty type defaults to terraform (back-compat)",
			componentType:       "",
			expectCustomCall:    false,
			expectTerraformCall: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			customCalled := false
			terraformCalled := false

			cmd := &StoreCommand{
				atmosConfig: &schema.AtmosConfiguration{},
				info: &schema.ConfigAndStacksInfo{
					ComponentType:    tc.componentType,
					ComponentFromArg: "test-component",
					Stack:            "test-stack",
					OutputsFilePath:  "/unused-during-this-test",
				},
				terraformOutputter: func(_ *schema.AtmosConfiguration, _ string, _ string, _ string, _ bool, _ *schema.AuthContext, _ any) (any, bool, error) {
					terraformCalled = true
					return "terraform-value", true, nil
				},
				customOutputter: func(_ *schema.ConfigAndStacksInfo, _ string) (any, bool, error) {
					customCalled = true
					return "custom-value", true, nil
				},
			}

			_, _, err := cmd.getOutputValue("test-hook", AfterTerraformApply, ".some_key")
			require.NoError(t, err)
			assert.Equal(t, tc.expectCustomCall, customCalled, "custom outputter call expectation")
			assert.Equal(t, tc.expectTerraformCall, terraformCalled, "terraform outputter call expectation")
		})
	}
}

// TestStoreCommand_CustomOutputFromFile verifies the end-to-end flow where the
// custom outputter reads the ATMOS_OUTPUTS file and surfaces values.
func TestStoreCommand_CustomOutputFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outputs.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"agent_id": "agent_abc"}`), 0o600))

	cmd := &StoreCommand{
		atmosConfig: &schema.AtmosConfiguration{},
		info: &schema.ConfigAndStacksInfo{
			ComponentType:    "agent",
			ComponentFromArg: "slack-knowledge",
			Stack:            "dev",
			OutputsFilePath:  path,
		},
		// Use the production custom outputter (the one wired by NewStoreCommand).
		customOutputter: defaultCustomOutputter,
	}

	key, val, err := cmd.getOutputValue("test-hook", AfterTerraformApply, ".agent_id")
	require.NoError(t, err)
	assert.Equal(t, "agent_id", key)
	assert.Equal(t, "agent_abc", val)
}

// TestStoreCommand_CustomOutputReadError verifies that a failure reading the
// outputs file (here, OutputsFilePath points at a directory) propagates out of
// the custom outputter instead of being masked as a missing key.
func TestStoreCommand_CustomOutputReadError(t *testing.T) {
	dir := t.TempDir() // a directory, not a file — os.ReadFile returns EISDIR

	cmd := &StoreCommand{
		atmosConfig: &schema.AtmosConfiguration{},
		info: &schema.ConfigAndStacksInfo{
			ComponentType:    "agent",
			ComponentFromArg: "slack-knowledge",
			Stack:            "dev",
			OutputsFilePath:  dir,
		},
		customOutputter: defaultCustomOutputter,
	}

	_, _, err := cmd.getOutputValue("test-hook", AfterTerraformApply, ".agent_id")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrReadOutputsFile,
		"read failure should propagate, not be reported as a missing key")
}

// TestStoreCommand_CustomOutputMissing surfaces a clear error when the apply
// step never wrote the referenced key to the outputs file.
func TestStoreCommand_CustomOutputMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outputs.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"agent_id": "agent_abc"}`), 0o600))

	cmd := &StoreCommand{
		atmosConfig: &schema.AtmosConfiguration{},
		info: &schema.ConfigAndStacksInfo{
			ComponentType:    "agent",
			ComponentFromArg: "slack-knowledge",
			Stack:            "dev",
			OutputsFilePath:  path,
		},
		customOutputter: defaultCustomOutputter,
	}

	_, _, err := cmd.getOutputValue("test-hook", AfterTerraformApply, ".missing_key")
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrCustomOutputMissing),
		"expected ErrCustomOutputMissing, got %v", err)
}
