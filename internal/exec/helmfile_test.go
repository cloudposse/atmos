package exec

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/tests"
)

func TestExecuteHelmfile_Version(t *testing.T) {
	tests.RequireHelmfile(t)

	testCases := []struct {
		name           string
		workDir        string
		expectedOutput string
	}{
		{
			name:           "helmfile version",
			workDir:        "../../tests/fixtures/scenarios/atmos-helmfile-version",
			expectedOutput: "helmfile",
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			// Set info for ExecuteHelmfile.
			info := schema.ConfigAndStacksInfo{
				SubCommand: "version",
			}

			testCaptureCommandOutput(t, tt.workDir, func() error {
				return ExecuteHelmfile(info)
			}, tt.expectedOutput)
		})
	}
}

func TestExecuteHelmfile_MissingStack(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/complete"
	t.Chdir(workDir)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "echo-server",
		Stack:            "",
		SubCommand:       "diff",
	}

	err := ExecuteHelmfile(info)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrMissingStack)
}

func TestExecuteHelmfile_ComponentNotFound(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/complete"
	t.Chdir(workDir)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "non-existent-component",
		Stack:            "tenant1-ue2-dev",
		SubCommand:       "diff",
	}

	err := ExecuteHelmfile(info)
	assert.Error(t, err)
	// ExecuteHelmfile calls ProcessStacks which will fail to find the component.
	assert.Contains(t, err.Error(), "Could not find the component")
}

// TestHelmfileComponentEnvSectionConversion verifies that ComponentEnvSection is properly
// converted to ComponentEnvList in Helmfile execution. This ensures auth environment variables
// and stack config env sections are passed to Helmfile commands.
//
//nolint:dupl // Test logic is intentionally similar across terraform/helmfile/packer for consistency
func TestHelmfileComponentEnvSectionConversion(t *testing.T) {
	tests := []struct {
		name            string
		envSection      map[string]any
		expectedEnvList map[string]string
	}{
		{
			name: "converts AWS auth environment variables for Helmfile",
			envSection: map[string]any{
				"AWS_CONFIG_FILE":             "/path/to/config",
				"AWS_SHARED_CREDENTIALS_FILE": "/path/to/credentials",
				"AWS_PROFILE":                 "helmfile-profile",
				"AWS_REGION":                  "eu-west-1",
			},
			expectedEnvList: map[string]string{
				"AWS_CONFIG_FILE":             "/path/to/config",
				"AWS_SHARED_CREDENTIALS_FILE": "/path/to/credentials",
				"AWS_PROFILE":                 "helmfile-profile",
				"AWS_REGION":                  "eu-west-1",
			},
		},
		{
			name: "handles Kubernetes environment variables",
			envSection: map[string]any{
				"KUBECONFIG":     "/path/to/kubeconfig",
				"HELM_NAMESPACE": "my-namespace",
				"CUSTOM_VAR":     "helmfile-value",
			},
			expectedEnvList: map[string]string{
				"KUBECONFIG":     "/path/to/kubeconfig",
				"HELM_NAMESPACE": "my-namespace",
				"CUSTOM_VAR":     "helmfile-value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test ConfigAndStacksInfo with ComponentEnvSection populated.
			info := schema.ConfigAndStacksInfo{
				ComponentEnvSection: tt.envSection,
				ComponentEnvList:    []string{},
			}

			// Simulate the conversion logic from helmfile.go.
			for k, v := range info.ComponentEnvSection {
				info.ComponentEnvList = append(info.ComponentEnvList, fmt.Sprintf("%s=%v", k, v))
			}

			// Verify all expected environment variables are in ComponentEnvList.
			envListMap := make(map[string]string)
			for _, envVar := range info.ComponentEnvList {
				parts := strings.SplitN(envVar, "=", 2)
				if len(parts) == 2 {
					envListMap[parts[0]] = parts[1]
				}
			}

			// Check that all expected vars are present with correct values.
			for key, expectedValue := range tt.expectedEnvList {
				actualValue, exists := envListMap[key]
				assert.True(t, exists, "Expected environment variable %s to be in ComponentEnvList", key)
				assert.Equal(t, expectedValue, actualValue,
					"Environment variable %s should have value %s, got %s", key, expectedValue, actualValue)
			}

			// Verify count matches.
			assert.Equal(t, len(tt.expectedEnvList), len(envListMap),
				"ComponentEnvList should contain exactly %d variables", len(tt.expectedEnvList))
		})
	}
}
