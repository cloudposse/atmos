package list

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/flags"
)

// rootLikeViper mimics the global Viper configured by the root command
// (SetEnvPrefix("ATMOS") + AutomaticEnv), which resolves ATMOS_<KEY> for any
// bare key before explicitly bound environment variables.
func rootLikeViper() *viper.Viper {
	v := viper.New()
	v.SetEnvPrefix("ATMOS")
	v.AutomaticEnv()
	return v
}

// bindRootLike registers the parser's flags on a fresh command and binds them to a root-like Viper.
func bindRootLike(t *testing.T, use string, parser *flags.StandardParser) (*cobra.Command, *viper.Viper) {
	t.Helper()

	cmd := newCmdWithListParser(use, parser.RegisterFlags)
	v := rootLikeViper()
	require.NoError(t, parser.BindFlagsToViper(cmd, v))
	return cmd, v
}

// TestListSelectors_IgnoreTerraformEnvVars proves a job-level ATMOS_TAGS / ATMOS_LABELS (owned by the
// terraform family) no longer narrows any list command, while the list-specific variables still work.
func TestListSelectors_IgnoreTerraformEnvVars(t *testing.T) {
	tests := []struct {
		name   string
		parser *flags.StandardParser
		parse  func(cmd *cobra.Command, v *viper.Viper) (selectorTags []string, labels string)
	}{
		{"components", componentsParser, func(cmd *cobra.Command, v *viper.Viper) ([]string, string) {
			o := parseComponentsOptions(cmd, v)
			return o.Tags, o.LabelsRaw
		}},
		{"stacks", stacksParser, func(cmd *cobra.Command, v *viper.Viper) ([]string, string) {
			o := parseStacksOptions(cmd, v)
			return o.Tags, o.LabelsRaw
		}},
		{"metadata", metadataParser, func(cmd *cobra.Command, v *viper.Viper) ([]string, string) {
			o := parseMetadataOptions(cmd, v)
			return o.Tags, o.LabelsRaw
		}},
		{"instances", instancesParser, func(cmd *cobra.Command, v *viper.Viper) ([]string, string) {
			o := parseInstancesOptions(cmd, v)
			return o.Tags, o.LabelsRaw
		}},
		{"sources", sourcesParser, func(cmd *cobra.Command, v *viper.Viper) ([]string, string) {
			o := parseSourcesOptions(cmd, v, nil)
			return o.Tags, o.LabelsRaw
		}},
		{"dependencies", dependenciesParser, func(cmd *cobra.Command, v *viper.Viper) ([]string, string) {
			o := parseDependenciesOptions(cmd, v, nil)
			return o.Tags, o.LabelsRaw
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name+"/terraform env vars do not leak", func(t *testing.T) {
			t.Setenv("ATMOS_TAGS", "leak")
			t.Setenv("ATMOS_LABELS", "ci=manual")

			cmd, v := bindRootLike(t, tt.name, tt.parser)
			selectorTags, labels := tt.parse(cmd, v)

			assert.Empty(t, selectorTags)
			assert.Empty(t, labels)
		})

		t.Run(tt.name+"/list env vars are honoured over the leaked ones", func(t *testing.T) {
			t.Setenv("ATMOS_TAGS", "leak")
			t.Setenv("ATMOS_LABELS", "ci=manual")
			t.Setenv("ATMOS_COMPONENT_TAGS", "istio, eks")
			t.Setenv("ATMOS_COMPONENT_LABELS", "team=platform")

			cmd, v := bindRootLike(t, tt.name, tt.parser)
			selectorTags, labels := tt.parse(cmd, v)

			assert.Equal(t, []string{"istio", "eks"}, selectorTags)
			assert.Equal(t, "team=platform", labels)
		})

		t.Run(tt.name+"/cli flags win over env vars", func(t *testing.T) {
			t.Setenv("ATMOS_COMPONENT_TAGS", "env")
			t.Setenv("ATMOS_COMPONENT_LABELS", "team=env")

			cmd := newCmdWithListParser(tt.name, tt.parser.RegisterFlags)
			setFlag(t, cmd, "tags", "cli")
			setFlag(t, cmd, "labels", "team=cli")
			v := rootLikeViper()
			require.NoError(t, tt.parser.BindFlagsToViper(cmd, v))
			selectorTags, labels := tt.parse(cmd, v)

			assert.Equal(t, []string{"cli"}, selectorTags)
			assert.Equal(t, "team=cli", labels)
		})
	}
}

func TestListVendorTags_IgnoreTerraformEnvVar(t *testing.T) {
	t.Run("ATMOS_TAGS does not leak", func(t *testing.T) {
		t.Setenv("ATMOS_TAGS", "leak")

		_, v := bindRootLike(t, "vendor", vendorParser)

		assert.Empty(t, v.GetString(vendorTagsViperKey))
	})

	t.Run("ATMOS_VENDOR_TAGS is honoured", func(t *testing.T) {
		t.Setenv("ATMOS_TAGS", "leak")
		t.Setenv("ATMOS_VENDOR_TAGS", "networking")

		_, v := bindRootLike(t, "vendor", vendorParser)

		assert.Equal(t, "networking", v.GetString(vendorTagsViperKey))
	})

	t.Run("ATMOS_COMPONENT_TAGS does not reach list vendor", func(t *testing.T) {
		t.Setenv("ATMOS_COMPONENT_TAGS", "istio")

		_, v := bindRootLike(t, "vendor", vendorParser)

		assert.Empty(t, v.GetString(vendorTagsViperKey))
	})

	t.Run("ATMOS_VENDOR_TAGS does not reach list components", func(t *testing.T) {
		t.Setenv("ATMOS_VENDOR_TAGS", "networking")

		cmd, v := bindRootLike(t, "components", componentsParser)
		opts := parseComponentsOptions(cmd, v)

		assert.Empty(t, opts.Tags)
		assert.Empty(t, v.GetString(tagsViperKey))
	})

	t.Run("both env vars stay independent when set together", func(t *testing.T) {
		t.Setenv("ATMOS_COMPONENT_TAGS", "istio")
		t.Setenv("ATMOS_VENDOR_TAGS", "networking")

		_, vv := bindRootLike(t, "vendor", vendorParser)
		cmd, cv := bindRootLike(t, "components", componentsParser)

		assert.Equal(t, "networking", vv.GetString(vendorTagsViperKey))
		assert.Equal(t, []string{"istio"}, parseComponentsOptions(cmd, cv).Tags)
	})
}

// TestListComponents_EnvSelectorsEndToEnd runs `list components` extraction against a fixture with
// metadata.tags: a leaked ATMOS_TAGS must not narrow the result, ATMOS_COMPONENT_TAGS must.
func TestListComponents_EnvSelectorsEndToEnd(t *testing.T) {
	componentNames := func(t *testing.T) []string {
		t.Helper()

		initExecutorTestIO(t)
		chdirToListComponentsClosureFixture(t)

		cmd, v := bindRootLike(t, "components", componentsParser)
		// Other tests in this package leak a viper "identity" value via the global singleton; this
		// fixture configures no auth, so disable identity resolution explicitly.
		require.NoError(t, cmd.Flags().Set("identity", "false"))
		opts := parseComponentsOptions(cmd, v)
		opts.Format = "json"
		opts.Stack = "prod"

		result, err := initAndExtractComponents(cmd, []string{}, opts)
		require.NoError(t, err)

		names := make([]string, 0, len(result.components))
		for _, component := range result.components {
			name, _ := component["component"].(string)
			names = append(names, name)
		}
		return names
	}

	t.Run("leaked ATMOS_TAGS does not narrow the listing", func(t *testing.T) {
		t.Setenv("ATMOS_TAGS", "istio")
		t.Setenv("ATMOS_LABELS", "ci=manual")

		assert.ElementsMatch(t, []string{"vpc", "eks/cluster", "eks/karpenter", "eks/istio/base", "eks/istio/istiod"}, componentNames(t))
	})

	t.Run("ATMOS_COMPONENT_TAGS narrows the listing", func(t *testing.T) {
		t.Setenv("ATMOS_TAGS", "network")
		t.Setenv("ATMOS_COMPONENT_TAGS", "istio")

		assert.ElementsMatch(t, []string{"eks/istio/base", "eks/istio/istiod"}, componentNames(t))
	})
}
