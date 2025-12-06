package terraform

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestParseVarfileFlags(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*viper.Viper)
		expected VarfileConfig
	}{
		{
			name: "all flags set",
			setup: func(v *viper.Viper) {
				v.Set(flagStack, "dev")
				v.Set(flagFile, "output.tfvars")
				v.Set(flagProcessTemplates, true)
				v.Set(flagProcessFunctions, false)
				v.Set(flagSkip, []string{"func1", "func2"})
			},
			expected: VarfileConfig{
				Stack:            "dev",
				File:             "output.tfvars",
				ProcessTemplates: true,
				ProcessFunctions: false,
				Skip:             []string{"func1", "func2"},
			},
		},
		{
			name: "empty values",
			setup: func(v *viper.Viper) {
				// Don't set anything
			},
			expected: VarfileConfig{
				Stack:            "",
				File:             "",
				ProcessTemplates: false,
				ProcessFunctions: false,
				Skip:             nil,
			},
		},
		{
			name: "only stack set",
			setup: func(v *viper.Viper) {
				v.Set(flagStack, "prod")
			},
			expected: VarfileConfig{
				Stack:            "prod",
				File:             "",
				ProcessTemplates: false,
				ProcessFunctions: false,
				Skip:             nil,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := viper.New()
			tt.setup(v)

			result := ParseVarfileFlags(v)

			assert.Equal(t, tt.expected.Stack, result.Stack, "Stack should match")
			assert.Equal(t, tt.expected.File, result.File, "File should match")
			assert.Equal(t, tt.expected.ProcessTemplates, result.ProcessTemplates, "ProcessTemplates should match")
			assert.Equal(t, tt.expected.ProcessFunctions, result.ProcessFunctions, "ProcessFunctions should match")
			assert.Equal(t, tt.expected.Skip, result.Skip, "Skip should match")
		})
	}
}

func TestValidateVarfileConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      *VarfileConfig
		expectedErr error
	}{
		{
			name: "valid config with all fields",
			config: &VarfileConfig{
				Component:        "vpc",
				Stack:            "dev",
				File:             "output.tfvars",
				ProcessTemplates: true,
				ProcessFunctions: true,
				Skip:             []string{},
			},
			expectedErr: nil,
		},
		{
			name: "valid config with minimal fields",
			config: &VarfileConfig{
				Component: "vpc",
				Stack:     "dev",
			},
			expectedErr: nil,
		},
		{
			name: "missing stack",
			config: &VarfileConfig{
				Component: "vpc",
				Stack:     "",
			},
			expectedErr: errUtils.ErrMissingStack,
		},
		{
			name: "empty config",
			config: &VarfileConfig{
				Component: "",
				Stack:     "",
			},
			expectedErr: errUtils.ErrMissingStack,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateVarfileConfig(tt.config)

			if tt.expectedErr != nil {
				assert.ErrorIs(t, err, tt.expectedErr)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateVarfileParser(t *testing.T) {
	parser := createVarfileParser()

	assert.NotNil(t, parser, "Parser should not be nil")
}
