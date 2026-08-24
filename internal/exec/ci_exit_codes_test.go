package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestMapCIExitCode(t *testing.T) {
	tests := []struct {
		name     string
		ci       schema.CIConfig
		tf       schema.TerraformCI
		exitCode int
		expected int
	}{
		{
			name:     "CI disabled preserves exit code",
			ci:       schema.CIConfig{Enabled: false},
			tf:       schema.TerraformCI{ExitCodes: map[int]bool{2: true}},
			exitCode: 2,
			expected: 2,
		},
		{
			name:     "CI enabled but no exit_codes map uses defaults (exit 2 → success)",
			ci:       schema.CIConfig{Enabled: true},
			tf:       schema.TerraformCI{},
			exitCode: 2,
			expected: 0,
		},
		{
			name:     "CI enabled but no exit_codes map uses defaults (exit 1 → failure)",
			ci:       schema.CIConfig{Enabled: true},
			tf:       schema.TerraformCI{},
			exitCode: 1,
			expected: 1,
		},
		{
			name:     "CI enabled with code mapped true returns 0",
			ci:       schema.CIConfig{Enabled: true},
			tf:       schema.TerraformCI{ExitCodes: map[int]bool{0: true, 2: true}},
			exitCode: 2,
			expected: 0,
		},
		{
			name:     "CI enabled with code mapped false preserves exit code",
			ci:       schema.CIConfig{Enabled: true},
			tf:       schema.TerraformCI{ExitCodes: map[int]bool{1: false}},
			exitCode: 1,
			expected: 1,
		},
		{
			name:     "CI enabled with unmapped code preserves exit code",
			ci:       schema.CIConfig{Enabled: true},
			tf:       schema.TerraformCI{ExitCodes: map[int]bool{0: true, 2: true}},
			exitCode: 1,
			expected: 1,
		},
		{
			name:     "CI enabled with exit 0 mapped true returns 0",
			ci:       schema.CIConfig{Enabled: true},
			tf:       schema.TerraformCI{ExitCodes: map[int]bool{0: true}},
			exitCode: 0,
			expected: 0,
		},
		{
			name:     "exit code 0 without mapping preserves 0",
			ci:       schema.CIConfig{Enabled: true},
			tf:       schema.TerraformCI{ExitCodes: map[int]bool{2: true}},
			exitCode: 0,
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{
				CI: tt.ci,
				Components: schema.Components{
					Terraform: schema.Terraform{
						CI: tt.tf,
					},
				},
			}
			result := mapCIExitCode(config, tt.exitCode)
			assert.Equal(t, tt.expected, result)
		})
	}
}
