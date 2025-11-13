package theme

import (
	"bytes"
	stdio "io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

func TestThemeCommand(t *testing.T) {
	t.Run("theme command exists", func(t *testing.T) {
		assert.Equal(t, "theme", themeCmd.Use)
		assert.NotEmpty(t, themeCmd.Short)
		assert.NotEmpty(t, themeCmd.Long)
	})

	t.Run("has list subcommand", func(t *testing.T) {
		hasListCmd := false
		for _, subCmd := range themeCmd.Commands() {
			if subCmd.Use == "list" {
				hasListCmd = true
				break
			}
		}
		assert.True(t, hasListCmd, "theme command should have list subcommand")
	})

	t.Run("has show subcommand", func(t *testing.T) {
		hasShowCmd := false
		for _, subCmd := range themeCmd.Commands() {
			if subCmd.Use == "show [theme-name]" {
				hasShowCmd = true
				break
			}
		}
		assert.True(t, hasShowCmd, "theme command should have show subcommand")
	})
}

func TestSetAtmosConfig(t *testing.T) {
	t.Run("sets config successfully", func(t *testing.T) {
		config := &schema.AtmosConfiguration{
			Settings: schema.AtmosSettings{
				Terminal: schema.Terminal{
					Theme: "dracula",
				},
			},
		}

		SetAtmosConfig(config)
		assert.Equal(t, config, atmosConfigPtr)
	})

	t.Run("handles nil config", func(t *testing.T) {
		SetAtmosConfig(nil)
		assert.Nil(t, atmosConfigPtr)
	})
}

func TestThemeCommandProvider(t *testing.T) {
	provider := &ThemeCommandProvider{}

	t.Run("GetCommand returns theme command", func(t *testing.T) {
		cmd := provider.GetCommand()
		assert.NotNil(t, cmd)
		assert.Equal(t, "theme", cmd.Use)
	})

	t.Run("GetName returns correct name", func(t *testing.T) {
		name := provider.GetName()
		assert.Equal(t, "theme", name)
	})

	t.Run("GetGroup returns correct group", func(t *testing.T) {
		group := provider.GetGroup()
		assert.Equal(t, "Other Commands", group)
	})
}

func TestThemeListCommand(t *testing.T) {
	t.Run("list command exists", func(t *testing.T) {
		assert.Equal(t, "list", themeListCmd.Use)
		assert.NotEmpty(t, themeListCmd.Short)
	})

	t.Run("has recommended flag", func(t *testing.T) {
		flag := themeListCmd.Flags().Lookup("recommended")
		require.NotNil(t, flag, "list command should have --recommended flag")
		assert.Equal(t, "bool", flag.Value.Type())
	})
}

func TestThemeShowCommand(t *testing.T) {
	t.Run("show command exists", func(t *testing.T) {
		assert.Equal(t, "show [theme-name]", themeShowCmd.Use)
		assert.NotEmpty(t, themeShowCmd.Short)
		assert.NotEmpty(t, themeShowCmd.Long)
	})

	t.Run("requires exactly one argument", func(t *testing.T) {
		// Validate Args is set to ExactArgs(1).
		err := themeShowCmd.Args(themeShowCmd, []string{})
		assert.Error(t, err, "show command should require exactly one argument")

		err = themeShowCmd.Args(themeShowCmd, []string{"dracula"})
		assert.NoError(t, err, "show command should accept one argument")

		err = themeShowCmd.Args(themeShowCmd, []string{"dracula", "extra"})
		assert.Error(t, err, "show command should reject more than one argument")
	})
}

// testStreams is a simple streams implementation for testing.
type testStreams struct {
	stdin  stdio.Reader
	stdout stdio.Writer
	stderr stdio.Writer
}

func (ts *testStreams) Input() stdio.Reader     { return ts.stdin }
func (ts *testStreams) Output() stdio.Writer    { return ts.stdout }
func (ts *testStreams) Error() stdio.Writer     { return ts.stderr }
func (ts *testStreams) RawOutput() stdio.Writer { return ts.stdout }
func (ts *testStreams) RawError() stdio.Writer  { return ts.stderr }

// setupTestUI creates test I/O context and initializes UI formatter.
//
//nolint:unparam // stdout may be used in future tests
func setupTestUI(t *testing.T) (stdout, stderr *bytes.Buffer, cleanup func()) {
	t.Helper()
	stdout = &bytes.Buffer{}
	stderr = &bytes.Buffer{}
	streams := &testStreams{
		stdin:  &bytes.Buffer{},
		stdout: stdout,
		stderr: stderr,
	}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	if err != nil {
		t.Fatalf("failed to create I/O context: %v", err)
	}

	// Initialize UI formatter.
	ui.InitFormatter(ioCtx)

	// Return cleanup function (no-op since we're in test).
	cleanup = func() {}

	return stdout, stderr, cleanup
}

func TestExecuteThemeList(t *testing.T) {
	tests := []struct {
		name        string
		atmosConfig *schema.AtmosConfiguration
		args        []string
		expectError bool
	}{
		{
			name:        "execute with nil atmosConfig",
			atmosConfig: nil,
			args:        []string{},
			expectError: false, // Should succeed (activeTheme will be empty)
		},
		{
			name: "execute with configured theme",
			atmosConfig: &schema.AtmosConfiguration{
				Settings: schema.AtmosSettings{
					Terminal: schema.Terminal{
						Theme: "dracula",
					},
				},
			},
			args:        []string{},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize UI formatter.
			_, _, cleanup := setupTestUI(t)
			defer cleanup()

			// Set atmosConfig for the command.
			SetAtmosConfig(tt.atmosConfig)

			// Execute the command.
			err := executeThemeList(themeListCmd, tt.args)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestExecuteThemeShow(t *testing.T) {
	tests := []struct {
		name        string
		themeName   string
		expectError bool
	}{
		{
			name:        "show valid theme",
			themeName:   "Dracula",
			expectError: false,
		},
		{
			name:        "show invalid theme",
			themeName:   "NonExistentTheme",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Initialize UI formatter.
			_, _, cleanup := setupTestUI(t)
			defer cleanup()

			// Execute the command.
			err := executeThemeShow(themeShowCmd, []string{tt.themeName})

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
