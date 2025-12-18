package vendor

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePullOptions(t *testing.T) {
	tests := []struct {
		name           string
		viperSetup     func(*viper.Viper)
		expectedOpts   *PullOptions
	}{
		{
			name: "default values",
			viperSetup: func(v *viper.Viper) {
				// Set defaults.
			},
			expectedOpts: &PullOptions{
				Component:     "",
				Stack:         "",
				Tags:          nil,
				DryRun:        false,
				Everything:    false,
				ComponentType: "",
			},
		},
		{
			name: "component set",
			viperSetup: func(v *viper.Viper) {
				v.Set("component", "vpc")
			},
			expectedOpts: &PullOptions{
				Component:     "vpc",
				Stack:         "",
				Tags:          nil,
				DryRun:        false,
				Everything:    false,
				ComponentType: "",
			},
		},
		{
			name: "stack set",
			viperSetup: func(v *viper.Viper) {
				v.Set("stack", "dev-us-west-2")
			},
			expectedOpts: &PullOptions{
				Component:     "",
				Stack:         "dev-us-west-2",
				Tags:          nil,
				DryRun:        false,
				Everything:    false,
				ComponentType: "",
			},
		},
		{
			name: "tags as CSV",
			viperSetup: func(v *viper.Viper) {
				v.Set("tags", "networking,database")
			},
			expectedOpts: &PullOptions{
				Component:     "",
				Stack:         "",
				Tags:          []string{"networking", "database"},
				DryRun:        false,
				Everything:    false,
				ComponentType: "",
			},
		},
		{
			name: "dry-run enabled",
			viperSetup: func(v *viper.Viper) {
				v.Set("dry-run", true)
			},
			expectedOpts: &PullOptions{
				Component:     "",
				Stack:         "",
				Tags:          nil,
				DryRun:        true,
				Everything:    false,
				ComponentType: "",
			},
		},
		{
			name: "everything enabled",
			viperSetup: func(v *viper.Viper) {
				v.Set("everything", true)
			},
			expectedOpts: &PullOptions{
				Component:     "",
				Stack:         "",
				Tags:          nil,
				DryRun:        false,
				Everything:    true,
				ComponentType: "",
			},
		},
		{
			name: "component type set",
			viperSetup: func(v *viper.Viper) {
				v.Set("type", "helmfile")
			},
			expectedOpts: &PullOptions{
				Component:     "",
				Stack:         "",
				Tags:          nil,
				DryRun:        false,
				Everything:    false,
				ComponentType: "helmfile",
			},
		},
		{
			name: "all options set",
			viperSetup: func(v *viper.Viper) {
				v.Set("component", "vpc")
				v.Set("tags", "core,networking")
				v.Set("dry-run", true)
				v.Set("type", "terraform")
			},
			expectedOpts: &PullOptions{
				Component:     "vpc",
				Stack:         "",
				Tags:          []string{"core", "networking"},
				DryRun:        true,
				Everything:    false,
				ComponentType: "terraform",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a new Viper instance for each test.
			v := viper.New()
			tt.viperSetup(v)

			// Create a dummy cobra command for testing.
			cmd := &cobra.Command{}

			opts, err := parsePullOptions(cmd, v, nil)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedOpts.Component, opts.Component)
			assert.Equal(t, tt.expectedOpts.Stack, opts.Stack)
			assert.Equal(t, tt.expectedOpts.Tags, opts.Tags)
			assert.Equal(t, tt.expectedOpts.DryRun, opts.DryRun)
			assert.Equal(t, tt.expectedOpts.Everything, opts.Everything)
			assert.Equal(t, tt.expectedOpts.ComponentType, opts.ComponentType)
		})
	}
}

func TestParsePullOptions_EmptyTags(t *testing.T) {
	v := viper.New()
	v.Set("tags", "")

	cmd := &cobra.Command{}
	opts, err := parsePullOptions(cmd, v, nil)

	require.NoError(t, err)
	assert.Nil(t, opts.Tags)
}

func TestParsePullOptions_SingleTag(t *testing.T) {
	v := viper.New()
	v.Set("tags", "networking")

	cmd := &cobra.Command{}
	opts, err := parsePullOptions(cmd, v, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"networking"}, opts.Tags)
}
