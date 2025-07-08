package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCheckTerraformFlags(t *testing.T) {
	tests := []struct {
		name          string
		info          *schema.ConfigAndStacksInfo
		expectedError error
	}{
		{
			name:          "valid - no flags",
			info:          &schema.ConfigAndStacksInfo{},
			expectedError: nil,
		},
		{
			name: "invalid - component with affected flag",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "test-component",
				Affected:         true,
			},
			expectedError: errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags,
		},
		{
			name: "invalid - component with all flag",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "test-component",
				All:              true,
			},
			expectedError: errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags,
		},
		{
			name: "invalid - component with components flag",
			info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "test-component",
				Components:       []string{"comp1", "comp2"},
			},
			expectedError: errUtils.ErrInvalidTerraformComponentWithMultiComponentFlags,
		},
		{
			name: "invalid - affected with all flag",
			info: &schema.ConfigAndStacksInfo{
				Affected: true,
				All:      true,
			},
			expectedError: errUtils.ErrInvalidTerraformFlagsWithAffectedFlag,
		},
		{
			name: "invalid - affected with components flag",
			info: &schema.ConfigAndStacksInfo{
				Affected:   true,
				Components: []string{"comp1", "comp2"},
			},
			expectedError: errUtils.ErrInvalidTerraformFlagsWithAffectedFlag,
		},
		{
			name: "invalid - affected with query flag",
			info: &schema.ConfigAndStacksInfo{
				Affected: true,
				Query:    "test-query",
			},
			expectedError: errUtils.ErrInvalidTerraformFlagsWithAffectedFlag,
		},
		{
			name: "invalid - single and multi component flags",
			info: &schema.ConfigAndStacksInfo{
				PlanFile: "plan.tfplan",
				All:      true,
			},
			expectedError: errUtils.ErrInvalidTerraformSingleComponentAndMultiComponentFlags,
		},
		{
			name: "invalid - from-plan with multi component flag",
			info: &schema.ConfigAndStacksInfo{
				UseTerraformPlan: true,
				Affected:         true,
			},
			expectedError: errUtils.ErrInvalidTerraformSingleComponentAndMultiComponentFlags,
		},
		{
			name: "valid - only single component flag",
			info: &schema.ConfigAndStacksInfo{
				PlanFile: "plan.tfplan",
			},
			expectedError: nil,
		},
		{
			name: "valid - only multi component flag",
			info: &schema.ConfigAndStacksInfo{
				All: true,
			},
			expectedError: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := checkTerraformFlags(test.info)
			if test.expectedError != nil {
				assert.ErrorIs(t, err, test.expectedError)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
