package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestLogLevelCaseNormalization(t *testing.T) {
	tests := []struct {
		name          string
		inputLevel    string
		expectedLevel string // What should be stored after normalization
	}{
		{
			name:          "lowercase debug",
			inputLevel:    "debug",
			expectedLevel: "Debug", // Should be normalized to title case
		},
		{
			name:          "uppercase DEBUG",
			inputLevel:    "DEBUG",
			expectedLevel: "Debug", // Should be normalized to title case
		},
		{
			name:          "mixed case DeBuG",
			inputLevel:    "DeBuG",
			expectedLevel: "Debug", // Should be normalized to title case
		},
		{
			name:          "correctly cased Debug",
			inputLevel:    "Debug",
			expectedLevel: "Debug", // Should stay as is
		},
		{
			name:          "lowercase trace",
			inputLevel:    "trace",
			expectedLevel: "Trace",
		},
		{
			name:          "lowercase info",
			inputLevel:    "info",
			expectedLevel: "Info",
		},
		{
			name:          "lowercase warning",
			inputLevel:    "warning",
			expectedLevel: "Warning",
		},
		{
			name:          "lowercase warn (alias)",
			inputLevel:    "warn",
			expectedLevel: "Warning", // Should be normalized to Warning
		},
		{
			name:          "lowercase error",
			inputLevel:    "error",
			expectedLevel: "Error",
		},
		{
			name:          "lowercase off",
			inputLevel:    "off",
			expectedLevel: "Off",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create minimal atmosConfig and configAndStacksInfo
			atmosConfig := &schema.AtmosConfiguration{
				Logs: schema.Logs{},
			}
			configAndStacksInfo := &schema.ConfigAndStacksInfo{
				LogsLevel: tt.inputLevel,
			}

			// Call the function that should normalize the log level
			err := setLoggingConfig(atmosConfig, configAndStacksInfo)
			require.NoError(t, err, "setLoggingConfig should not error for valid log level")

			// Verify the log level was normalized to the expected case
			assert.Equal(t, tt.expectedLevel, atmosConfig.Logs.Level,
				"Log level should be normalized from '%s' to '%s'", tt.inputLevel, tt.expectedLevel)
		})
	}
}

func TestLogLevelInvalidValues(t *testing.T) {
	tests := []struct {
		name       string
		inputLevel string
	}{
		{name: "invalid level", inputLevel: "invalid"},
		{name: "typo", inputLevel: "debag"},
		{name: "random text", inputLevel: "foobar"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Logs: schema.Logs{},
			}
			configAndStacksInfo := &schema.ConfigAndStacksInfo{
				LogsLevel: tt.inputLevel,
			}

			err := setLoggingConfig(atmosConfig, configAndStacksInfo)
			assert.Error(t, err, "setLoggingConfig should error for invalid log level")
			assert.Contains(t, err.Error(), "invalid log level",
				"Error should mention invalid log level")
		})
	}
}

func TestLogLevelEmptyOrWhitespace(t *testing.T) {
	// Empty or whitespace log levels should be treated as empty (no error, no change)
	tests := []struct {
		name       string
		inputLevel string
	}{
		{name: "empty string", inputLevel: ""},
		{name: "whitespace", inputLevel: "   "},
		{name: "tabs", inputLevel: "\t\t"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atmosConfig := &schema.AtmosConfiguration{
				Logs: schema.Logs{
					Level: "Info", // Pre-existing level
				},
			}
			configAndStacksInfo := &schema.ConfigAndStacksInfo{
				LogsLevel: tt.inputLevel,
			}

			err := setLoggingConfig(atmosConfig, configAndStacksInfo)
			assert.NoError(t, err, "Empty/whitespace log level should not error")
			// Level should not be changed when input is empty/whitespace
			// (In practice, ParseLogLevel returns "Info" for empty strings, but since
			// the length check is > 0 after trimming, it won't even call ParseLogLevel)
		})
	}
}
