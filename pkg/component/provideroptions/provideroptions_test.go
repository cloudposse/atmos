package provideroptions

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels for the schema fields these tests depend on.
var _ = schema.ComponentProviderOption{Value: "", Sensitive: true}

func declaration(fields map[string]any) any {
	base := map[string]any{"name": "level", "type": "string", "description": "A level.", "commands": []any{"plan"}}
	for key, value := range fields {
		base[key] = value
	}
	return []any{base}
}

func TestParseAcceptsEveryType(t *testing.T) {
	raw := []any{
		map[string]any{"name": "feature", "type": "map", "config": "features", "env": []any{"TG_FEATURE"}, "description": "Features.", "commands": []any{"plan", "apply"}, "modes": []any{"native", "executable"}},
		map[string]any{"name": "retries", "type": "int", "default": 3, "description": "Retries.", "commands": []any{"plan"}, "aliases": []any{"retry-count"}},
		map[string]any{"name": "quiet", "type": "bool", "default": false, "description": "Quiet.", "commands": []any{"plan"}},
		map[string]any{"name": "paths", "type": "list", "default": []any{"a"}, "description": "Paths.", "commands": []any{"plan"}},
		map[string]any{"name": "token", "type": "string", "sensitive": true, "description": "Token.", "commands": []any{"plan"}},
	}
	options, err := Parse("tool", raw)
	require.NoError(t, err)
	require.Len(t, options, 5)
	assert.Equal(t, "tool-feature", options[0].FlagName("tool"))
	assert.Equal(t, []string{"ATMOS_TOOL_FEATURE", "TG_FEATURE"}, options[0].EnvNames("tool"))
	assert.Equal(t, 3, options[1].Default)
	assert.Equal(t, []string{"a"}, options[3].Default)
	assert.True(t, options[4].Sensitive)
	assert.True(t, options[0].AppliesTo(ModeExecutable))
	assert.True(t, options[1].AppliesTo(ModeNative), "no modes means every mode")
	last, err := Parse("tool", nil)
	require.NoError(t, err)
	assert.Nil(t, last)
}

func TestParseRejectsInvalidDeclarations(t *testing.T) {
	cases := map[string]any{
		"not a list":        "options",
		"not a mapping":     []any{"level"},
		"unknown key":       declaration(map[string]any{"flavor": "x"}),
		"bad name":          declaration(map[string]any{"name": "Level"}),
		"bad type":          declaration(map[string]any{"type": "float"}),
		"missing help":      declaration(map[string]any{"description": ""}),
		"no commands":       declaration(map[string]any{"commands": nil}),
		"commands type":     declaration(map[string]any{"commands": "plan"}),
		"secret in config":  declaration(map[string]any{"sensitive": true, "config": "level"}),
		"secret default":    declaration(map[string]any{"sensitive": true, "default": "x"}),
		"default mismatch":  declaration(map[string]any{"type": "int", "default": "x"}),
		"map default":       declaration(map[string]any{"type": "map", "default": map[string]any{}}),
		"bad flag":          declaration(map[string]any{"flag": "Bad_Flag"}),
		"bad env":           declaration(map[string]any{"env": []any{"lower"}}),
		"bad mode":          declaration(map[string]any{"modes": []any{"fast"}}),
		"duplicate aliases": declaration(map[string]any{"aliases": []any{"tool-level"}}),
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Parse("tool", raw)
			require.ErrorIs(t, err, errUtils.ErrProviderOption)
		})
	}
}

func TestParseRejectsTwoOptionsWithOneFlag(t *testing.T) {
	_, err := Parse("tool", []any{
		map[string]any{"name": "one", "type": "string", "description": "One.", "commands": []any{"plan"}, "flag": "same"},
		map[string]any{"name": "two", "type": "string", "description": "Two.", "commands": []any{"plan"}, "flag": "same"},
	})
	require.ErrorIs(t, err, errUtils.ErrProviderOption)
}

// newTree builds a terraform-like command with two subcommands.
func newTree() (root, plan, apply *cobra.Command) {
	root = &cobra.Command{Use: "terraform"}
	plan = &cobra.Command{Use: "plan", RunE: func(*cobra.Command, []string) error { return nil }}
	apply = &cobra.Command{Use: "apply", RunE: func(*cobra.Command, []string) error { return nil }}
	root.AddCommand(plan, apply)
	return root, plan, apply
}

func providerFor(t *testing.T, section string, raw []any) Provider {
	t.Helper()
	options, err := Parse("tool", raw)
	require.NoError(t, err)
	return Provider{Section: section, Name: "tool", Mode: ModeNative, Options: options}
}

func TestRegisterAddsFlagsOnlyWhereTheyApply(t *testing.T) {
	root, plan, apply := newTree()
	provider := providerFor(t, "tool", []any{
		map[string]any{"name": "level", "type": "string", "description": "A level.", "commands": []any{"plan"}},
		map[string]any{"name": "fast", "type": "bool", "description": "Fast.", "commands": []any{"plan", "apply"}, "modes": []any{"executable"}},
		map[string]any{"name": "elsewhere", "type": "string", "description": "Missing command.", "commands": []any{"destroy"}},
		map[string]any{"name": "old", "type": "string", "description": "Old.", "commands": []any{"apply"}, "deprecated": "use --tool-level", "aliases": []any{"tool-legacy"}},
	})
	require.NoError(t, Register(root, []Provider{provider}))
	assert.NotNil(t, plan.Flags().Lookup("tool-level"))
	assert.Nil(t, apply.Flags().Lookup("tool-level"))
	assert.Nil(t, plan.Flags().Lookup("tool-fast"), "an executable-only option is not registered in native mode")
	assert.True(t, apply.Flags().Lookup("tool-old").Deprecated != "")
	assert.True(t, apply.Flags().Lookup("tool-legacy").Hidden)
	// Registering again is a no-op, not a collision with itself.
	require.NoError(t, Register(root, []Provider{provider}))
}

func TestRegisterRejectsCollisions(t *testing.T) {
	root, plan, _ := newTree()
	plan.Flags().Bool("tool-level", false, "an Atmos flag")
	provider := providerFor(t, "tool", []any{
		map[string]any{"name": "level", "type": "string", "description": "A level.", "commands": []any{"plan"}},
	})
	err := Register(root, []Provider{provider})
	require.ErrorIs(t, err, errUtils.ErrProviderOptionCollision)

	root, plan, _ = newTree()
	plan.PersistentFlags().Bool("all", false, "selection")
	reserved := providerFor(t, "tool", []any{
		map[string]any{"name": "everything", "flag": "all", "type": "bool", "description": "All.", "commands": []any{"plan"}},
	})
	require.ErrorIs(t, Register(root, []Provider{reserved}), errUtils.ErrProviderOptionCollision)
}

func TestRegisterRejectsTwoProvidersWithOneFlag(t *testing.T) {
	root, _, _ := newTree()
	raw := []any{map[string]any{"name": "level", "type": "string", "flag": "shared-level", "description": "A level.", "commands": []any{"plan"}}}
	first := providerFor(t, "one", raw)
	first.Name = "one"
	second := providerFor(t, "two", raw)
	second.Name = "two"
	require.ErrorIs(t, Register(root, []Provider{first, second}), errUtils.ErrProviderOptionCollision)
}

func resolveWith(t *testing.T, raw []any, args []string, env map[string]string) (map[string]map[string]schema.ComponentProviderOption, error) {
	t.Helper()
	for name, value := range env {
		t.Setenv(name, value)
	}
	root, plan, _ := newTree()
	require.NoError(t, Register(root, []Provider{providerFor(t, "tool", raw)}))
	require.NoError(t, plan.ParseFlags(args))
	return Resolve(plan, viper.New())
}

var sample = []any{
	map[string]any{"name": "feature", "type": "map", "env": []any{"TG_TEST_FEATURE"}, "description": "Features.", "commands": []any{"plan"}},
	map[string]any{"name": "level", "type": "string", "default": "info", "description": "Level.", "commands": []any{"plan"}},
	map[string]any{"name": "retries", "type": "int", "description": "Retries.", "commands": []any{"plan"}},
	map[string]any{"name": "quiet", "type": "bool", "description": "Quiet.", "commands": []any{"plan"}},
	map[string]any{"name": "paths", "type": "list", "description": "Paths.", "commands": []any{"plan"}},
	map[string]any{"name": "token", "type": "string", "sensitive": true, "description": "Token.", "commands": []any{"plan"}},
}

func TestResolveReportsOnlyExplicitOptions(t *testing.T) {
	resolved, err := resolveWith(t, sample, nil, nil)
	require.NoError(t, err)
	assert.Empty(t, resolved, "defaults are not explicit values")
}

func TestResolvePreservesExplicitFalseZeroAndEmpty(t *testing.T) {
	resolved, err := resolveWith(t, sample, []string{"--tool-quiet=false", "--tool-retries=0", "--tool-level=", "--tool-paths="}, nil)
	require.NoError(t, err)
	values := resolved["tool"]
	assert.Equal(t, false, values["quiet"].Value)
	assert.Equal(t, 0, values["retries"].Value)
	assert.Equal(t, "", values["level"].Value)
	assert.NotNil(t, values["paths"].Value)
	assert.Len(t, values, 4)
}

func TestResolveMapsAndListsFromTheCommandLine(t *testing.T) {
	resolved, err := resolveWith(t, sample, []string{"--tool-feature", "alpha=true", "--tool-feature=beta=two", "--tool-paths", "a", "--tool-paths", "b"}, nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"alpha": "true", "beta": "two"}, resolved["tool"]["feature"].Value)
	assert.Equal(t, []string{"a", "b"}, resolved["tool"]["paths"].Value)
}

func TestResolveCommandLineBeatsEnvironment(t *testing.T) {
	resolved, err := resolveWith(t, sample, []string{"--tool-level=cli"}, map[string]string{"ATMOS_TOOL_LEVEL": "env", "ATMOS_TOOL_RETRIES": "7", "TG_TEST_FEATURE": "x=1,y=2"})
	require.NoError(t, err)
	values := resolved["tool"]
	assert.Equal(t, "cli", values["level"].Value)
	assert.Equal(t, 7, values["retries"].Value)
	assert.Equal(t, map[string]any{"x": "1", "y": "2"}, values["feature"].Value)
}

func TestResolveRejectsInvalidValuesWithoutEchoingThem(t *testing.T) {
	for name, args := range map[string][]string{
		"bad map":   {"--tool-feature=novalue"},
		"empty key": {"--tool-feature==x"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := resolveWith(t, sample, args, nil)
			require.ErrorIs(t, err, errUtils.ErrProviderOption)
		})
	}
	// Environment values are parsed by Atmos, so a malformed one is rejected without being echoed.
	for name, value := range map[string]string{"ATMOS_TOOL_QUIET": "maybe", "ATMOS_TOOL_RETRIES": "lots"} {
		_, err := resolveWith(t, sample, nil, map[string]string{name: value})
		require.ErrorIs(t, err, errUtils.ErrProviderOption)
		assert.NotContains(t, err.Error(), value)
	}
}

func TestResolveMarksSensitiveValues(t *testing.T) {
	resolved, err := resolveWith(t, sample, []string{"--tool-token=s3cret-value"}, nil)
	require.NoError(t, err)
	assert.True(t, resolved["tool"]["token"].Sensitive)
	assert.Equal(t, "s3cret-value", resolved["tool"]["token"].Value)
}

func TestResolveAcceptsAliasFlags(t *testing.T) {
	raw := []any{map[string]any{"name": "level", "type": "string", "description": "Level.", "commands": []any{"plan"}, "aliases": []any{"tool-lvl"}}}
	resolved, err := resolveWith(t, raw, []string{"--tool-lvl=debug"}, nil)
	require.NoError(t, err)
	assert.Equal(t, "debug", resolved["tool"]["level"].Value)
}

func TestSplitFlagsGroupsProviderFlags(t *testing.T) {
	root, plan, _ := newTree()
	plan.Flags().Bool("affected", false, "Atmos flag")
	require.NoError(t, Register(root, []Provider{providerFor(t, "tool", sample)}))
	rest, sections := SplitFlags(plan.Flags())
	assert.NotNil(t, rest.Lookup("affected"))
	assert.Nil(t, rest.Lookup("tool-level"))
	require.Len(t, sections, 1)
	assert.Equal(t, "TOOL FLAGS", sections[0].Title)
	assert.NotNil(t, sections[0].Flags.Lookup("tool-level"))
}

func TestNamesAreDerivedFromProvidersWithSlashes(t *testing.T) {
	option := Option{Name: "iam_role"}
	assert.Equal(t, "vendor-tool-iam-role", option.FlagName("vendor/tool"))
	assert.Equal(t, []string{"ATMOS_VENDOR_TOOL_IAM_ROLE"}, option.EnvNames("vendor/tool"))
}
