package exec

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/stretchr/testify/assert"
)

func TestBuildTerraformWorkspace(t *testing.T) {
	tests := []struct {
		name              string
		backendType       string
		workspacesEnabled *bool
		stack             string
		expectedWorkspace string
		shouldReturnError bool
	}{
		{
			name:              "Default behavior (workspaces enabled, non-HTTP backend)",
			backendType:       "s3",
			workspacesEnabled: nil,
			stack:             "dev/us-east-1",
			expectedWorkspace: "dev-us-east-1",
			shouldReturnError: false,
		},
		{
			name:              "HTTP backend automatically disables workspaces",
			backendType:       "http",
			workspacesEnabled: nil,
			stack:             "dev/us-east-1",
			expectedWorkspace: "default",
			shouldReturnError: false,
		},
		{
			name:              "Explicitly disabled workspaces",
			backendType:       "s3",
			workspacesEnabled: boolPtr(false),
			stack:             "dev/us-east-1",
			expectedWorkspace: "default",
			shouldReturnError: false,
		},
		{
			name:              "Explicitly enabled workspaces",
			backendType:       "s3",
			workspacesEnabled: boolPtr(true),
			stack:             "dev/us-east-1",
			expectedWorkspace: "dev-us-east-1",
			shouldReturnError: false,
		},
		{
			name:              "HTTP backend with explicitly enabled workspaces",
			backendType:       "http",
			workspacesEnabled: boolPtr(true),
			stack:             "dev/us-east-1",
			expectedWorkspace: "default",
			shouldReturnError: false,
		},
		{
			name:              "Empty stack name",
			backendType:       "s3",
			workspacesEnabled: nil,
			stack:             "",
			expectedWorkspace: "",
			shouldReturnError: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup test config.
			atmosConfig := schema.AtmosConfiguration{
				Components: schema.Components{
					Terraform: schema.Terraform{
						WorkspacesEnabled: tc.workspacesEnabled,
					},
				},
			}

			info := schema.ConfigAndStacksInfo{
				ComponentBackendType: tc.backendType,
				Component:            "test-component",
				Stack:                tc.stack,
			}

			// Test function.
			workspace, err := BuildTerraformWorkspace(atmosConfig, info)

			// Assert results.
			if tc.shouldReturnError {
				assert.Error(t, err, "Expected error for case: %s", tc.name)
			} else {
				assert.NoError(t, err, "Did not expect error for case: %s", tc.name)
				assert.Equal(t, tc.expectedWorkspace, workspace, "Expected workspace to match for case: %s", tc.name)
			}
		})
	}
}
