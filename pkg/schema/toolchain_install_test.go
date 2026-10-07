package schema

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestToolchainInstallIsValid(t *testing.T) {
	tests := []struct {
		policy ToolchainInstall
		want   bool
	}{
		{policy: "", want: true},
		{policy: ToolchainInstallNever, want: true},
		{policy: ToolchainInstallDeclared, want: true},
		{policy: ToolchainInstallAuto, want: true},
		{policy: ToolchainInstallAlways, want: true},
		{policy: "Auto", want: false},
		{policy: "sometimes", want: false},
	}
	for _, tt := range tests {
		t.Run(string(tt.policy), func(t *testing.T) {
			assert.Equal(t, tt.want, tt.policy.IsValid())
		})
	}
}

func TestToolchainInstallValuesAreAllValid(t *testing.T) {
	assert.Equal(t, []ToolchainInstall{"never", "declared", "auto", "always"}, ToolchainInstallValues)
	for _, policy := range ToolchainInstallValues {
		assert.True(t, policy.IsValid(), policy)
	}
}

func TestToolchainEffectiveInstall(t *testing.T) {
	assert.Equal(t, ToolchainInstallAuto, (&Toolchain{}).EffectiveInstall())
	assert.Equal(t, ToolchainInstallNever, (&Toolchain{Install: ToolchainInstallNever}).EffectiveInstall())
	assert.Equal(t, ToolchainInstallDeclared, (&Toolchain{Install: ToolchainInstallDeclared}).EffectiveInstall())
}
