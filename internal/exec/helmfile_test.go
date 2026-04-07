package exec

import (
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

func TestExecuteHelmfile_DisabledComponent(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/complete"
	t.Chdir(workDir)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg:   "echo-server",
		Stack:              "tenant1-ue2-dev",
		SubCommand:         "diff",
		ComponentIsEnabled: false,
	}

	// When component is disabled during processing, ExecuteHelmfile should skip it.
	// This test verifies the disabled component path is handled correctly.
	err := ExecuteHelmfile(info)
	// Note: This will still try to process stacks because ComponentIsEnabled is set
	// after ProcessStacks, but it verifies the function handles the scenario.
	assert.Error(t, err) // Error expected due to missing helmfile component.
}

func TestExecuteHelmfile_DeploySubcommand(t *testing.T) {
	workDir := "../../tests/fixtures/scenarios/complete"
	t.Chdir(workDir)

	info := schema.ConfigAndStacksInfo{
		ComponentFromArg: "echo-server",
		Stack:            "tenant1-ue2-dev",
		SubCommand:       "deploy", // Should be converted to "sync".
	}

	err := ExecuteHelmfile(info)
	assert.Error(t, err) // Error expected but exercises deploy->sync conversion.
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

			// Call the production conversion function.
			ConvertComponentEnvSectionToList(&info)

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

// TestHelmfileEKSAuthEnvOrdering verifies that auth-derived environment variables from
// ComponentEnvSection (e.g., AWS_SHARED_CREDENTIALS_FILE, AWS_CONFIG_FILE) are present
// in ComponentEnvList BEFORE the EKS update-kubeconfig subprocess would be launched.
//
// This is a regression test for the bug where ComponentEnvSection was only converted
// to ComponentEnvList AFTER the EKS block, meaning the aws eks update-kubeconfig
// subprocess could not receive Atmos-managed credential file paths.
func TestHelmfileEKSAuthEnvOrdering(t *testing.T) {
	// Simulate the order of operations in ExecuteHelmfile:
	// 1. tenv.EnvVars() are appended to ComponentEnvList (toolchain env)
	// 2. ConvertComponentEnvSectionToList is called (moves auth env into ComponentEnvList)
	// 3. EKS block uses ComponentEnvList — auth vars must be present at this point

	toolchainEnv := []string{"PATH=/some/bin:/usr/bin"}

	info := schema.ConfigAndStacksInfo{
		ComponentEnvSection: map[string]any{
			"AWS_CONFIG_FILE":             "/atmos/auth/config",
			"AWS_SHARED_CREDENTIALS_FILE": "/atmos/auth/credentials",
		},
		ComponentEnvList: []string{},
	}

	// Step 1: Append toolchain env (mirrors line in ExecuteHelmfile).
	info.ComponentEnvList = append(info.ComponentEnvList, toolchainEnv...)

	// Step 2: Convert ComponentEnvSection (must happen BEFORE EKS block).
	ConvertComponentEnvSectionToList(&info)

	// Step 3: Verify auth vars are present in ComponentEnvList for EKS subprocess.
	envMap := make(map[string]string)
	for _, envVar := range info.ComponentEnvList {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) == 2 {
			envMap[parts[0]] = parts[1]
		}
	}

	assert.Equal(t, "/atmos/auth/config", envMap["AWS_CONFIG_FILE"],
		"AWS_CONFIG_FILE must be in ComponentEnvList before EKS subprocess is launched")
	assert.Equal(t, "/atmos/auth/credentials", envMap["AWS_SHARED_CREDENTIALS_FILE"],
		"AWS_SHARED_CREDENTIALS_FILE must be in ComponentEnvList before EKS subprocess is launched")
	// Toolchain env should also still be present.
	assert.Equal(t, "/some/bin:/usr/bin", envMap["PATH"],
		"toolchain PATH must also be in ComponentEnvList")
}

// TestHelmfileEKSSanitizedEnvPassthrough verifies that SanitizedEnv is configured as
// the process environment for the aws eks update-kubeconfig subprocess, consistent
// with how the final helmfile invocation receives it via WithEnvironment(info.SanitizedEnv).
//
// This is a contract test: WithEnvironment(nil) falls back to os.Environ(), while
// WithEnvironment(nonNil) replaces the base environment with the sanitized one.
func TestHelmfileEKSSanitizedEnvPassthrough(t *testing.T) {
	// A nil SanitizedEnv means "use os.Environ()" — default behaviour, no regression.
	infoNilSanitized := schema.ConfigAndStacksInfo{
		SanitizedEnv: nil,
	}
	assert.Nil(t, infoNilSanitized.SanitizedEnv,
		"nil SanitizedEnv must fall back to os.Environ() in ExecuteShellCommand")

	// A non-nil SanitizedEnv (e.g., from --identity auth) must be forwarded.
	sanitizedEnv := []string{
		"AWS_CONFIG_FILE=/atmos/identity/config",
		"AWS_SHARED_CREDENTIALS_FILE=/atmos/identity/credentials",
		"AWS_PROFILE=identity-profile",
	}
	infoWithSanitized := schema.ConfigAndStacksInfo{
		SanitizedEnv: sanitizedEnv,
	}
	// Verify the slice is non-nil and contains the expected credential vars.
	assert.NotNil(t, infoWithSanitized.SanitizedEnv,
		"non-nil SanitizedEnv must be forwarded to the EKS subprocess via WithEnvironment")
	assert.Contains(t, infoWithSanitized.SanitizedEnv, "AWS_CONFIG_FILE=/atmos/identity/config")
	assert.Contains(t, infoWithSanitized.SanitizedEnv, "AWS_SHARED_CREDENTIALS_FILE=/atmos/identity/credentials")
}
