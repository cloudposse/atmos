package standalone

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
)

func TestParseDecimalInt(t *testing.T) {
	t.Parallel()
	for raw, want := range map[string]int{"0": 0, "10": 10, "010": 10, "-7": -7, "+5": 5} {
		got, err := ParseDecimalInt(raw)
		require.NoError(t, err, raw)
		assert.Equal(t, want, got, raw)
	}
	for _, raw := range []string{"", "0x10", "0b1", "0o7", "1_0", "1.0", " 1", "9999999999999999999999"} {
		_, err := ParseDecimalInt(raw)
		require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue, raw)
	}
}

func TestDecimalIntIsAPflagValue(t *testing.T) {
	t.Parallel()
	set := pflag.NewFlagSet("test", pflag.ContinueOnError)
	set.Int("count", 3, "")
	set.Lookup("count").Value = NewDecimalInt(3)
	require.NoError(t, set.Parse([]string{"--count", "010"}))
	got, err := set.GetInt("count")
	require.NoError(t, err)
	assert.Equal(t, 10, got)
	assert.Equal(t, "int", set.Lookup("count").Value.Type())
	require.Error(t, set.Parse([]string{"--count=0x10"}))
}

func TestParseStringListMatchesPflagStringSlice(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"", "a", "a,b", "a b", `"a,b",c`, `a,"b ""quoted"""`, ",", "a,,b"} {
		set := pflag.NewFlagSet("test", pflag.ContinueOnError)
		want := set.StringSlice("list", nil, "")
		require.NoError(t, set.Parse([]string{"--list=" + raw}), raw)
		got, err := ParseStringList(raw)
		require.NoError(t, err, raw)
		assert.Equal(t, *want, got, raw)
	}
	_, err := ParseStringList(`"open`)
	require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
}

func TestValidatePositionals(t *testing.T) {
	t.Parallel()
	specs := []*flags.PositionalArgSpec{{Name: "service", Required: true}, {Name: "region"}}
	assert.NoError(t, ValidatePositionals(specs, []string{"api"}))
	assert.NoError(t, ValidatePositionals(specs, []string{"api", "east"}))
	assert.NoError(t, ValidatePositionals(nil, nil))

	err := ValidatePositionals(specs, nil)
	require.ErrorIs(t, err, errUtils.ErrScriptUsage)
	assert.ErrorContains(t, err, "missing required argument `<service>`")

	err = ValidatePositionals(specs, []string{"api", "east", "extra"})
	require.ErrorIs(t, err, errUtils.ErrScriptUsage)
	assert.ErrorContains(t, err, `"extra"`)

	err = ValidatePositionals(nil, []string{"extra"})
	require.ErrorIs(t, err, errUtils.ErrScriptUsage)
	assert.ErrorContains(t, err, "takes no positional arguments")
}

func TestNewUsageError(t *testing.T) {
	t.Parallel()
	cmd := &cobra.Command{Use: "tool <service>"}
	plain := NewUsageError(cmd, "./tool", errUtils.ErrInvalidFlagValue)
	require.ErrorIs(t, plain, errUtils.ErrScriptUsage)
	require.ErrorIs(t, plain, errUtils.ErrInvalidFlagValue, "the cause stays reachable")
	assert.Equal(t, UsageExitCode, errUtils.GetExitCode(plain))
	assert.Equal(t, "Run ./tool --help for usage.", cockroach.FlattenHints(plain))
	assert.Contains(t, cockroach.FlattenDetails(plain), "tool <service>")

	// An error that is already a usage error is not prefixed twice.
	already := NewUsageError(cmd, "./tool", ValidatePositionals([]*flags.PositionalArgSpec{{Name: "s", Required: true}}, nil))
	assert.Equal(t, 1, countOccurrences(already.Error(), errUtils.ErrScriptUsage.Error()))
}

func countOccurrences(text, part string) int {
	count := 0
	for index := 0; index+len(part) <= len(text); index++ {
		if text[index:index+len(part)] == part {
			count++
		}
	}
	return count
}

func TestAnnotateFlags(t *testing.T) {
	t.Parallel()
	set := pflag.NewFlagSet("test", pflag.ContinueOnError)
	declared := []flags.Flag{
		&flags.StringFlag{Name: "plain", Description: "Plain"},
		&flags.StringFlag{Name: "token", Description: "Token", Required: true, EnvVars: []string{"A", "B"}},
		&flags.StringFlag{Name: "stage", ValidValues: []string{"dev", "prod"}},
		&flags.StringSliceFlag{Name: "zones", ValidValues: []string{"a", "b"}, EnvVars: []string{"Z"}},
		&flags.BoolFlag{Name: "ghost"},
	}
	for _, name := range []string{"plain", "token", "stage", "zones"} {
		set.String(name, "", "")
	}
	for _, definition := range declared[:2] {
		set.Lookup(definition.GetName()).Usage = definition.GetDescription()
	}
	AnnotateFlags(set, declared)
	assert.Equal(t, "Plain", set.Lookup("plain").Usage)
	assert.Equal(t, "Token (required) [env: A, B]", set.Lookup("token").Usage)
	assert.Equal(t, "(one of: dev, prod)", set.Lookup("stage").Usage)
	assert.Equal(t, "(one of: a, b) [env: Z]", set.Lookup("zones").Usage)
}

func TestDefaultNameDropsTheExtension(t *testing.T) {
	t.Parallel()
	for filename, want := range map[string]string{
		"helplate.star": "helplate", "greeter": "greeter", "a.b.star": "a.b", "stdin": "stdin", "UPPER.STAR": "UPPER",
	} {
		assert.Equal(t, want, DefaultName(filename), filename)
	}
}

func newHelpCommand(use string) *cobra.Command {
	return &cobra.Command{Use: use, Short: "Short text", Run: func(*cobra.Command, []string) {}}
}

func helpText(t *testing.T, cmd *cobra.Command) string {
	t.Helper()
	var out strings.Builder
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	require.NoError(t, cmd.Help())
	return out.String()
}

func TestDescribeArgumentsAddsAnArgumentsSection(t *testing.T) {
	t.Parallel()
	cmd := newHelpCommand("deploy <service> [region]")
	DescribeArguments(cmd, []*flags.PositionalArgSpec{
		{Name: "service", Description: "Service to deploy", Required: true},
		{Name: "region", Description: "Target region"},
		{Name: "extra"},
	})
	help := helpText(t, cmd)
	assert.Contains(t, help, "Arguments:\n  <service>  Service to deploy\n  [region]   Target region\n  [extra]\n")
	assert.Less(t, strings.Index(help, "Usage:"), strings.Index(help, "Arguments:"), "arguments follow the usage line")
}

func TestDescribeArgumentsWithoutArgumentsLeavesHelpAlone(t *testing.T) {
	t.Parallel()
	cmd := newHelpCommand("tool")
	DescribeArguments(cmd, nil)
	assert.NotContains(t, helpText(t, cmd), "Arguments:")
}

func TestLabelListFlagsShowsListNotStrings(t *testing.T) {
	t.Parallel()
	cmd := newHelpCommand("tool")
	cmd.Flags().StringSlice("tags", nil, "Deploy tags")
	cmd.Flags().StringSlice("zones", []string{"a", "b"}, "Zones")
	cmd.Flags().String("name", "", "Name")
	require.Contains(t, helpText(t, cmd), "strings", "pflag labels lists as strings by default")

	require.NoError(t, cmd.Flags().Parse([]string{"--tags=x,y"}))
	LabelListFlags(cmd.Flags())
	help := helpText(t, cmd)
	assert.Contains(t, help, "--tags list")
	assert.Contains(t, help, "--zones list")
	assert.Contains(t, help, "--name string")
	assert.NotContains(t, help, "strings")
	assert.NotContains(t, help, "(default [])", "an unset list shows no default")
	assert.Contains(t, help, "(default [a,b])")
}

func TestValueErrorsStaySentinelCompatibleWithoutRepeatingIt(t *testing.T) {
	t.Parallel()
	_, err := ParseDecimalInt("abc")
	require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
	assert.Equal(t, "not a base-10 integer", err.Error())

	_, err = ParseDecimalInt("99999999999999999999")
	require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
	assert.Equal(t, fmt.Sprintf("out of range for a %d-bit integer", strconv.IntSize), err.Error())

	_, err = ParseStringList(`"open`)
	require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
}

func TestUsageErrorsDoNotRepeatTheInvalidValueLeadIn(t *testing.T) {
	t.Parallel()
	cmd := newHelpCommand("tool <service>")
	repeated := fmt.Errorf("%w: invalid value %q for flag --stage", errUtils.ErrInvalidFlagValue, "qa")
	err := NewUsageError(cmd, "./tool.star", repeated)
	assert.Equal(t, `usage error: invalid value "qa" for flag --stage`, err.Error())
	require.ErrorIs(t, err, errUtils.ErrScriptUsage)
	require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)

	other := NewUsageError(cmd, "./tool.star", errors.New("unknown flag: --bogus"))
	assert.Equal(t, "usage error: unknown flag: --bogus", other.Error())
}
