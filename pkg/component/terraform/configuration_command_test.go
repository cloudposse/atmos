package terraform

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestConfigurationCommandPrecedence(t *testing.T) {
	for _, tc := range []struct{ name, global, stack, flag, want string }{
		{"default", "", "", "", "terraform"},
		{"global", "tofu", "", "", "tofu"},
		{"component", "tofu", "terraform", "", "terraform"},
		{"inherited command", "tofu", "/inherited/terraform", "", "/inherited/terraform"},
		{"override command", "tofu", "/overrides/terraform", "", "/overrides/terraform"},
		{"explicit flag wins over global and stack", "tofu", "/stack/tofu", "/cli/terraform", "/cli/terraform"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &schema.AtmosConfiguration{}
			config.Components.Terraform.Command = tc.global
			info := &schema.ConfigAndStacksInfo{Command: tc.stack, ComponentCommand: tc.flag}
			resolveTerraformCommand(config, info)
			require.Equal(t, tc.want, info.Command)
			require.Equal(t, tc.flag, info.ComponentCommand, "explicit flag provenance survives resolution")
		})
	}
}
