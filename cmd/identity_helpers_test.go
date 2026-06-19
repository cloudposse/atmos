package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestExtractIdentityFromArgs(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "long flag with equals",
			args: []string{"atmos", "describe", "component", "--identity=admin"},
			want: "admin",
		},
		{
			name: "long flag with empty equals selects interactively",
			args: []string{"atmos", "describe", "component", "--identity="},
			want: cfg.IdentityFlagSelectValue,
		},
		{
			name: "short flag with equals",
			args: []string{"atmos", "describe", "component", "-i=readonly"},
			want: "readonly",
		},
		{
			name: "short flag with empty equals selects interactively",
			args: []string{"atmos", "describe", "component", "-i="},
			want: cfg.IdentityFlagSelectValue,
		},
		{
			name: "long flag with separate value",
			args: []string{"atmos", "describe", "component", "--identity", "admin"},
			want: "admin",
		},
		{
			name: "short flag with separate value",
			args: []string{"atmos", "describe", "component", "-i", "admin"},
			want: "admin",
		},
		{
			name: "long flag without value selects interactively",
			args: []string{"atmos", "describe", "component", "--identity"},
			want: cfg.IdentityFlagSelectValue,
		},
		{
			name: "short flag followed by another flag selects interactively",
			args: []string{"atmos", "describe", "component", "-i", "--stack", "dev"},
			want: cfg.IdentityFlagSelectValue,
		},
		{
			name: "flag not present",
			args: []string{"atmos", "describe", "component"},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, extractIdentityFromArgs(tt.args))
		})
	}
}
