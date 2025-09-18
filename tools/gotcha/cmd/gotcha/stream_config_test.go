package cmd

import (
	"os"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/tools/gotcha/cmd/gotcha/constants"
)

// TestExtractStreamConfig tests the extractStreamConfig function
func TestExtractStreamConfig(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		setup      func(cmd *cobra.Command)
		setupViper func()
		setupEnv   func()
		cleanupEnv func()
		want       func(t *testing.T, config *StreamConfig)
		wantErr    bool
	}{
		{
			name: "default configuration",
			args: []string{},
			setup: func(cmd *cobra.Command) {
				// No flags set
			},
			want: func(t *testing.T, config *StreamConfig) {
				assert.Equal(t, DefaultTestPath, config.TestPath)
				assert.Equal(t, DefaultShowFilter, config.ShowFilter)
				assert.Equal(t, constants.FormatTerminal, config.Format)
				assert.False(t, config.Cover)
				assert.False(t, config.CIMode)
			},
		},
		{
			name: "custom test path from args",
			args: []string{"./cmd/..."},
			setup: func(cmd *cobra.Command) {
				// No flags needed
			},
			want: func(t *testing.T, config *StreamConfig) {
				assert.Equal(t, "./cmd/...", config.TestPath)
			},
		},
		{
			name: "show filter from flag",
			args: []string{},
			setup: func(cmd *cobra.Command) {
				cmd.Flags().Set("show", "failed")
			},
			want: func(t *testing.T, config *StreamConfig) {
				assert.Equal(t, "failed", config.ShowFilter)
			},
		},
		{
			name: "show filter from viper when flag not changed",
			args: []string{},
			setupViper: func() {
				viper.Set("show", "failed")
			},
			want: func(t *testing.T, config *StreamConfig) {
				assert.Equal(t, "failed", config.ShowFilter)
			},
		},
		{
			name: "format from flag",
			args: []string{},
			setup: func(cmd *cobra.Command) {
				cmd.Flags().Set("format", "markdown")
			},
			want: func(t *testing.T, config *StreamConfig) {
				assert.Equal(t, "markdown", config.Format)
			},
		},
		{
			name: "format from viper",
			args: []string{},
			setupViper: func() {
				viper.Set("format", "json")
			},
			want: func(t *testing.T, config *StreamConfig) {
				assert.Equal(t, "json", config.Format)
			},
		},
		{
			name: "coverage enabled from flag",
			args: []string{},
			setup: func(cmd *cobra.Command) {
				cmd.Flags().Set("cover", "true")
			},
			want: func(t *testing.T, config *StreamConfig) {
				assert.True(t, config.Cover)
				assert.NotEmpty(t, config.CoverProfile) // Should auto-generate
			},
		},
		{
			name: "coverage profile from flag",
			args: []string{},
			setup: func(cmd *cobra.Command) {
				cmd.Flags().Set("coverprofile", "custom.out")
			},
			want: func(t *testing.T, config *StreamConfig) {
				assert.True(t, config.Cover) // Should be enabled when profile is set
				assert.Equal(t, "custom.out", config.CoverProfile)
			},
		},
		{
			name: "CI mode from flag",
			args: []string{},
			setup: func(cmd *cobra.Command) {
				cmd.Flags().Set("ci", "true")
			},
			want: func(t *testing.T, config *StreamConfig) {
				assert.True(t, config.CIMode)
				// Format should switch to markdown in CI
				assert.Equal(t, "markdown", config.Format)
			},
		},
		{
			name: "CI mode auto-detected from environment",
			args: []string{},
			setupEnv: func() {
				os.Setenv("CI", "true")
				// Need to reinitialize viper bindings after setting env vars
				viper.Reset()
				viper.BindEnv("runtime.ci", "CI")
			},
			cleanupEnv: func() {
				os.Unsetenv("CI")
				viper.Reset()
			},
			want: func(t *testing.T, config *StreamConfig) {
				assert.True(t, config.CIMode)
			},
		},
		{
			name: "GitHub Actions detected",
			args: []string{},
			setupEnv: func() {
				os.Setenv("GITHUB_ACTIONS", "true")
				// Need to reinitialize viper bindings after setting env vars
				viper.Reset()
				viper.BindEnv("runtime.github.actions", "GITHUB_ACTIONS")
			},
			cleanupEnv: func() {
				os.Unsetenv("GITHUB_ACTIONS")
				viper.Reset()
			},
			want: func(t *testing.T, config *StreamConfig) {
				assert.True(t, config.CIMode)
			},
		},
		{
			name: "output file from flag",
			args: []string{},
			setup: func(cmd *cobra.Command) {
				cmd.Flags().Set("output", "results.json")
			},
			want: func(t *testing.T, config *StreamConfig) {
				assert.Equal(t, "results.json", config.OutputFile)
			},
		},
		{
			name: "output file auto-determined for markdown format",
			args: []string{},
			setup: func(cmd *cobra.Command) {
				cmd.Flags().Set("format", "markdown")
			},
			want: func(t *testing.T, config *StreamConfig) {
				assert.Equal(t, DefaultOutputMD, config.OutputFile)
			},
		},
		{
			name: "output file auto-determined for json format",
			args: []string{},
			setup: func(cmd *cobra.Command) {
				cmd.Flags().Set("format", "json")
			},
			want: func(t *testing.T, config *StreamConfig) {
				assert.Equal(t, DefaultOutputJSON, config.OutputFile)
			},
		},
		{
			name: "post strategy from environment",
			args: []string{},
			setupEnv: func() {
				os.Setenv("POST_COMMENT", "always")
			},
			cleanupEnv: func() {
				os.Unsetenv("POST_COMMENT")
			},
			want: func(t *testing.T, config *StreamConfig) {
				assert.Equal(t, "always", config.PostStrategy)
			},
		},
		{
			name: "verbosity affects show filter",
			args: []string{},
			setup: func(cmd *cobra.Command) {
				cmd.Flags().Set("verbosity", "minimal")
			},
			want: func(t *testing.T, config *StreamConfig) {
				assert.Equal(t, "failed", config.ShowFilter)
			},
		},
		{
			name: "verbosity verbose shows all",
			args: []string{},
			setup: func(cmd *cobra.Command) {
				cmd.Flags().Set("verbosity", "verbose")
			},
			want: func(t *testing.T, config *StreamConfig) {
				assert.Equal(t, "all", config.ShowFilter)
			},
		},
		{
			name: "exclude mocks from viper",
			args: []string{},
			setupViper: func() {
				viper.Set("exclude-mocks", true)
			},
			want: func(t *testing.T, config *StreamConfig) {
				assert.True(t, config.ExcludeMocks)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper for each test
			viper.Reset()

			// Create a test logger
			logger := log.New(os.Stderr)
			logger.SetLevel(log.DebugLevel)

			// Create a test command with all flags
			cmd := &cobra.Command{}
			cmd.Flags().String("show", "", "Show filter")
			cmd.Flags().String("format", "", "Output format")
			cmd.Flags().String("output", "", "Output file")
			cmd.Flags().Bool("alert", false, "Alert on failure")
			cmd.Flags().String("verbosity", "", "Verbosity level")
			cmd.Flags().Bool("cover", false, "Enable coverage")
			cmd.Flags().String("coverprofile", "", "Coverage profile")
			cmd.Flags().String("coverpkg", "", "Coverage packages")
			cmd.Flags().Bool("ci", false, "CI mode")
			cmd.Flags().String("post-comment", "", "Post comment strategy")
			cmd.Flags().String("github-token", "", "GitHub token")
			cmd.Flags().Bool("exclude-mocks", false, "Exclude mocks")
			cmd.Flags().String("include", "", "Include patterns")
			cmd.Flags().String("exclude", "", "Exclude patterns")
			cmd.Flags().String("run", "", "Run specific tests")
			cmd.Flags().String("timeout", "", "Test timeout")
			cmd.Flags().Bool("short", false, "Run short tests")
			cmd.Flags().Bool("race", false, "Enable race detection")
			cmd.Flags().Int("count", 1, "Run tests N times")
			cmd.Flags().Bool("shuffle", false, "Shuffle tests")

			// Setup viper if needed
			if tt.setupViper != nil {
				tt.setupViper()
			}

			// Setup environment if needed
			if tt.setupEnv != nil {
				tt.setupEnv()
			}
			if tt.cleanupEnv != nil {
				defer tt.cleanupEnv()
			}

			// Setup command flags if needed
			if tt.setup != nil {
				tt.setup(cmd)
			}

			// Extract configuration
			config, err := extractStreamConfig(cmd, tt.args, logger)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				require.NotNil(t, config)
				if tt.want != nil {
					tt.want(t, config)
				}
			}
		})
	}
}

// TestExtractTestArguments tests the extractTestArguments function
func TestExtractTestArguments(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(cmd *cobra.Command)
		setupArgs func()
		cleanup   func()
		want      []string
	}{
		{
			name: "no test arguments",
			setup: func(cmd *cobra.Command) {
				// No flags set
			},
			want: []string{},
		},
		{
			name: "run flag",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().Set("run", "TestSpecific")
			},
			want: []string{"-run", "TestSpecific"},
		},
		{
			name: "timeout flag",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().Set("timeout", "5m")
			},
			want: []string{"-timeout", "5m"},
		},
		{
			name: "short flag",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().Set("short", "true")
			},
			want: []string{"-short"},
		},
		{
			name: "race flag",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().Set("race", "true")
			},
			want: []string{"-race"},
		},
		{
			name: "count flag",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().Set("count", "3")
			},
			want: []string{"-count", "3"},
		},
		{
			name: "shuffle flag",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().Set("shuffle", "true")
			},
			want: []string{"-shuffle", "on"},
		},
		{
			name: "multiple flags",
			setup: func(cmd *cobra.Command) {
				cmd.Flags().Set("run", "TestSpecific")
				cmd.Flags().Set("timeout", "5m")
				cmd.Flags().Set("short", "true")
				cmd.Flags().Set("race", "true")
			},
			want: []string{"-run", "TestSpecific", "-timeout", "5m", "-short", "-race"},
		},
		{
			name: "raw args after double dash",
			setupArgs: func() {
				// Simulate command line args with --
				os.Args = []string{"gotcha", "stream", "--", "-v", "-count=2", "-parallel=4"}
			},
			cleanup: func() {
				os.Args = []string{}
			},
			want: []string{"-v", "-count=2", "-parallel=4"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test command with all flags
			cmd := &cobra.Command{}
			cmd.Flags().String("run", "", "Run specific tests")
			cmd.Flags().String("timeout", "", "Test timeout")
			cmd.Flags().Bool("short", false, "Run short tests")
			cmd.Flags().Bool("race", false, "Enable race detection")
			cmd.Flags().Int("count", 1, "Run tests N times")
			cmd.Flags().Bool("shuffle", false, "Shuffle tests")

			// Setup args if needed
			if tt.setupArgs != nil {
				tt.setupArgs()
			}
			if tt.cleanup != nil {
				defer tt.cleanup()
			}

			// Setup command flags if needed
			if tt.setup != nil {
				tt.setup(cmd)
			}

			// Extract test arguments
			args := extractTestArguments(cmd)

			assert.Equal(t, tt.want, args)
		})
	}
}

// TestParseTestPackages tests the parseTestPackages method
func TestParseTestPackages(t *testing.T) {
	tests := []struct {
		name     string
		config   *StreamConfig
		expected []string
	}{
		{
			name: "default test path",
			config: &StreamConfig{
				TestPath: "./...",
			},
			expected: []string{"./..."},
		},
		{
			name: "ellipsis only",
			config: &StreamConfig{
				TestPath: "...",
			},
			expected: []string{"./..."},
		},
		{
			name: "recursive path",
			config: &StreamConfig{
				TestPath: "./cmd/...",
			},
			expected: []string{"./cmd/..."},
		},
		{
			name: "comma-separated packages",
			config: &StreamConfig{
				TestPath: "./cmd, ./pkg, ./internal",
			},
			expected: []string{"./cmd", "./pkg", "./internal"},
		},
		{
			name: "single package",
			config: &StreamConfig{
				TestPath: "./cmd/gotcha",
			},
			expected: []string{"./cmd/gotcha"},
		},
		{
			name: "relative path",
			config: &StreamConfig{
				TestPath: "../other",
			},
			expected: []string{"../other"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.parseTestPackages()
			assert.Equal(t, tt.expected, tt.config.TestPackages)
		})
	}
}

// TestDetectCIMode tests the detectCIMode method
func TestDetectCIMode(t *testing.T) {
	tests := []struct {
		name       string
		config     *StreamConfig
		setupEnv   func()
		cleanupEnv func()
		wantCIMode bool
	}{
		{
			name: "CI not detected when no env vars",
			config: &StreamConfig{
				CIMode: false,
			},
			wantCIMode: false,
		},
		{
			name: "CI detected from CI env var",
			config: &StreamConfig{
				CIMode: false,
			},
			setupEnv: func() {
				os.Setenv("CI", "true")
				// Need to reinitialize viper bindings after setting env vars
				viper.Reset()
				viper.BindEnv("runtime.ci", "CI")
			},
			cleanupEnv: func() {
				os.Unsetenv("CI")
				viper.Reset()
			},
			wantCIMode: true,
		},
		{
			name: "CI detected from GITHUB_ACTIONS",
			config: &StreamConfig{
				CIMode: false,
			},
			setupEnv: func() {
				os.Setenv("GITHUB_ACTIONS", "true")
				// Need to reinitialize viper bindings after setting env vars
				viper.Reset()
				viper.BindEnv("runtime.github.actions", "GITHUB_ACTIONS")
			},
			cleanupEnv: func() {
				os.Unsetenv("GITHUB_ACTIONS")
				viper.Reset()
			},
			wantCIMode: true,
		},
		{
			name: "CI mode already enabled not changed",
			config: &StreamConfig{
				CIMode: true,
			},
			wantCIMode: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := log.New(os.Stderr)
			logger.SetLevel(log.DebugLevel)

			if tt.setupEnv != nil {
				tt.setupEnv()
			}
			if tt.cleanupEnv != nil {
				defer tt.cleanupEnv()
			}

			tt.config.detectCIMode(logger)
			assert.Equal(t, tt.wantCIMode, tt.config.CIMode)
		})
	}
}

// TestAdjustFormatForCI tests the adjustFormatForCI method
func TestAdjustFormatForCI(t *testing.T) {
	tests := []struct {
		name        string
		config      *StreamConfig
		flagChanged bool
		wantFormat  string
	}{
		{
			name: "format changed to markdown in CI when terminal",
			config: &StreamConfig{
				CIMode: true,
				Format: constants.FormatTerminal,
			},
			flagChanged: false,
			wantFormat:  "markdown",
		},
		{
			name: "format not changed when user explicitly set",
			config: &StreamConfig{
				CIMode: true,
				Format: constants.FormatTerminal,
			},
			flagChanged: true,
			wantFormat:  constants.FormatTerminal,
		},
		{
			name: "format not changed when not in CI",
			config: &StreamConfig{
				CIMode: false,
				Format: constants.FormatTerminal,
			},
			flagChanged: false,
			wantFormat:  constants.FormatTerminal,
		},
		{
			name: "format not changed when already markdown",
			config: &StreamConfig{
				CIMode: true,
				Format: "markdown",
			},
			flagChanged: false,
			wantFormat:  "markdown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			logger := log.New(os.Stderr)
			logger.SetLevel(log.DebugLevel)

			cmd := &cobra.Command{}
			cmd.Flags().String("format", "", "Output format")
			if tt.flagChanged {
				cmd.Flags().Set("format", tt.config.Format)
			}

			tt.config.adjustFormatForCI(cmd, logger)
			assert.Equal(t, tt.wantFormat, tt.config.Format)
		})
	}
}

// TestAdjustShowFilterForVerbosity tests the adjustShowFilterForVerbosity method
func TestAdjustShowFilterForVerbosity(t *testing.T) {
	tests := []struct {
		name           string
		config         *StreamConfig
		verbositySet   bool
		wantShowFilter string
	}{
		{
			name: "minimal verbosity sets failed filter",
			config: &StreamConfig{
				VerbosityLevel: "minimal",
				ShowFilter:     "all",
			},
			verbositySet:   true,
			wantShowFilter: "failed",
		},
		{
			name: "verbose verbosity sets all filter",
			config: &StreamConfig{
				VerbosityLevel: "verbose",
				ShowFilter:     "failed",
			},
			verbositySet:   true,
			wantShowFilter: "all",
		},
		{
			name: "standard verbosity keeps existing filter",
			config: &StreamConfig{
				VerbosityLevel: "standard",
				ShowFilter:     "failed",
			},
			verbositySet:   true,
			wantShowFilter: "failed",
		},
		{
			name: "no change when verbosity not set",
			config: &StreamConfig{
				VerbosityLevel: "minimal",
				ShowFilter:     "all",
			},
			verbositySet:   false,
			wantShowFilter: "all",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("verbosity", "", "Verbosity level")
			if tt.verbositySet {
				cmd.Flags().Set("verbosity", tt.config.VerbosityLevel)
			}

			tt.config.adjustShowFilterForVerbosity(cmd)
			assert.Equal(t, tt.wantShowFilter, tt.config.ShowFilter)
		})
	}
}

// TestNormalizePostingStrategyConfig tests the normalizePostingStrategy function in config context
func TestNormalizePostingStrategyConfig(t *testing.T) {
	tests := []struct {
		name         string
		strategy     string
		flagPresent  bool
		wantStrategy string
	}{
		{
			name:         "empty strategy defaults to on-failure when no flag",
			strategy:     "",
			flagPresent:  false,
			wantStrategy: "on-failure",
		},
		{
			name:         "empty strategy defaults to never when flag present",
			strategy:     "",
			flagPresent:  true,
			wantStrategy: "never",
		},
		{
			name:         "always strategy unchanged",
			strategy:     "always",
			flagPresent:  true,
			wantStrategy: "always",
		},
		{
			name:         "never strategy unchanged",
			strategy:     "never",
			flagPresent:  true,
			wantStrategy: "never",
		},
		{
			name:         "on-failure strategy unchanged",
			strategy:     "on-failure",
			flagPresent:  true,
			wantStrategy: "on-failure",
		},
		{
			name:         "adaptive strategy unchanged",
			strategy:     "adaptive",
			flagPresent:  true,
			wantStrategy: "adaptive",
		},
		{
			name:         "invalid strategy returns as-is",
			strategy:     "invalid",
			flagPresent:  true,
			wantStrategy: "invalid",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizePostingStrategy(tt.strategy, tt.flagPresent)
			assert.Equal(t, tt.wantStrategy, result)
		})
	}
}

// TestViperBindings tests that viper bindings work correctly
func TestViperBindings(t *testing.T) {
	t.Run("environment variable binding", func(t *testing.T) {
		// Reset viper
		viper.Reset()

		// Set environment variables
		os.Setenv("GOTCHA_POST_COMMENT", "always")
		os.Setenv("GITHUB_TOKEN", "test-token")
		defer os.Unsetenv("GOTCHA_POST_COMMENT")
		defer os.Unsetenv("GITHUB_TOKEN")

		// Create command and bind
		cmd := &cobra.Command{}
		cmd.Flags().String("post-comment", "", "Post comment strategy")
		cmd.Flags().String("github-token", "", "GitHub token")

		// Bind flags to viper
		viper.BindPFlag("post-comment", cmd.Flags().Lookup("post-comment"))
		viper.BindEnv("post-comment", "GOTCHA_POST_COMMENT", "POST_COMMENT")
		viper.BindPFlag("github-token", cmd.Flags().Lookup("github-token"))
		viper.BindEnv("github-token", "GITHUB_TOKEN")

		// Check values
		assert.Equal(t, "always", viper.GetString("post-comment"))
		assert.Equal(t, "test-token", viper.GetString("github-token"))
	})

	t.Run("config file values", func(t *testing.T) {
		// Reset viper
		viper.Reset()

		// Set values directly (simulating config file)
		viper.Set("show", "failed")
		viper.Set("format", "json")
		viper.Set("coverage.enabled", true)
		viper.Set("coverage.profile", "coverage.out")

		// Check values
		assert.Equal(t, "failed", viper.GetString("show"))
		assert.Equal(t, "json", viper.GetString("format"))
		assert.True(t, viper.GetBool("coverage.enabled"))
		assert.Equal(t, "coverage.out", viper.GetString("coverage.profile"))
	})
}
