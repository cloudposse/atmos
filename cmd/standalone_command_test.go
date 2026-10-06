package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/script"
)

func TestStandaloneCommandNativeFlags(t *testing.T) {
	spec := script.CommandSpec{
		Name: "capacity",
		Args: []*flags.PositionalArgSpec{
			{Name: "service", Required: true},
			{Name: "region"},
		},
		Flags: []flags.Flag{
			&flags.IntFlag{Name: "replicas", Shorthand: "r", Default: 2},
			&flags.BoolFlag{Name: "verbose", Shorthand: "v"},
			&flags.StringFlag{Name: "config", Default: "default.json"},
			&flags.StringSliceFlag{Name: "tags", Default: []string{"default"}},
		},
	}
	file := &script.File{Path: "capacity.star", Args: []string{
		"api", "-r", "3", "-v", "--config", "script.json", "--tags", "blue,green", "--tags", "red",
	}}
	var stdout bytes.Buffer
	input, err := standaloneCommandParser(file, &stdout)(t.Context(), spec)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"service": "api", "region": nil}, input.Args)
	assert.Equal(t, map[string]any{
		"replicas": 3, "verbose": true, "config": "script.json", "tags": []string{"blue", "green", "red"},
	}, input.Flags)
	assert.False(t, input.Help)
	assert.Empty(t, stdout.String())

	input, err = standaloneCommandParser(&script.File{Path: file.Path, Args: []string{"worker"}}, &stdout)(t.Context(), spec)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"replicas": 2, "verbose": false, "config": "default.json", "tags": []string{"default"},
	}, input.Flags)
}

func TestStandaloneCommandHelpSkipsValidation(t *testing.T) {
	for _, help := range []string{"--help", "-h"} {
		t.Run(help, func(t *testing.T) {
			var stdout bytes.Buffer
			spec := script.CommandSpec{
				Name: "release", Description: "Prepare a release without shell parsing",
				Args:  []*flags.PositionalArgSpec{{Name: "service", Required: true}},
				Flags: []flags.Flag{&flags.StringFlag{Name: "stage", Required: true, Description: "Target stage"}},
			}
			input, err := standaloneCommandParser(&script.File{Path: "release.star", Args: []string{help}}, &stdout)(t.Context(), spec)
			require.NoError(t, err)
			assert.True(t, input.Help)
			assert.Nil(t, input.Args)
			assert.Nil(t, input.Flags)
			assert.Contains(t, stdout.String(), "Prepare a release without shell parsing")
			assert.Contains(t, stdout.String(), "release <service>")
			assert.Contains(t, stdout.String(), "--stage")
			assert.Contains(t, stdout.String(), "Target stage")
			assert.NotContains(t, stdout.String(), "--chdir")
		})
	}
}

func TestStandaloneCommandEnvironmentPrecedence(t *testing.T) {
	t.Setenv("STANDALONE_REPLICAS", "7")
	t.Setenv("STANDALONE_VERBOSE", "true")
	t.Setenv("STANDALONE_STAGE", "dev")
	t.Setenv("STANDALONE_TAGS", "blue green")
	spec := script.CommandSpec{Flags: []flags.Flag{
		&flags.IntFlag{Name: "replicas", Default: 2, EnvVars: []string{"STANDALONE_REPLICAS"}},
		&flags.BoolFlag{Name: "verbose", EnvVars: []string{"STANDALONE_VERBOSE"}},
		&flags.StringFlag{Name: "stage", Required: true, EnvVars: []string{"STANDALONE_STAGE"}, ValidValues: []string{"dev", "prod"}},
		&flags.StringSliceFlag{Name: "tags", EnvVars: []string{"STANDALONE_TAGS"}},
	}}
	var stdout bytes.Buffer
	input, err := standaloneCommandParser(&script.File{Path: "env.star"}, &stdout)(t.Context(), spec)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"replicas": 7, "verbose": true, "stage": "dev", "tags": []string{"blue", "green"},
	}, input.Flags)
	input, err = standaloneCommandParser(&script.File{Path: "env.star", Args: []string{
		"--replicas=4", "--verbose=false", "--stage=prod", "--tags=red",
	}}, &stdout)(t.Context(), spec)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{
		"replicas": 4, "verbose": false, "stage": "prod", "tags": []string{"red"},
	}, input.Flags)
}

func TestStandaloneCommandValidation(t *testing.T) {
	tests := []struct {
		name string
		spec script.CommandSpec
		argv []string
		want string
	}{
		{name: "unknown flag", argv: []string{"--config=atmos.yaml"}, want: "unknown flag: --config"},
		{name: "integer", spec: script.CommandSpec{Flags: []flags.Flag{&flags.IntFlag{Name: "replicas"}}}, argv: []string{"--replicas=lots"}, want: "invalid argument"},
		{name: "boolean", spec: script.CommandSpec{Flags: []flags.Flag{&flags.BoolFlag{Name: "force"}}}, argv: []string{"--force=perhaps"}, want: "invalid argument"},
		{name: "required flag", spec: script.CommandSpec{Flags: []flags.Flag{&flags.StringFlag{Name: "stage", Required: true}}}, want: "stage"},
		{name: "choices", spec: script.CommandSpec{Flags: []flags.Flag{&flags.StringFlag{Name: "stage", ValidValues: []string{"dev", "prod"}}}}, argv: []string{"--stage=invalid"}, want: "invalid"},
		{name: "slice choices", spec: script.CommandSpec{Flags: []flags.Flag{&flags.StringSliceFlag{Name: "tags", ValidValues: []string{"blue", "green"}}}}, argv: []string{"--tags=blue,red"}, want: "red"},
		{name: "required argument", spec: script.CommandSpec{Args: []*flags.PositionalArgSpec{{Name: "service", Required: true}}}, want: "accepts 1 arg(s)"},
		{name: "surplus argument", spec: script.CommandSpec{Args: []*flags.PositionalArgSpec{{Name: "service"}}}, argv: []string{"api", "worker"}, want: "at most 1 arg(s)"},
		{name: "no arguments", argv: []string{"extra"}, want: "accepts 0 arg(s)"},
		{name: "surplus after separator", spec: script.CommandSpec{Args: []*flags.PositionalArgSpec{{Name: "service"}}}, argv: []string{"api", "--", "worker"}, want: "at most 1 arg(s)"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			_, err := standaloneCommandParser(&script.File{Path: "test.star", Args: test.argv}, &stdout)(t.Context(), test.spec)
			require.ErrorContains(t, err, test.want)
		})
	}
}

func TestStandaloneCommandSeparatorAndIsolation(t *testing.T) {
	previous := viper.Get("config")
	viper.Set("config", "global-config")
	t.Cleanup(func() { viper.Set("config", previous) })
	var stdout bytes.Buffer
	spec := script.CommandSpec{
		Args:  []*flags.PositionalArgSpec{{Name: "literal", Required: true}},
		Flags: []flags.Flag{&flags.StringFlag{Name: "config", Default: "script-config"}},
	}
	for _, literal := range []string{"--help", "--config=literal", "--"} {
		input, err := standaloneCommandParser(&script.File{Path: "script.star", Args: []string{"--", literal}}, &stdout)(t.Context(), spec)
		require.NoError(t, err)
		assert.Equal(t, literal, input.Args["literal"])
		assert.Equal(t, "script-config", input.Flags["config"])
		assert.False(t, input.Help)
	}
	assert.Equal(t, "global-config", viper.GetString("config"))
}

func TestStandaloneCommandInvalidEnvironmentChoice(t *testing.T) {
	t.Setenv("STANDALONE_STAGE", "invalid")
	spec := script.CommandSpec{Flags: []flags.Flag{
		&flags.StringFlag{Name: "stage", EnvVars: []string{"STANDALONE_STAGE"}, ValidValues: []string{"dev", "prod"}},
	}}
	var stdout bytes.Buffer
	_, err := standaloneCommandParser(&script.File{Path: "env.star"}, &stdout)(t.Context(), spec)
	require.ErrorContains(t, err, "invalid")
}

func TestStandaloneCommandMalformedSpecs(t *testing.T) {
	var typedNil *flags.StringFlag
	tests := []script.CommandSpec{
		{Flags: []flags.Flag{nil}},
		{Flags: []flags.Flag{typedNil}},
		{Flags: []flags.Flag{&flags.StringArrayFlag{}}},
		{Flags: []flags.Flag{&flags.StringFlag{}}},
		{Flags: []flags.Flag{&flags.StringFlag{Name: "--bad"}}},
		{Flags: []flags.Flag{&flags.StringFlag{Name: "bad name"}}},
		{Flags: []flags.Flag{&flags.StringFlag{Name: "help"}}},
		{Flags: []flags.Flag{&flags.StringFlag{Name: "x"}, &flags.StringFlag{Name: "x"}}},
		{Flags: []flags.Flag{&flags.StringFlag{Name: "x", Shorthand: "ab"}}},
		{Flags: []flags.Flag{&flags.StringFlag{Name: "x", Shorthand: "h"}}},
		{Flags: []flags.Flag{&flags.StringFlag{Name: "x", Shorthand: "-"}}},
		{Flags: []flags.Flag{&flags.StringFlag{Name: "x", Shorthand: "v"}, &flags.StringFlag{Name: "y", Shorthand: "v"}}},
		{Args: []*flags.PositionalArgSpec{nil}},
		{Args: []*flags.PositionalArgSpec{{}}},
		{Args: []*flags.PositionalArgSpec{{Name: "x"}, {Name: "x"}}},
		{Args: []*flags.PositionalArgSpec{{Name: "x"}, {Name: "y", Required: true}}},
	}
	for _, spec := range tests {
		var stdout bytes.Buffer
		require.NotPanics(t, func() {
			_, err := standaloneCommandParser(&script.File{Path: "bad.star"}, &stdout)(t.Context(), spec)
			require.Error(t, err)
		})
	}
}

func TestStandaloneCommandCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	var stdout bytes.Buffer
	_, err := standaloneCommandParser(&script.File{Path: "test.star"}, &stdout)(ctx, script.CommandSpec{})
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, stdout.String())
}

func TestStandaloneCommandEmptyRequiredFlag(t *testing.T) {
	var stdout bytes.Buffer
	_, err := standaloneCommandParser(&script.File{Path: "test.star", Args: []string{"--stage="}}, &stdout)(t.Context(), script.CommandSpec{
		Flags: []flags.Flag{&flags.StringFlag{Name: "stage", Required: true}},
	})
	require.ErrorIs(t, err, errUtils.ErrRequiredFlagEmpty)
}

func TestStandaloneCommandHelpSkipsScriptCallbacks(t *testing.T) {
	NewTestKit(t)
	path := filepath.Join(t.TempDir(), "help.star")
	source := `def validate(args, flags):
    fail("validation must not run for help")
def main(args, flags):
    fail("main must not run for help")
cli.command(
    description = "Script-owned command help",
    args = [cli.arg("service", required=True)],
    flags = [cli.flag("stage", required=True)],
    validate = validate,
    run = main,
)
`
	require.NoError(t, os.WriteFile(path, []byte(source), 0o600))
	command := &cobra.Command{}
	command.SetContext(t.Context())
	require.NoError(t, runStandaloneScript(command, &script.File{Path: path, Args: []string{"--help"}}))
}

func TestStandaloneCommandRequiredTypedFlags(t *testing.T) {
	t.Setenv("STANDALONE_REQUIRED_COUNT", "")
	t.Setenv("STANDALONE_REQUIRED_TAGS", "")
	spec := script.CommandSpec{Flags: []flags.Flag{
		&flags.IntFlag{Name: "count", Required: true, EnvVars: []string{"STANDALONE_REQUIRED_COUNT"}},
		&flags.StringSliceFlag{Name: "tags", Required: true, EnvVars: []string{"STANDALONE_REQUIRED_TAGS"}},
	}}
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{name: "missing both"},
		{name: "missing count", argv: []string{"--tags=blue"}, want: "--count"},
		{name: "missing tags", argv: []string{"--count=0"}, want: "--tags"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var stdout bytes.Buffer
			_, err := standaloneCommandParser(&script.File{Path: "required.star", Args: test.argv}, &stdout)(t.Context(), spec)
			require.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
			assert.Contains(t, err.Error(), test.want)
		})
	}
	var stdout bytes.Buffer
	parse := func(argv ...string) (script.CommandInput, error) {
		return standaloneCommandParser(&script.File{Path: "required.star", Args: argv}, &stdout)(t.Context(), spec)
	}
	input, err := parse("--count=0", "--tags=")
	require.NoError(t, err)
	assert.Equal(t, 0, input.Flags["count"])
	assert.Empty(t, input.Flags["tags"])
	input, err = parse("--help")
	require.NoError(t, err)
	assert.True(t, input.Help)
	t.Setenv("STANDALONE_REQUIRED_COUNT", "0")
	t.Setenv("STANDALONE_REQUIRED_TAGS", "blue green")
	input, err = parse()
	require.NoError(t, err)
	assert.Equal(t, 0, input.Flags["count"])
	assert.Equal(t, []string{"blue", "green"}, input.Flags["tags"])
}

func TestStandaloneCommandRejectsMalformedTypedEnvironment(t *testing.T) {
	tests := []struct {
		name string
		flag flags.Flag
		bad  string
		cli  string
		want any
	}{
		{name: "integer", flag: &flags.IntFlag{Name: "value", EnvVars: []string{"STANDALONE_BAD_VALUE"}}, bad: "invalid", cli: "--value=0", want: 0},
		{name: "boolean", flag: &flags.BoolFlag{Name: "value", EnvVars: []string{"STANDALONE_BAD_VALUE"}}, bad: "perhaps", cli: "--value=false", want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Setenv("STANDALONE_BAD_VALUE", test.bad)
			var stdout bytes.Buffer
			spec := script.CommandSpec{Flags: []flags.Flag{test.flag}}
			_, err := standaloneCommandParser(&script.File{Path: "env.star"}, &stdout)(t.Context(), spec)
			require.ErrorIs(t, err, errUtils.ErrInvalidFlagValue)
			assert.Contains(t, err.Error(), "environment value for --value")
			input, err := standaloneCommandParser(&script.File{Path: "env.star", Args: []string{test.cli}}, &stdout)(t.Context(), spec)
			require.NoError(t, err)
			assert.Equal(t, test.want, input.Flags["value"])
			input, err = standaloneCommandParser(&script.File{Path: "env.star", Args: []string{"--help"}}, &stdout)(t.Context(), spec)
			require.NoError(t, err)
			assert.True(t, input.Help)
		})
	}
}

func TestStandaloneCommandEnvironmentBindingOrder(t *testing.T) {
	t.Setenv("STANDALONE_FIRST", "")
	t.Setenv("STANDALONE_SECOND", "7")
	spec := script.CommandSpec{Flags: []flags.Flag{
		&flags.IntFlag{Name: "count", Required: true, EnvVars: []string{"STANDALONE_FIRST", "STANDALONE_SECOND"}},
	}}
	var stdout bytes.Buffer
	parse := standaloneCommandParser(&script.File{Path: "env.star"}, &stdout)
	input, err := parse(t.Context(), spec)
	require.NoError(t, err)
	assert.Equal(t, 7, input.Flags["count"])
	t.Setenv("STANDALONE_FIRST", "3")
	input, err = parse(t.Context(), spec)
	require.NoError(t, err)
	assert.Equal(t, 3, input.Flags["count"])
}
