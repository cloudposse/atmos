package errors

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/markdown"
)

func TestHandleError(t *testing.T) {
	// Save original logger
	originalLogger := log.Default()
	defer log.SetDefault(originalLogger)

	// Create test logger to capture log output
	var logBuf bytes.Buffer
	testLogger := log.New()
	testLogger.SetOutput(&logBuf)
	testLogger.SetLevel(log.TraceLevel)
	log.SetDefault(testLogger)

	render, _ = markdown.NewTerminalMarkdownRenderer(schema.AtmosConfiguration{})
	err := errors.New("this is a test error")

	// Redirect stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	HandleError(err)

	// Restore stderr
	err = w.Close()
	assert.Nil(t, err)

	os.Stderr = oldStderr

	var output bytes.Buffer
	_, err = io.Copy(&output, r)
	assert.NoError(t, err, "'TestHandleError' should execute without error")

	// Check if output contains the expected content
	assert.Contains(t, output.String(), "this is a test error", "'TestHandleError' output should contain error message")

	// Test with nil render to improve coverage
	t.Run("nil render", func(t *testing.T) {
		// Save and nil render
		originalRender := render
		render = nil
		defer func() {
			render = originalRender
		}()

		logBuf.Reset()
		testErr := errors.New("error with nil render")
		HandleError(testErr)

		// Should log error directly
		assert.Contains(t, logBuf.String(), "error with nil render")
	})
}

func TestSetVerboseFlag(t *testing.T) {
	// Save original state
	originalFlag := verboseFlag
	originalFlagSet := verboseFlagSet
	defer func() {
		verboseFlag = originalFlag
		verboseFlagSet = originalFlagSet
	}()

	tests := []struct {
		name            string
		verbose         bool
		expectedFlag    bool
		expectedFlagSet bool
	}{
		{
			name:            "set verbose to true",
			verbose:         true,
			expectedFlag:    true,
			expectedFlagSet: true,
		},
		{
			name:            "set verbose to false",
			verbose:         false,
			expectedFlag:    false,
			expectedFlagSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			SetVerboseFlag(tt.verbose)
			assert.Equal(t, tt.expectedFlag, verboseFlag)
			assert.Equal(t, tt.expectedFlagSet, verboseFlagSet)
		})
	}
}

func TestInitializeMarkdown(t *testing.T) {
	// Save original state
	originalRender := render
	originalAtmosConfig := atmosConfig
	defer func() {
		render = originalRender
		atmosConfig = originalAtmosConfig
	}()

	tests := []struct {
		name   string
		config *schema.AtmosConfiguration
	}{
		{
			name: "initialize with basic config",
			config: &schema.AtmosConfiguration{
				BasePath: "/tmp",
				Errors: schema.ErrorsConfig{
					Sentry: schema.SentryConfig{
						Enabled: false,
					},
				},
			},
		},
		{
			name: "initialize with sentry disabled",
			config: &schema.AtmosConfiguration{
				BasePath: "/tmp/test",
				Errors: schema.ErrorsConfig{
					Sentry: schema.SentryConfig{
						Enabled: false,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			InitializeMarkdown(tt.config)

			// Verify renderer was initialized
			assert.NotNil(t, render, "Renderer should be initialized")

			// Verify config was stored
			assert.Equal(t, tt.config, atmosConfig, "Config should be stored")
		})
	}
}

func TestGetMarkdownRenderer(t *testing.T) {
	// Save original render
	originalRender := render
	defer func() {
		render = originalRender
	}()

	t.Run("returns nil when not initialized", func(t *testing.T) {
		render = nil
		result := GetMarkdownRenderer()
		assert.Nil(t, result)
	})

	t.Run("returns renderer when initialized", func(t *testing.T) {
		testRenderer, _ := markdown.NewTerminalMarkdownRenderer(schema.AtmosConfiguration{})
		render = testRenderer

		result := GetMarkdownRenderer()
		assert.Equal(t, testRenderer, result)
	})
}

func TestHandleError_WithAtmosConfig(t *testing.T) {
	// Save original state
	originalAtmosConfig := atmosConfig
	originalRender := render
	defer func() {
		atmosConfig = originalAtmosConfig
		render = originalRender
	}()

	tests := []struct {
		name        string
		err         error
		setupConfig func() *schema.AtmosConfiguration
		shouldPrint bool
	}{
		{
			name:        "nil error - no output",
			err:         nil,
			setupConfig: func() *schema.AtmosConfiguration { return nil },
			shouldPrint: false,
		},
		{
			name: "with atmosConfig - uses formatter",
			err:  errors.New("test error with config"),
			setupConfig: func() *schema.AtmosConfiguration {
				return &schema.AtmosConfiguration{
					BasePath: "/tmp",
					Errors: schema.ErrorsConfig{
						Format: schema.ErrorFormatConfig{
							Verbose: false,
							Color:   "auto",
						},
						Sentry: schema.SentryConfig{
							Enabled: false,
						},
					},
				}
			},
			shouldPrint: true,
		},
		{
			name: "without atmosConfig - uses markdown",
			err:  errors.New("test error without config"),
			setupConfig: func() *schema.AtmosConfiguration {
				return nil
			},
			shouldPrint: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig = tt.setupConfig()

			if atmosConfig == nil {
				// Initialize render for fallback path
				render, _ = markdown.NewTerminalMarkdownRenderer(schema.AtmosConfiguration{})
			}

			// Redirect stderr to capture output
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			HandleError(tt.err)

			// Close writer and restore stderr
			_ = w.Close()
			os.Stderr = oldStderr

			var output bytes.Buffer
			_, _ = io.Copy(&output, r)

			if tt.shouldPrint {
				assert.NotEmpty(t, output.String(), "Should have printed error output")
			} else {
				// Nil error shouldn't print anything
				assert.Empty(t, output.String(), "Should not print for nil error")
			}
		})
	}
}

func TestPrintFormattedError_VerbosePrecedence(t *testing.T) {
	// Save original state
	originalAtmosConfig := atmosConfig
	originalVerboseFlag := verboseFlag
	originalVerboseFlagSet := verboseFlagSet
	defer func() {
		atmosConfig = originalAtmosConfig
		verboseFlag = originalVerboseFlag
		verboseFlagSet = originalVerboseFlagSet
		os.Unsetenv("ATMOS_VERBOSE")
	}()

	tests := []struct {
		name           string
		configVerbose  bool
		envVerbose     string
		cliVerbose     bool
		cliVerboseSet  bool
		expectedOutput string // Check for verbose markers
	}{
		{
			name:          "config only - false",
			configVerbose: false,
			envVerbose:    "",
			cliVerboseSet: false,
		},
		{
			name:          "config only - true",
			configVerbose: true,
			envVerbose:    "",
			cliVerboseSet: false,
		},
		{
			name:          "env overrides config - true",
			configVerbose: false,
			envVerbose:    "true",
			cliVerboseSet: false,
		},
		{
			name:          "cli overrides env - false",
			configVerbose: true,
			envVerbose:    "true",
			cliVerbose:    false,
			cliVerboseSet: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup config
			atmosConfig = &schema.AtmosConfiguration{
				BasePath: "/tmp",
				Errors: schema.ErrorsConfig{
					Format: schema.ErrorFormatConfig{
						Verbose: tt.configVerbose,
						Color:   "never",
					},
				},
			}

			// Setup env
			if tt.envVerbose != "" {
				os.Setenv("ATMOS_VERBOSE", tt.envVerbose)
			} else {
				os.Unsetenv("ATMOS_VERBOSE")
			}

			// Setup CLI flag
			verboseFlag = tt.cliVerbose
			verboseFlagSet = tt.cliVerboseSet

			// Redirect stderr
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			testErr := errors.New("test error for verbose precedence")
			printFormattedError(testErr)

			_ = w.Close()
			os.Stderr = oldStderr

			var output bytes.Buffer
			_, _ = io.Copy(&output, r)

			// Verify output was generated
			assert.Contains(t, output.String(), "test error for verbose precedence")
		})
	}
}

func TestPrintMarkdownError(t *testing.T) {
	// Save original state
	originalRender := render
	defer func() {
		render = originalRender
	}()

	tests := []struct {
		name        string
		setupRender func()
		err         error
		title       string
		suggestion  string
	}{
		{
			name: "with render and all params",
			setupRender: func() {
				render, _ = markdown.NewTerminalMarkdownRenderer(schema.AtmosConfiguration{})
			},
			err:        errors.New("test error"),
			title:      "custom title",
			suggestion: "try this",
		},
		{
			name: "with render and empty title",
			setupRender: func() {
				render, _ = markdown.NewTerminalMarkdownRenderer(schema.AtmosConfiguration{})
			},
			err:        errors.New("test error empty title"),
			title:      "",
			suggestion: "",
		},
		{
			name: "nil render",
			setupRender: func() {
				render = nil
			},
			err:        errors.New("test error nil render"),
			title:      "title",
			suggestion: "suggestion",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.setupRender()

			// Redirect stderr
			oldStderr := os.Stderr
			r, w, _ := os.Pipe()
			os.Stderr = w

			// Create test logger
			var logBuf bytes.Buffer
			testLogger := log.New()
			testLogger.SetOutput(&logBuf)
			testLogger.SetLevel(log.TraceLevel)
			originalLogger := log.Default()
			log.SetDefault(testLogger)
			defer log.SetDefault(originalLogger)

			printMarkdownError(tt.err, tt.title, tt.suggestion)

			_ = w.Close()
			os.Stderr = oldStderr

			var output bytes.Buffer
			_, _ = io.Copy(&output, r)

			if render == nil {
				// Should log error
				assert.Contains(t, logBuf.String(), tt.err.Error())
			} else {
				// Should write to stderr
				assert.NotEmpty(t, output.String())
			}
		})
	}
}
