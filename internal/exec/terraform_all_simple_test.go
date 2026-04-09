package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecuteTerraformAll_ValidationSimple(t *testing.T) {
	tests := []struct {
		name        string
		info        *schema.ConfigAndStacksInfo
		expectError bool
		errorMsg    string
	}{
		{
			name: "no stack specified",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "",
				Stack:            "",
				SubCommand:       "plan",
			},
			expectError: true,
			errorMsg:    "stack is required",
		},
		{
			name: "component specified with --all",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
				Stack:            "dev",
				SubCommand:       "plan",
			},
			expectError: true,
			errorMsg:    "component argument can't be used",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ExecuteTerraformAll(tt.info)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorMsg != "" {
					assert.Contains(t, err.Error(), tt.errorMsg)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
