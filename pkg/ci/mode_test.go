package ci

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestReportingGates(t *testing.T) {
	boolPtr := func(value bool) *bool { return &value }
	tests := []struct {
		name                          string
		config                        *schema.AtmosConfiguration
		enabled, annotations, results bool
	}{
		{name: "nil config"},
		{name: "defaults", config: &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}, enabled: true, annotations: true},
		{name: "explicit settings", config: &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true, Annotations: schema.CIAnnotationsConfig{Enabled: boolPtr(false)}, Results: schema.CIResultsConfig{Enabled: boolPtr(true)}}}, enabled: true, results: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.enabled, Enabled(tt.config))
			assert.Equal(t, tt.annotations, AnnotationsEnabled(tt.config))
			assert.Equal(t, tt.results, ResultsEnabled(tt.config))
		})
	}
}

func TestModeEnabledFromFlag(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().Bool("ci", false, "")
	assert.False(t, ModeEnabled(cmd))
	assert.NoError(t, cmd.Flags().Set("ci", "true"))
	assert.True(t, ModeEnabled(cmd))
}
