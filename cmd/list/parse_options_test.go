package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
)

// setFlag sets a cobra flag and fails the test if the flag cannot be set.
func setFlag(t *testing.T, cmd *cobra.Command, name, value string) {
	t.Helper()
	require.NoError(t, cmd.Flags().Set(name, value), "setting %s", name)
}

// bindFlagsToViper returns a fresh viper.Viper bound to `cmd`'s flags via
// the same `parser.BindFlagsToViper` helper production RunE closures use,
// so the parse* helpers see the real binding semantics (env-var aliases,
// prefix handling, etc.) and tests are isolated from global viper state.
//
// Calling `parser.BindFlagsToViper` directly also respects the repo rule
// that commands never touch `viper.BindPFlag` directly outside of
// `pkg/flags/`.
func bindFlagsToViper(t *testing.T, cmd *cobra.Command, parser *flags.StandardParser) *viper.Viper {
	t.Helper()
	v := viper.New()
	require.NoError(t, parser.BindFlagsToViper(cmd, v), "binding flags to viper")
	return v
}

// TestParseComponentsOptions verifies the viper→options mapping for the
// `list components` RunE closure. Exercises defaults, explicit overrides,
// and the tri-state enabled/locked semantics.
func TestParseComponentsOptions(t *testing.T) {
	buildCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "components"}
		componentsParser.RegisterFlags(cmd)
		return cmd
	}

	t.Run("defaults", func(t *testing.T) {
		cmd := buildCmd()
		v := bindFlagsToViper(t, cmd, componentsParser)

		opts := parseComponentsOptions(cmd, v)

		assert.Equal(t, "", opts.Format)
		assert.False(t, opts.Abstract)
		assert.Nil(t, opts.Enabled, "enabled should be nil when flag was not changed")
		assert.Nil(t, opts.Locked, "locked should be nil when flag was not changed")
		assert.True(t, opts.ProcessTemplates)
		assert.True(t, opts.ProcessFunctions)
	})

	t.Run("explicit_flags", func(t *testing.T) {
		cmd := buildCmd()
		setFlag(t, cmd, "format", "yaml")
		setFlag(t, cmd, "stack", "staging")
		setFlag(t, cmd, "enabled", "true")
		setFlag(t, cmd, "locked", "false")
		setFlag(t, cmd, "process-templates", "false")
		setFlag(t, cmd, "process-functions", "false")
		v := bindFlagsToViper(t, cmd, componentsParser)

		opts := parseComponentsOptions(cmd, v)

		assert.Equal(t, "yaml", opts.Format)
		assert.Equal(t, "staging", opts.Stack)
		require.NotNil(t, opts.Enabled)
		assert.True(t, *opts.Enabled)
		require.NotNil(t, opts.Locked)
		assert.False(t, *opts.Locked)
		assert.False(t, opts.ProcessTemplates)
		assert.False(t, opts.ProcessFunctions)
	})
}

// TestParseMetadataOptions verifies the viper→options mapping for
// `list metadata`.
func TestParseMetadataOptions(t *testing.T) {
	buildCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "metadata"}
		metadataParser.RegisterFlags(cmd)
		return cmd
	}

	t.Run("defaults", func(t *testing.T) {
		cmd := buildCmd()
		v := bindFlagsToViper(t, cmd, metadataParser)

		opts := parseMetadataOptions(cmd, v)

		assert.Equal(t, "", opts.Format)
		assert.True(t, opts.ProcessTemplates)
		assert.True(t, opts.ProcessFunctions)
	})

	t.Run("explicit_flags", func(t *testing.T) {
		cmd := buildCmd()
		setFlag(t, cmd, "format", "csv")
		setFlag(t, cmd, "stack", "dev-*")
		setFlag(t, cmd, "process-templates", "false")
		setFlag(t, cmd, "process-functions", "true")
		v := bindFlagsToViper(t, cmd, metadataParser)

		opts := parseMetadataOptions(cmd, v)

		assert.Equal(t, "csv", opts.Format)
		assert.Equal(t, "dev-*", opts.Stack)
		assert.False(t, opts.ProcessTemplates)
		assert.True(t, opts.ProcessFunctions)
	})
}

// TestParseSourcesOptions verifies the viper→options mapping for
// `list sources` and the positional component-filter arg handling.
func TestParseSourcesOptions(t *testing.T) {
	buildCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "sources"}
		sourcesParser.RegisterFlags(cmd)
		return cmd
	}

	t.Run("defaults_no_args", func(t *testing.T) {
		cmd := buildCmd()
		v := bindFlagsToViper(t, cmd, sourcesParser)

		opts := parseSourcesOptions(cmd, v, nil)

		assert.Equal(t, "", opts.Format)
		assert.Equal(t, "", opts.Component, "no positional arg → empty component filter")
		assert.True(t, opts.ProcessTemplates)
		assert.True(t, opts.ProcessFunctions)
	})

	t.Run("component_filter_from_args", func(t *testing.T) {
		cmd := buildCmd()
		v := bindFlagsToViper(t, cmd, sourcesParser)

		opts := parseSourcesOptions(cmd, v, []string{"vpc"})

		assert.Equal(t, "vpc", opts.Component, "args[0] becomes the component filter")
	})

	t.Run("explicit_flags", func(t *testing.T) {
		cmd := buildCmd()
		setFlag(t, cmd, "format", "json")
		setFlag(t, cmd, "stack", "prod-us-east-1")
		setFlag(t, cmd, "process-functions", "false")
		v := bindFlagsToViper(t, cmd, sourcesParser)

		opts := parseSourcesOptions(cmd, v, nil)

		assert.Equal(t, "json", opts.Format)
		assert.Equal(t, "prod-us-east-1", opts.Stack)
		assert.True(t, opts.ProcessTemplates)
		assert.False(t, opts.ProcessFunctions)
	})
}

// TestParseStacksOptions verifies the viper→options mapping for
// `list stacks`.
func TestParseStacksOptions(t *testing.T) {
	buildCmd := func() *cobra.Command {
		cmd := &cobra.Command{Use: "stacks"}
		stacksParser.RegisterFlags(cmd)
		return cmd
	}

	t.Run("defaults", func(t *testing.T) {
		cmd := buildCmd()
		v := bindFlagsToViper(t, cmd, stacksParser)

		opts := parseStacksOptions(cmd, v)

		assert.Equal(t, "", opts.Format)
		assert.False(t, opts.Provenance)
		assert.True(t, opts.ProcessTemplates)
		assert.True(t, opts.ProcessFunctions)
	})

	t.Run("explicit_flags", func(t *testing.T) {
		cmd := buildCmd()
		setFlag(t, cmd, "format", "tree")
		setFlag(t, cmd, "provenance", "true")
		setFlag(t, cmd, "component", "vpc")
		setFlag(t, cmd, "process-templates", "false")
		setFlag(t, cmd, "process-functions", "false")
		v := bindFlagsToViper(t, cmd, stacksParser)

		opts := parseStacksOptions(cmd, v)

		assert.Equal(t, "tree", opts.Format)
		assert.True(t, opts.Provenance)
		assert.Equal(t, "vpc", opts.Component)
		assert.False(t, opts.ProcessTemplates)
		assert.False(t, opts.ProcessFunctions)
	})
}
