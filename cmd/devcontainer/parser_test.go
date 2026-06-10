package devcontainer

import (
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
)

func newBoundTestParser(t *testing.T, parser *flags.StandardFlagParser) *flags.StandardFlagParser {
	t.Helper()

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)
	require.NoError(t, parser.BindToViper(viper.New()))
	return parser
}

func parseTestArgs(t *testing.T, parser *flags.StandardFlagParser, args []string) *flags.ParsedConfig {
	t.Helper()

	parsed, err := parser.Parse(context.Background(), args)
	require.NoError(t, err)
	return parsed
}

func TestDevcontainerRequiredNameParsers(t *testing.T) {
	tests := []struct {
		name      string
		parser    func() *flags.StandardFlagParser
		args      []string
		wantFlags map[string]interface{}
	}{
		{
			name: "attach",
			parser: func() *flags.StandardFlagParser {
				parser, _ := newDevcontainerParser(
					true,
					flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
					flags.WithBoolFlag("pty", "", false, "Experimental: Use PTY mode with masking support (not available on Windows)"),
				)
				return parser
			},
			args: []string{"app", "--instance", "debug", "--pty"},
			wantFlags: map[string]interface{}{
				"instance": "debug",
				"pty":      true,
			},
		},
		{
			name: "config",
			parser: func() *flags.StandardFlagParser {
				parser, _ := newDevcontainerParser(true)
				return parser
			},
			args:      []string{"app"},
			wantFlags: map[string]interface{}{},
		},
		{
			name: "logs",
			parser: func() *flags.StandardFlagParser {
				parser, _ := newDevcontainerParser(
					true,
					flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
					flags.WithBoolFlag("follow", "f", false, "Follow log output"),
					flags.WithStringFlag("tail", "", "all", "Number of lines to show from the end of the logs"),
				)
				return parser
			},
			args: []string{"app", "--instance", "debug", "-f", "--tail", "50"},
			wantFlags: map[string]interface{}{
				"instance": "debug",
				"follow":   true,
				"tail":     "50",
			},
		},
		{
			name: "rebuild",
			parser: func() *flags.StandardFlagParser {
				parser, _ := newDevcontainerParser(
					true,
					flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
					flags.WithBoolFlag("attach", "", false, "Attach to the container after rebuilding"),
					flags.WithBoolFlag("no-pull", "", false, "Don't pull the latest image before rebuilding"),
					flags.WithIdentityFlag(),
				)
				return parser
			},
			args: []string{"app", "--instance", "debug", "--attach", "--no-pull", "--identity=dev"},
			wantFlags: map[string]interface{}{
				"instance": "debug",
				"attach":   true,
				"no-pull":  true,
				"identity": "dev",
			},
		},
		{
			name: "remove",
			parser: func() *flags.StandardFlagParser {
				parser, _ := newDevcontainerParser(
					true,
					flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
					flags.WithBoolFlag("force", "f", false, "Force remove even if running"),
				)
				return parser
			},
			args: []string{"app", "--instance", "debug", "--force"},
			wantFlags: map[string]interface{}{
				"instance": "debug",
				"force":    true,
			},
		},
		{
			name: "start",
			parser: func() *flags.StandardFlagParser {
				parser, _ := newDevcontainerParser(
					true,
					flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
					flags.WithBoolFlag("attach", "", false, "Attach to the container after starting"),
					flags.WithIdentityFlag(),
				)
				return parser
			},
			args: []string{"app", "--instance", "debug", "--attach", "--identity=dev"},
			wantFlags: map[string]interface{}{
				"instance": "debug",
				"attach":   true,
				"identity": "dev",
			},
		},
		{
			name: "stop",
			parser: func() *flags.StandardFlagParser {
				parser, _ := newDevcontainerParser(
					true,
					flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
					flags.WithIntFlag("timeout", "", defaultStopTimeout, "Timeout in seconds for stopping the container"),
					flags.WithBoolFlag("rm", "", false, "Automatically remove the container after stopping"),
				)
				return parser
			},
			args: []string{"app", "--instance", "debug", "--timeout", "30", "--rm"},
			wantFlags: map[string]interface{}{
				"instance": "debug",
				"timeout":  30,
				"rm":       true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			parser := newBoundTestParser(t, tt.parser())
			parsed := parseTestArgs(t, parser, tt.args)
			assert.Equal(t, []string{"app"}, parsed.PositionalArgs)
			assert.Empty(t, parsed.SeparatedArgs)

			for key, want := range tt.wantFlags {
				assert.Equal(t, want, parsed.Flags[key])
			}

			_, err := parser.Parse(context.Background(), []string{})
			require.Error(t, err)
			assert.True(t, errors.Is(err, errUtils.ErrInvalidPositionalArgs))
		})
	}
}

func TestDevcontainerShellParser(t *testing.T) {
	parser, _ := newDevcontainerParser(
		false,
		flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
		flags.WithIdentityFlag(),
		flags.WithBoolFlag("pty", "", false, "Experimental: Use PTY mode with masking support (not available on Windows)"),
		flags.WithBoolFlag(flagNew, "", false, "Create a new instance with auto-generated name"),
		flags.WithBoolFlag(flagReplace, "", false, "Destroy and recreate the current instance"),
		flags.WithBoolFlag("rm", "", false, "Automatically remove the container when the shell exits"),
		flags.WithBoolFlag("no-pull", "", false, "Skip pulling the image when using --replace (use cached image)"),
	)
	parser = newBoundTestParser(t, parser)

	parsed := parseTestArgs(t, parser, []string{"--instance", "debug", "--identity=dev", "--pty", "--new", "--rm", "--no-pull"})
	assert.Empty(t, parsed.PositionalArgs)
	assert.Equal(t, "debug", parsed.Flags["instance"])
	assert.Equal(t, "dev", parsed.Flags["identity"])
	assert.Equal(t, true, parsed.Flags["pty"])
	assert.Equal(t, true, parsed.Flags[flagNew])
	assert.Equal(t, false, parsed.Flags[flagReplace])
	assert.Equal(t, true, parsed.Flags["rm"])
	assert.Equal(t, true, parsed.Flags["no-pull"])

	parsed = parseTestArgs(t, parser, []string{"app"})
	assert.Equal(t, []string{"app"}, parsed.PositionalArgs)

	_, err := parser.Parse(context.Background(), []string{"app", "extra"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrInvalidPositionalArgs))
}

func TestDevcontainerExecParser(t *testing.T) {
	newExecTestParser := func(t *testing.T) *flags.StandardFlagParser {
		t.Helper()

		return newBoundTestParser(t, flags.NewStandardFlagParser(
			flags.WithStringFlag("instance", "", "default", "Instance name for this devcontainer"),
			flags.WithBoolFlag("interactive", "i", false, "Enable interactive TTY mode (disables output masking)"),
			flags.WithBoolFlag("pty", "", false, "Experimental: Use PTY mode with masking support (not available on Windows)"),
		))
	}

	t.Run("command after separator", func(t *testing.T) {
		parser := newExecTestParser(t)
		args := []string{"app", "--instance", "debug", "--", "sh", "-lc", "echo hi"}
		parsed := parseTestArgs(t, parser, args)
		name, command, err := parseExecInvocation(args, parsed)
		require.NoError(t, err)
		assert.Equal(t, "app", name)
		assert.Equal(t, []string{"sh", "-lc", "echo hi"}, command)
		assert.Equal(t, "debug", parsed.Flags["instance"])
	})

	t.Run("native flags after separator", func(t *testing.T) {
		parser := newExecTestParser(t)
		args := []string{"app", "--", "docker", "run", "--rm", "--pull=never", "alpine"}
		parsed := parseTestArgs(t, parser, args)
		name, command, err := parseExecInvocation(args, parsed)
		require.NoError(t, err)
		assert.Equal(t, "app", name)
		assert.Equal(t, []string{"docker", "run", "--rm", "--pull=never", "alpine"}, command)
	})

	t.Run("missing name before separator", func(t *testing.T) {
		parser := newExecTestParser(t)
		args := []string{"--", "sh"}
		parsed := parseTestArgs(t, parser, args)
		_, _, err := parseExecInvocation(args, parsed)
		require.Error(t, err)
		assert.True(t, errors.Is(err, errUtils.ErrInvalidArguments))
	})

	t.Run("multiple names before separator", func(t *testing.T) {
		parser := newExecTestParser(t)
		args := []string{"app", "extra", "--", "sh"}
		parsed := parseTestArgs(t, parser, args)
		_, _, err := parseExecInvocation(args, parsed)
		require.Error(t, err)
		assert.True(t, errors.Is(err, errUtils.ErrInvalidArguments))
	})

	t.Run("missing command after separator", func(t *testing.T) {
		parser := newExecTestParser(t)
		args := []string{"app", "--"}
		parsed := parseTestArgs(t, parser, args)
		_, _, err := parseExecInvocation(args, parsed)
		require.Error(t, err)
		assert.True(t, errors.Is(err, errUtils.ErrInvalidArguments))
	})

	t.Run("legacy no separator form", func(t *testing.T) {
		parser := newExecTestParser(t)
		args := []string{"app", "echo", "hello"}
		parsed := parseTestArgs(t, parser, args)
		name, command, err := parseExecInvocation(args, parsed)
		require.NoError(t, err)
		assert.Equal(t, "app", name)
		assert.Equal(t, []string{"echo", "hello"}, command)
	})
}
