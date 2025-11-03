package flags

import (
	"context"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAuthParser_Parse(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		envVars  map[string]string
		expected *AuthOptions
		wantErr  bool
	}{
		{
			name: "default values",
			args: []string{},
			expected: &AuthOptions{
				GlobalFlags: GlobalFlags{
					LogsLevel: "Info",
					LogsFile:  "/dev/stderr",
				},
				Verbose:     false,
				Output:      "",
				Destination: "",
				Duration:    0,
				Issuer:      "",
				PrintOnly:   false,
				NoOpen:      false,
			},
		},
		{
			name: "with verbose flag",
			args: []string{"--verbose"},
			expected: &AuthOptions{
				GlobalFlags: GlobalFlags{
					LogsLevel: "Info",
					LogsFile:  "/dev/stderr",
				},
				Verbose: true,
			},
		},
		{
			name: "with output flag",
			args: []string{"--output", "json"},
			expected: &AuthOptions{
				GlobalFlags: GlobalFlags{
					LogsLevel: "Info",
					LogsFile:  "/dev/stderr",
				},
				Output: "json",
			},
		},
		{
			name: "with destination flag",
			args: []string{"--destination", "ec2"},
			expected: &AuthOptions{
				GlobalFlags: GlobalFlags{
					LogsLevel: "Info",
					LogsFile:  "/dev/stderr",
				},
				Destination: "ec2",
			},
		},
		{
			name: "with duration flag",
			args: []string{"--duration", "2h"},
			expected: &AuthOptions{
				GlobalFlags: GlobalFlags{
					LogsLevel: "Info",
					LogsFile:  "/dev/stderr",
				},
				Duration: 2 * time.Hour,
			},
		},
		{
			name: "with issuer flag",
			args: []string{"--issuer", "myissuer"},
			expected: &AuthOptions{
				GlobalFlags: GlobalFlags{
					LogsLevel: "Info",
					LogsFile:  "/dev/stderr",
				},
				Issuer: "myissuer",
			},
		},
		{
			name: "with print-only flag",
			args: []string{"--print-only"},
			expected: &AuthOptions{
				GlobalFlags: GlobalFlags{
					LogsLevel: "Info",
					LogsFile:  "/dev/stderr",
				},
				PrintOnly: true,
			},
		},
		{
			name: "with no-open flag",
			args: []string{"--no-open"},
			expected: &AuthOptions{
				GlobalFlags: GlobalFlags{
					LogsLevel: "Info",
					LogsFile:  "/dev/stderr",
				},
				NoOpen: true,
			},
		},
		{
			name: "with multiple flags",
			args: []string{"--verbose", "--output", "table", "--destination", "s3", "--duration", "1h30m"},
			expected: &AuthOptions{
				GlobalFlags: GlobalFlags{
					LogsLevel: "Info",
					LogsFile:  "/dev/stderr",
				},
				Verbose:     true,
				Output:      "table",
				Destination: "s3",
				Duration:    90 * time.Minute,
			},
		},
		{
			name:    "with environment variable",
			args:    []string{},
			envVars: map[string]string{"ATMOS_OUTPUT": "json"},
			expected: &AuthOptions{
				GlobalFlags: GlobalFlags{
					LogsLevel: "Info",
					LogsFile:  "/dev/stderr",
				},
				Output: "json",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper for each test.
			v := viper.New()

			// Set environment variables.
			for k, val := range tt.envVars {
				t.Setenv(k, val)
				_ = v.BindEnv(k)
			}

			// Create parser and command.
			parser := NewAuthOptionsBuilder().
				WithVerbose().
				WithOutput("").
				WithDestination().
				WithDuration(""). // Empty default so tests can verify zero values
				WithIssuer("").
				WithPrintOnly().
				WithNoOpen().
				Build()

			cmd := &cobra.Command{Use: "test"}
			parser.RegisterFlags(cmd)
			require.NoError(t, parser.BindToViper(v))

			// Set args and parse.
			cmd.SetArgs(tt.args)
			require.NoError(t, cmd.Execute())

			// Parse options.
			opts, err := parser.Parse(context.Background(), tt.args)
			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, opts)

			// Check expected values.
			assert.Equal(t, tt.expected.Verbose, opts.Verbose)
			assert.Equal(t, tt.expected.Output, opts.Output)
			assert.Equal(t, tt.expected.Destination, opts.Destination)
			assert.Equal(t, tt.expected.Duration, opts.Duration)
			assert.Equal(t, tt.expected.Issuer, opts.Issuer)
			assert.Equal(t, tt.expected.PrintOnly, opts.PrintOnly)
			assert.Equal(t, tt.expected.NoOpen, opts.NoOpen)
		})
	}
}

func TestAuthBuilder_Methods(t *testing.T) {
	t.Run("NewAuthOptionsBuilder creates empty builder", func(t *testing.T) {
		builder := NewAuthOptionsBuilder()
		assert.NotNil(t, builder)
		assert.NotNil(t, builder.options)
	})

	t.Run("WithVerbose adds verbose flag", func(t *testing.T) {
		builder := NewAuthOptionsBuilder().WithVerbose()
		assert.NotNil(t, builder)
	})

	t.Run("WithOutput adds output flag", func(t *testing.T) {
		builder := NewAuthOptionsBuilder().WithOutput("table")
		assert.NotNil(t, builder)
	})

	t.Run("WithDestination adds destination flag", func(t *testing.T) {
		builder := NewAuthOptionsBuilder().WithDestination()
		assert.NotNil(t, builder)
	})

	t.Run("WithDuration adds duration flag", func(t *testing.T) {
		builder := NewAuthOptionsBuilder().WithDuration("1h")
		assert.NotNil(t, builder)
	})

	t.Run("WithIssuer adds issuer flag", func(t *testing.T) {
		builder := NewAuthOptionsBuilder().WithIssuer("test")
		assert.NotNil(t, builder)
	})

	t.Run("WithPrintOnly adds print-only flag", func(t *testing.T) {
		builder := NewAuthOptionsBuilder().WithPrintOnly()
		assert.NotNil(t, builder)
	})

	t.Run("WithNoOpen adds no-open flag", func(t *testing.T) {
		builder := NewAuthOptionsBuilder().WithNoOpen()
		assert.NotNil(t, builder)
	})

	t.Run("Build creates parser", func(t *testing.T) {
		parser := NewAuthOptionsBuilder().
			WithVerbose().
			WithOutput("table").
			Build()
		assert.NotNil(t, parser)
		assert.NotNil(t, parser.parser)
	})
}

func TestAuthParser_RegisterFlagsAndBindToViper(t *testing.T) {
	parser := NewAuthOptionsBuilder().
		WithVerbose().
		WithOutput("table").
		Build()

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Check flags were registered.
	assert.NotNil(t, cmd.Flags().Lookup("verbose"))
	assert.NotNil(t, cmd.Flags().Lookup("output"))

	// Check binding to viper.
	v := viper.New()
	err := parser.BindToViper(v)
	assert.NoError(t, err)
}

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected time.Duration
	}{
		{"empty string", "", 0},
		{"1 hour", "1h", 1 * time.Hour},
		{"30 minutes", "30m", 30 * time.Minute},
		{"1h30m", "1h30m", 90 * time.Minute},
		{"invalid", "invalid", 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseDuration(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIdentityFlag_Methods(t *testing.T) {
	t.Run("NewIdentityFlag creates flag with value", func(t *testing.T) {
		flag := NewIdentityFlag("test-identity")
		assert.Equal(t, "test-identity", flag.Value())
		assert.False(t, flag.IsInteractiveSelector())
		assert.False(t, flag.IsEmpty())
	})

	t.Run("IsInteractiveSelector detects __SELECT__", func(t *testing.T) {
		flag := NewIdentityFlag("__SELECT__")
		assert.True(t, flag.IsInteractiveSelector())
		assert.False(t, flag.IsEmpty())
	})

	t.Run("IsEmpty detects empty string", func(t *testing.T) {
		flag := NewIdentityFlag("")
		assert.True(t, flag.IsEmpty())
		assert.False(t, flag.IsInteractiveSelector())
	})
}
