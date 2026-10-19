package cmd

import (
	"bytes"
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/script"
)

func parseStandalone(t *testing.T, spec *script.CommandSpec, argv ...string) (script.CommandInput, string, error) {
	t.Helper()
	var stdout bytes.Buffer
	input, err := standaloneCommandParser(&script.File{Path: "/work/tool.star", Invoked: "./tool.star", Args: argv}, &stdout)(t.Context(), *spec)
	return input, stdout.String(), err
}

func TestStandaloneEnvironmentListsParseLikeCommandLineValues(t *testing.T) {
	listSpec := func(env string) *script.CommandSpec {
		return &script.CommandSpec{Flags: []flags.Flag{
			&flags.StringSliceFlag{Name: "tags", EnvVars: []string{env}},
			&flags.StringSliceFlag{Name: "zones", EnvVars: []string{env + "_ZONES"}, ValidValues: []string{"a", "b"}},
		}}
	}
	for _, tc := range []struct {
		name, env string
		want      []string
	}{
		{"comma separated", "a,b", []string{"a", "b"}},
		{"whitespace is not a separator", "a b", []string{"a b"}},
		{"csv quoting keeps commas", `"a,b",c`, []string{"a,b", "c"}},
		{"single item", "solo", []string{"solo"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("FT_TAGS_TEST", tc.env)
			fromEnv, _, err := parseStandalone(t, listSpec("FT_TAGS_TEST"))
			require.NoError(t, err)
			assert.Equal(t, tc.want, fromEnv.Flags["tags"])

			// The same text on the command line must produce the same list.
			fromCLI, _, err := parseStandalone(t, &script.CommandSpec{Flags: []flags.Flag{&flags.StringSliceFlag{Name: "tags"}}}, "--tags="+tc.env)
			require.NoError(t, err)
			assert.Equal(t, fromCLI.Flags["tags"], fromEnv.Flags["tags"])
		})
	}

	t.Run("choices validate each element", func(t *testing.T) {
		t.Setenv("FT_TAGS_TEST_ZONES", "a,b")
		input, _, err := parseStandalone(t, listSpec("FT_TAGS_TEST"))
		require.NoError(t, err)
		assert.Equal(t, []string{"a", "b"}, input.Flags["zones"])

		t.Setenv("FT_TAGS_TEST_ZONES", "a,z")
		_, _, err = parseStandalone(t, listSpec("FT_TAGS_TEST"))
		require.ErrorIs(t, err, errUtils.ErrScriptUsage)
		assert.ErrorContains(t, err, `"z"`)
	})
	t.Run("command line wins over the environment", func(t *testing.T) {
		t.Setenv("FT_TAGS_TEST", "env1,env2")
		input, _, err := parseStandalone(t, listSpec("FT_TAGS_TEST"), "--tags=cli")
		require.NoError(t, err)
		assert.Equal(t, []string{"cli"}, input.Flags["tags"])
	})
	t.Run("malformed CSV is a usage error", func(t *testing.T) {
		t.Setenv("FT_TAGS_TEST", `"unterminated`)
		_, _, err := parseStandalone(t, listSpec("FT_TAGS_TEST"))
		require.ErrorIs(t, err, errUtils.ErrScriptUsage)
		require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
	})
}

func TestStandaloneIntegersAreBaseTen(t *testing.T) {
	spec := script.CommandSpec{Flags: []flags.Flag{&flags.IntFlag{Name: "count", Default: 1, EnvVars: []string{"FT_COUNT_TEST"}}}}
	t.Run("leading zeros are decimal, not octal", func(t *testing.T) {
		input, _, err := parseStandalone(t, &spec, "--count", "010")
		require.NoError(t, err)
		assert.Equal(t, 10, input.Flags["count"])
	})
	t.Run("environment leading zeros are decimal", func(t *testing.T) {
		t.Setenv("FT_COUNT_TEST", "010")
		input, _, err := parseStandalone(t, &spec)
		require.NoError(t, err)
		assert.Equal(t, 10, input.Flags["count"])
	})
	t.Run("negative and shorthand-free values", func(t *testing.T) {
		input, _, err := parseStandalone(t, &spec, "--count=-3")
		require.NoError(t, err)
		assert.Equal(t, -3, input.Flags["count"])
	})
	for _, bad := range []string{"0x10", "0b11", "0o7", "1_000", "1.5", "ten"} {
		t.Run("command line rejects "+bad, func(t *testing.T) {
			_, _, err := parseStandalone(t, &spec, "--count="+bad)
			require.ErrorIs(t, err, errUtils.ErrScriptUsage)
			require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
			assert.ErrorContains(t, err, "base-10")
		})
		t.Run("environment rejects "+bad, func(t *testing.T) {
			t.Setenv("FT_COUNT_TEST", bad)
			_, _, err := parseStandalone(t, &spec)
			require.ErrorIs(t, err, errUtils.ErrScriptUsage)
			require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
			assert.ErrorContains(t, err, "base-10")
		})
	}
	t.Run("default survives when unset", func(t *testing.T) {
		input, _, err := parseStandalone(t, &spec)
		require.NoError(t, err)
		assert.Equal(t, 1, input.Flags["count"])
	})
}

func TestStandaloneBooleanWordAfterBooleanFlag(t *testing.T) {
	spec := script.CommandSpec{
		Args: []*flags.PositionalArgSpec{{Name: "service"}, {Name: "region", Required: false}},
		Flags: []flags.Flag{
			&flags.BoolFlag{Name: "verbose", Shorthand: "v"},
			&flags.BoolFlag{Name: "quiet", Shorthand: "q"},
			&flags.StringFlag{Name: "name"},
		},
	}
	for _, tc := range []struct {
		argv []string
		hint string
	}{
		{[]string{"svc", "--verbose", "false"}, "--verbose=false"},
		{[]string{"svc", "--verbose", "FALSE"}, "--verbose=false"},
		{[]string{"svc", "-v", "true"}, "--verbose=true"},
		{[]string{"svc", "-vq", "True"}, "--quiet=true"},
		{[]string{"--verbose", "false", "svc"}, "--verbose=false"},
	} {
		t.Run(strings.Join(tc.argv, " "), func(t *testing.T) {
			_, _, err := parseStandalone(t, &spec, tc.argv...)
			require.ErrorIs(t, err, errUtils.ErrScriptUsage)
			assert.Contains(t, cockroach.FlattenHints(err), tc.hint)
			assert.Equal(t, 2, errUtils.GetExitCode(err))
		})
	}

	for name, argv := range map[string][]string{
		"explicit equals":                    {"svc", "--verbose=false"},
		"value consumed by a string flag":    {"svc", "--name", "false"},
		"word before the flag":               {"false", "--verbose"},
		"after the separator":                {"svc", "--verbose", "--", "false"},
		"string flag takes the next flag":    {"svc", "--name", "--verbose"},
		"unrelated positional after boolean": {"svc", "--verbose", "east"},
	} {
		t.Run("allowed "+name, func(t *testing.T) {
			_, _, err := parseStandalone(t, &spec, argv...)
			require.NoError(t, err)
		})
	}
}

func TestStandaloneUserInputErrorsArePresentedAsUsageErrors(t *testing.T) {
	spec := script.CommandSpec{
		Args: []*flags.PositionalArgSpec{{Name: "service", Required: true}},
		Flags: []flags.Flag{
			&flags.StringFlag{Name: "stage", ValidValues: []string{"dev", "prod"}},
			&flags.IntFlag{Name: "count"},
			&flags.StringFlag{Name: "token", Required: true},
		},
	}
	for _, tc := range []struct {
		name string
		argv []string
		want string
	}{
		{"unknown flag", []string{"svc", "--token=t", "--bogus"}, "unknown flag: --bogus"},
		{"missing positional is named", []string{"--token=t"}, "missing required argument `<service>`"},
		{"extra positional", []string{"svc", "other", "--token=t"}, "unexpected argument"},
		{"invalid value", []string{"svc", "--token=t", "--count=many"}, "count"},
		{"missing required flag", []string{"svc"}, "token"},
		{"failed choices", []string{"svc", "--token=t", "--stage=qa"}, "qa"},
		{"flag without value", []string{"svc", "--token"}, "token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parseStandalone(t, &spec, tc.argv...)
			require.ErrorIs(t, err, errUtils.ErrScriptUsage)
			assert.ErrorContains(t, err, tc.want)
			assert.NotErrorIs(t, err, errUtils.ErrStarlark, "usage errors are not script failures")
			assert.Equal(t, 2, errUtils.GetExitCode(err), "usage errors exit with status 2")
			assert.Contains(t, cockroach.FlattenHints(err), "Run ./tool.star --help for usage.")
			assert.Contains(t, cockroach.FlattenDetails(err), "tool <service> [flags]", "the command name drops the script extension")
		})
	}
}

func TestStandaloneUsageHintFallsBackToFileName(t *testing.T) {
	var stdout bytes.Buffer
	_, err := standaloneCommandParser(&script.File{Path: "/work/tool.star", Args: []string{"--bogus"}}, &stdout)(t.Context(), script.CommandSpec{})
	require.ErrorIs(t, err, errUtils.ErrScriptUsage)
	assert.Contains(t, cockroach.FlattenHints(err), "Run tool.star --help for usage.")
}

func TestStandaloneHelpMarksRequiredChoicesAndEnvironment(t *testing.T) {
	spec := script.CommandSpec{
		Name: "deploy",
		Flags: []flags.Flag{
			&flags.StringFlag{Name: "token", Required: true, Description: "API token", EnvVars: []string{"FT_TOKEN_HELP"}},
			&flags.StringFlag{Name: "stage", Default: "dev", ValidValues: []string{"dev", "prod"}, EnvVars: []string{"FT_STAGE_HELP", "STAGE"}},
			&flags.StringSliceFlag{Name: "zones", ValidValues: []string{"a", "b"}},
			&flags.IntFlag{Name: "count", Default: 1, Description: "How many"},
		},
	}
	input, help, err := parseStandalone(t, &spec, "--help")
	require.NoError(t, err)
	assert.True(t, input.Help)
	assert.Contains(t, help, "API token (required) [env: FT_TOKEN_HELP]")
	assert.Contains(t, help, "(one of: dev, prod) [env: FT_STAGE_HELP, STAGE]")
	assert.Contains(t, help, "(one of: a, b)")
	assert.Contains(t, help, "How many (default 1)")
	assert.NotContains(t, help, "How many (required)")
}

func TestStandaloneRejectsCaseCollidingNames(t *testing.T) {
	_, _, err := parseStandalone(t, &script.CommandSpec{Flags: []flags.Flag{
		&flags.StringFlag{Name: "Stage"}, &flags.StringFlag{Name: "stage"},
	}})
	require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
	assert.ErrorContains(t, err, "case-insensitive")

	_, _, err = parseStandalone(t, &script.CommandSpec{Flags: []flags.Flag{&flags.StringFlag{Name: "HELP"}}})
	require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue, "help is reserved regardless of case")

	_, _, err = parseStandalone(t, &script.CommandSpec{Args: []*flags.PositionalArgSpec{
		{Name: "Service", Required: true}, {Name: "service"},
	}})
	require.ErrorIs(t, err, errUtils.ErrInvalidPositionalArgs)
	assert.ErrorContains(t, err, "case-insensitive")

	_, _, err = parseStandalone(t, &script.CommandSpec{Flags: []flags.Flag{
		&flags.StringFlag{Name: "stage"}, &flags.StringFlag{Name: "region"},
	}})
	require.NoError(t, err, "distinct names remain valid")
}

func TestStandaloneHelpListsArgumentsAndDropsTheExtension(t *testing.T) {
	spec := script.CommandSpec{
		Description: "Calculate capacity",
		Args: []*flags.PositionalArgSpec{
			{Name: "service", Description: "Service name", Required: true},
			{Name: "region", Description: "Where to run"},
		},
		Flags: []flags.Flag{
			&flags.StringSliceFlag{Name: "tags", Description: "Deploy tags"},
			&flags.IntFlag{Name: "replicas", Default: 2},
		},
	}
	input, help, err := parseStandalone(t, &spec, "--help")
	require.NoError(t, err)
	assert.True(t, input.Help)
	assert.Contains(t, help, "tool <service> [region] [flags]", "the default name has no .star extension")
	assert.NotContains(t, help, "tool.star")
	assert.Contains(t, help, "Arguments:\n  <service>  Service name\n  [region]   Where to run\n")
	assert.Contains(t, help, "--tags list")
	assert.NotContains(t, help, "strings")

	_, help, err = parseStandalone(t, &script.CommandSpec{Name: "custom"}, "--help")
	require.NoError(t, err)
	assert.Contains(t, help, "custom [flags]", "an explicit name is used as written")
	assert.NotContains(t, help, "Arguments:", "no section when the command takes no arguments")
}

func TestStandaloneEnvironmentErrorsNameTheVariable(t *testing.T) {
	spec := script.CommandSpec{Flags: []flags.Flag{
		&flags.StringFlag{Name: "stage", ValidValues: []string{"dev", "prod"}, EnvVars: []string{"FT_STAGE_NAMED"}},
		&flags.IntFlag{Name: "count", EnvVars: []string{"FT_COUNT_NAMED"}},
		&flags.StringSliceFlag{Name: "zones", ValidValues: []string{"a", "b"}, EnvVars: []string{"FT_ZONES_NAMED"}},
	}}
	for _, tc := range []struct {
		name, variable, value, want string
		argv                        []string
	}{
		{"choice", "FT_STAGE_NAMED", "qa", `invalid value "qa" from FT_STAGE_NAMED for flag --stage`, nil},
		{"integer", "FT_COUNT_NAMED", "many", `invalid value "many" from FT_COUNT_NAMED for flag --count: not a base-10 integer`, nil},
		{"integer range", "FT_COUNT_NAMED", "99999999999999999999", "out of range for a 64-bit integer", nil},
		{"list item", "FT_ZONES_NAMED", "a,z", `from FT_ZONES_NAMED for flag --zones: "z" is not one of: a, b`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(tc.variable, tc.value)
			_, _, err := parseStandalone(t, &spec, tc.argv...)
			require.ErrorIs(t, err, errUtils.ErrScriptUsage)
			require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
			assert.ErrorContains(t, err, tc.want)
			assert.NotContains(t, err.Error(), "invalid value for flag: invalid value")
		})
	}

	t.Run("a valid environment value is accepted", func(t *testing.T) {
		t.Setenv("FT_STAGE_NAMED", "prod")
		input, _, err := parseStandalone(t, &spec)
		require.NoError(t, err)
		assert.Equal(t, "prod", input.Flags["stage"])
	})
	t.Run("the command line is not checked against the environment", func(t *testing.T) {
		t.Setenv("FT_STAGE_NAMED", "qa")
		input, _, err := parseStandalone(t, &spec, "--stage=dev")
		require.NoError(t, err)
		assert.Equal(t, "dev", input.Flags["stage"])
	})
}

func TestStandaloneCommandLineErrorsAreNotDoubled(t *testing.T) {
	spec := script.CommandSpec{Flags: []flags.Flag{
		&flags.IntFlag{Name: "count"}, &flags.StringFlag{Name: "stage", ValidValues: []string{"dev", "prod"}},
	}}
	_, _, err := parseStandalone(t, &spec, "--count=99999999999999999999")
	require.ErrorIs(t, err, errUtils.ErrScriptUsage)
	assert.ErrorContains(t, err, "out of range for a 64-bit integer")
	assert.NotContains(t, err.Error(), "invalid value for flag: ")

	_, _, err = parseStandalone(t, &spec, "--stage=qa")
	require.ErrorIs(t, err, errUtils.ErrScriptUsage)
	assert.NotContains(t, err.Error(), "invalid value for flag: invalid value")
}

// An explicitly empty value is still a value the user typed, so choices apply to it. An unset flag
// stays empty and is not checked.
func TestStandaloneExplicitEmptyValueIsCheckedAgainstChoices(t *testing.T) {
	spec := func() *script.CommandSpec {
		return &script.CommandSpec{Flags: []flags.Flag{
			&flags.StringFlag{Name: "stage", ValidValues: []string{"dev", "prod"}},
			&flags.StringSliceFlag{Name: "zones", ValidValues: []string{"a", "b"}},
		}}
	}
	for _, argv := range [][]string{{"--stage="}, {"--stage", ""}, {"--zones=a,"}} {
		t.Run(strings.Join(argv, " "), func(t *testing.T) {
			_, _, err := parseStandalone(t, spec(), argv...)
			require.ErrorIs(t, err, errUtils.ErrScriptUsage)
			require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
			assert.ErrorContains(t, err, `invalid value ""`)
			assert.ErrorContains(t, err, "valid values")
		})
	}
	t.Run("unset flags stay empty", func(t *testing.T) {
		input, _, err := parseStandalone(t, spec())
		require.NoError(t, err)
		assert.Equal(t, "", input.Flags["stage"])
	})
	t.Run("a flag without choices accepts an empty value", func(t *testing.T) {
		input, _, err := parseStandalone(t, &script.CommandSpec{Flags: []flags.Flag{&flags.StringFlag{Name: "note"}}}, "--note=")
		require.NoError(t, err)
		assert.Equal(t, "", input.Flags["note"])
	})
	t.Run("an empty environment value is ignored as before", func(t *testing.T) {
		t.Setenv("FT_STAGE_EMPTY", "")
		input, _, err := parseStandalone(t, &script.CommandSpec{Flags: []flags.Flag{
			&flags.StringFlag{Name: "stage", ValidValues: []string{"dev"}, EnvVars: []string{"FT_STAGE_EMPTY"}},
		}})
		require.NoError(t, err)
		assert.Equal(t, "", input.Flags["stage"])
	})
}
