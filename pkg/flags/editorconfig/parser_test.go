package editorconfig

import (
	"context"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEditorConfigParser_Parse(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected *EditorConfigOptions
		wantErr  bool
	}{
		{
			name: "default values",
			args: []string{},
			expected: &EditorConfigOptions{
				Exclude:                       "",
				Init:                          false,
				IgnoreDefaults:                false,
				DryRun:                        false,
				ShowVersion:                   false,
				Format:                        "",
				DisableEndOfLine:              false,
				DisableTrimTrailingWhitespace: false,
				DisableInsertFinalNewline:     false,
				DisableIndentation:            false,
				DisableIndentSize:             false,
				DisableMaxLineLength:          false,
			},
		},
		{
			name: "with exclude flag",
			args: []string{"--exclude", "node_modules"},
			expected: &EditorConfigOptions{
				Exclude: "node_modules",
			},
		},
		{
			name: "with init flag",
			args: []string{"--init"},
			expected: &EditorConfigOptions{
				Init: true,
			},
		},
		{
			name: "with ignore-defaults flag",
			args: []string{"--ignore-defaults"},
			expected: &EditorConfigOptions{
				IgnoreDefaults: true,
			},
		},
		{
			name: "with dry-run flag",
			args: []string{"--dry-run"},
			expected: &EditorConfigOptions{
				DryRun: true,
			},
		},
		{
			name: "with show-version flag",
			args: []string{"--show-version"},
			expected: &EditorConfigOptions{
				ShowVersion: true,
			},
		},
		{
			name: "with format flag",
			args: []string{"--format", "json"},
			expected: &EditorConfigOptions{
				Format: "json",
			},
		},
		{
			name: "with disable-end-of-line flag",
			args: []string{"--disable-end-of-line"},
			expected: &EditorConfigOptions{
				DisableEndOfLine: true,
			},
		},
		{
			name: "with disable-trim-trailing-whitespace flag",
			args: []string{"--disable-trim-trailing-whitespace"},
			expected: &EditorConfigOptions{
				DisableTrimTrailingWhitespace: true,
			},
		},
		{
			name: "with disable-insert-final-newline flag",
			args: []string{"--disable-insert-final-newline"},
			expected: &EditorConfigOptions{
				DisableInsertFinalNewline: true,
			},
		},
		{
			name: "with disable-indentation flag",
			args: []string{"--disable-indentation"},
			expected: &EditorConfigOptions{
				DisableIndentation: true,
			},
		},
		{
			name: "with disable-indent-size flag",
			args: []string{"--disable-indent-size"},
			expected: &EditorConfigOptions{
				DisableIndentSize: true,
			},
		},
		{
			name: "with disable-max-line-length flag",
			args: []string{"--disable-max-line-length"},
			expected: &EditorConfigOptions{
				DisableMaxLineLength: true,
			},
		},
		{
			name: "with multiple flags",
			args: []string{
				"--init",
				"--exclude", "*.test",
				"--format", "yaml",
				"--disable-end-of-line",
				"--disable-indentation",
			},
			expected: &EditorConfigOptions{
				Exclude:            "*.test",
				Init:               true,
				Format:             "yaml",
				DisableEndOfLine:   true,
				DisableIndentation: true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper for each test.
			v := viper.New()

			// Create parser via builder.
			parser := NewEditorConfigOptionsBuilder().
				WithExclude().
				WithInit().
				WithIgnoreDefaults().
				WithDryRun().
				WithShowVersion().
				WithFormat("").
				WithDisableEndOfLine().
				WithDisableTrimTrailingWhitespace().
				WithDisableInsertFinalNewline().
				WithDisableIndentation().
				WithDisableIndentSize().
				WithDisableMaxLineLength().
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
			assert.Equal(t, tt.expected.Exclude, opts.Exclude)
			assert.Equal(t, tt.expected.Init, opts.Init)
			assert.Equal(t, tt.expected.IgnoreDefaults, opts.IgnoreDefaults)
			assert.Equal(t, tt.expected.DryRun, opts.DryRun)
			assert.Equal(t, tt.expected.ShowVersion, opts.ShowVersion)
			assert.Equal(t, tt.expected.Format, opts.Format)
			assert.Equal(t, tt.expected.DisableEndOfLine, opts.DisableEndOfLine)
			assert.Equal(t, tt.expected.DisableTrimTrailingWhitespace, opts.DisableTrimTrailingWhitespace)
			assert.Equal(t, tt.expected.DisableInsertFinalNewline, opts.DisableInsertFinalNewline)
			assert.Equal(t, tt.expected.DisableIndentation, opts.DisableIndentation)
			assert.Equal(t, tt.expected.DisableIndentSize, opts.DisableIndentSize)
			assert.Equal(t, tt.expected.DisableMaxLineLength, opts.DisableMaxLineLength)
		})
	}
}

func TestEditorConfigBuilder_Methods(t *testing.T) {
	t.Run("NewEditorConfigOptionsBuilder creates empty builder", func(t *testing.T) {
		builder := NewEditorConfigOptionsBuilder()
		assert.NotNil(t, builder)
		assert.NotNil(t, builder.options)
	})

	t.Run("WithExclude adds exclude flag", func(t *testing.T) {
		builder := NewEditorConfigOptionsBuilder().WithExclude()
		assert.NotNil(t, builder)
	})

	t.Run("WithInit adds init flag", func(t *testing.T) {
		builder := NewEditorConfigOptionsBuilder().WithInit()
		assert.NotNil(t, builder)
	})

	t.Run("WithIgnoreDefaults adds ignore-defaults flag", func(t *testing.T) {
		builder := NewEditorConfigOptionsBuilder().WithIgnoreDefaults()
		assert.NotNil(t, builder)
	})

	t.Run("WithDryRun adds dry-run flag", func(t *testing.T) {
		builder := NewEditorConfigOptionsBuilder().WithDryRun()
		assert.NotNil(t, builder)
	})

	t.Run("WithShowVersion adds show-version flag", func(t *testing.T) {
		builder := NewEditorConfigOptionsBuilder().WithShowVersion()
		assert.NotNil(t, builder)
	})

	t.Run("WithFormat adds format flag", func(t *testing.T) {
		builder := NewEditorConfigOptionsBuilder().WithFormat("")
		assert.NotNil(t, builder)
	})

	t.Run("Build creates parser", func(t *testing.T) {
		parser := NewEditorConfigOptionsBuilder().
			WithExclude().
			WithInit().
			WithFormat("").
			Build()
		assert.NotNil(t, parser)
		assert.NotNil(t, parser.parser)
	})
}

func TestEditorConfigParser_RegisterFlagsAndBindToViper(t *testing.T) {
	parser := NewEditorConfigOptionsBuilder().
		WithExclude().
		WithInit().
		WithIgnoreDefaults().
		WithDryRun().
		WithShowVersion().
		WithFormat("").
		WithDisableEndOfLine().
		WithDisableTrimTrailingWhitespace().
		WithDisableInsertFinalNewline().
		WithDisableIndentation().
		WithDisableIndentSize().
		WithDisableMaxLineLength().
		Build()

	cmd := &cobra.Command{Use: "test"}
	parser.RegisterFlags(cmd)

	// Check flags were registered.
	assert.NotNil(t, cmd.Flags().Lookup("exclude"))
	assert.NotNil(t, cmd.Flags().Lookup("init"))
	assert.NotNil(t, cmd.Flags().Lookup("ignore-defaults"))
	assert.NotNil(t, cmd.Flags().Lookup("dry-run"))
	assert.NotNil(t, cmd.Flags().Lookup("show-version"))
	assert.NotNil(t, cmd.Flags().Lookup("format"))
	assert.NotNil(t, cmd.Flags().Lookup("disable-end-of-line"))
	assert.NotNil(t, cmd.Flags().Lookup("disable-trim-trailing-whitespace"))
	assert.NotNil(t, cmd.Flags().Lookup("disable-insert-final-newline"))
	assert.NotNil(t, cmd.Flags().Lookup("disable-indentation"))
	assert.NotNil(t, cmd.Flags().Lookup("disable-indent-size"))
	assert.NotNil(t, cmd.Flags().Lookup("disable-max-line-length"))

	// Check binding to viper.
	v := viper.New()
	err := parser.BindToViper(v)
	assert.NoError(t, err)
}
