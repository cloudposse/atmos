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

func TestFeatureGates(t *testing.T) {
	boolPtr := func(value bool) *bool { return &value }

	// gate describes one feature gate: its accessor, how to set its flag, and its nil default.
	type gate struct {
		name       string
		fn         func(*schema.AtmosConfiguration) bool
		set        func(*schema.CIConfig, *bool)
		defaultVal bool
	}
	gates := []gate{
		{name: "summary", fn: SummaryEnabled, set: func(c *schema.CIConfig, v *bool) { c.Summary.Enabled = v }, defaultVal: true},
		{name: "output", fn: OutputEnabled, set: func(c *schema.CIConfig, v *bool) { c.Output.Enabled = v }, defaultVal: true},
		{name: "checks", fn: ChecksEnabled, set: func(c *schema.CIConfig, v *bool) { c.Checks.Enabled = v }, defaultVal: false},
		{name: "comments", fn: CommentsEnabled, set: func(c *schema.CIConfig, v *bool) { c.Comments.Enabled = v }, defaultVal: false},
	}

	for _, g := range gates {
		t.Run(g.name, func(t *testing.T) {
			build := func(master bool, flag *bool) *schema.AtmosConfiguration {
				cfg := &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: master}}
				g.set(&cfg.CI, flag)
				return cfg
			}
			tests := []struct {
				name string
				cfg  *schema.AtmosConfiguration
				want bool
			}{
				{name: "nil config", cfg: nil, want: false},
				{name: "master switch off, flag true", cfg: build(false, boolPtr(true)), want: false},
				{name: "master switch off, flag unset", cfg: build(false, nil), want: false},
				{name: "flag unset uses default", cfg: build(true, nil), want: g.defaultVal},
				{name: "flag true", cfg: build(true, boolPtr(true)), want: true},
				{name: "flag false", cfg: build(true, boolPtr(false)), want: false},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					assert.Equal(t, tt.want, g.fn(tt.cfg))
				})
			}
		})
	}
}
