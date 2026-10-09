package cloudformation

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
)

// TestSelectorFlagsIgnoreTerraformEnvironment verifies that Terraform selectors
// cannot select CloudFormation components, while explicit CLI selectors still work.
func TestSelectorFlagsIgnoreTerraformEnvironment(t *testing.T) {
	t.Setenv("ATMOS_TAGS", "terraform-only")
	t.Setenv("ATMOS_LABELS", "owner=terraform")
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.SetEnvPrefix("ATMOS")
	viper.AutomaticEnv()
	assert.Equal(t, "terraform-only", viper.GetString("tags"))
	assert.Equal(t, "owner=terraform", viper.GetString("labels"))

	for _, tt := range []struct {
		name   string
		flags  map[string]string
		tags   []string
		labels map[string]string
	}{
		{name: "no selectors"},
		{
			name:   "explicit selectors",
			flags:  map[string]string{"tags": "production,tier-1", "labels": "owner=cloudformation"},
			tags:   []string{"production", "tier-1"},
			labels: map[string]string{"owner": "cloudformation"},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			cmd := configuredOperationCommand(t, "apply", tt.flags)
			info := buildConfigAndStacksInfo(cmd)
			if tt.flags == nil {
				assert.Empty(t, info.Tags)
				assert.Empty(t, info.Labels)
				assert.False(t, hasSelectionFlags(cmd))
				return
			}
			assert.Equal(t, tt.tags, info.Tags)
			assert.Equal(t, tt.labels, info.Labels)
			assert.True(t, hasSelectionFlags(cmd))
		})
	}
}
