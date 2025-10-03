package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestProcessHelp(t *testing.T) {
	tests := []struct {
		name          string
		componentType string
		command       string
		wantErr       bool
	}{
		{
			name:          "terraform with empty command",
			componentType: "terraform",
			command:       "",
			wantErr:       false,
		},
		{
			name:          "helmfile with empty command",
			componentType: "helmfile",
			command:       "",
			wantErr:       false,
		},
		{
			name:          "terraform with subcommand",
			componentType: "terraform",
			command:       "plan",
			wantErr:       false,
		},
		{
			name:          "helmfile with subcommand",
			componentType: "helmfile",
			command:       "apply",
			wantErr:       false,
		},
		{
			name:          "other component type",
			componentType: "packer",
			command:       "",
			wantErr:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := schema.AtmosConfiguration{}
			err := processHelp(atmosConfig, tt.componentType, tt.command)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
