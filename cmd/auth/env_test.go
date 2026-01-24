package auth

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutputEnvAsExport(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		contains    []string
		notContains []string
	}{
		{
			name:     "empty map",
			envVars:  map[string]string{},
			contains: []string{},
		},
		{
			name: "basic variable",
			envVars: map[string]string{
				"AWS_REGION": "us-east-1",
			},
			contains: []string{"export AWS_REGION='us-east-1'"},
		},
		{
			name: "escapes single quotes",
			envVars: map[string]string{
				"VAR": "it's a test",
			},
			contains: []string{"export VAR='it'\\''s a test'"},
		},
		{
			name: "multiple variables sorted",
			envVars: map[string]string{
				"Z_VAR": "z",
				"A_VAR": "a",
				"M_VAR": "m",
			},
			contains: []string{"A_VAR", "M_VAR", "Z_VAR"},
		},
		{
			name: "special characters",
			envVars: map[string]string{
				"VAR": "value with spaces",
			},
			contains: []string{"export VAR='value with spaces'"},
		},
		{
			name: "empty value",
			envVars: map[string]string{
				"EMPTY": "",
			},
			contains: []string{"export EMPTY=''"},
		},
		{
			name: "value with equals",
			envVars: map[string]string{
				"VAR": "key=value",
			},
			contains: []string{"export VAR='key=value'"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := outputEnvAsExport(tt.envVars)
			require.NoError(t, err)

			// Restore stdout and read output.
			w.Close()
			os.Stdout = oldStdout

			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)
			output := buf.String()

			for _, expected := range tt.contains {
				assert.Contains(t, output, expected)
			}

			for _, notExpected := range tt.notContains {
				assert.NotContains(t, output, notExpected)
			}
		})
	}
}

func TestOutputEnvAsExport_Sorting(t *testing.T) {
	envVars := map[string]string{
		"Z_VAR": "z",
		"A_VAR": "a",
		"M_VAR": "m",
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputEnvAsExport(envVars)
	require.NoError(t, err)

	// Restore stdout and read output.
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Verify A comes before M, M comes before Z.
	aIndex := strings.Index(output, "A_VAR")
	mIndex := strings.Index(output, "M_VAR")
	zIndex := strings.Index(output, "Z_VAR")

	assert.Less(t, aIndex, mIndex, "A_VAR should come before M_VAR")
	assert.Less(t, mIndex, zIndex, "M_VAR should come before Z_VAR")
}

func TestOutputEnvAsDotenv(t *testing.T) {
	tests := []struct {
		name        string
		envVars     map[string]string
		contains    []string
		notContains []string
	}{
		{
			name:     "empty map",
			envVars:  map[string]string{},
			contains: []string{},
		},
		{
			name: "basic variable",
			envVars: map[string]string{
				"AWS_REGION": "us-east-1",
			},
			contains:    []string{"AWS_REGION='us-east-1'"},
			notContains: []string{"export"},
		},
		{
			name: "escapes single quotes",
			envVars: map[string]string{
				"VAR": "it's a test",
			},
			contains: []string{"VAR='it'\\''s a test'"},
		},
		{
			name: "no export prefix",
			envVars: map[string]string{
				"VAR": "value",
			},
			contains:    []string{"VAR='value'"},
			notContains: []string{"export"},
		},
		{
			name: "multiple variables sorted",
			envVars: map[string]string{
				"Z_VAR": "z",
				"A_VAR": "a",
			},
			contains: []string{"A_VAR", "Z_VAR"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Capture stdout.
			oldStdout := os.Stdout
			r, w, _ := os.Pipe()
			os.Stdout = w

			err := outputEnvAsDotenv(tt.envVars)
			require.NoError(t, err)

			// Restore stdout and read output.
			w.Close()
			os.Stdout = oldStdout

			var buf bytes.Buffer
			_, _ = io.Copy(&buf, r)
			output := buf.String()

			for _, expected := range tt.contains {
				assert.Contains(t, output, expected)
			}

			for _, notExpected := range tt.notContains {
				assert.NotContains(t, output, notExpected)
			}
		})
	}
}

func TestOutputEnvAsDotenv_Sorting(t *testing.T) {
	envVars := map[string]string{
		"Z_VAR": "z",
		"A_VAR": "a",
		"M_VAR": "m",
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputEnvAsDotenv(envVars)
	require.NoError(t, err)

	// Restore stdout and read output.
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Verify A comes before M, M comes before Z.
	aIndex := strings.Index(output, "A_VAR")
	mIndex := strings.Index(output, "M_VAR")
	zIndex := strings.Index(output, "Z_VAR")

	assert.Less(t, aIndex, mIndex, "A_VAR should come before M_VAR")
	assert.Less(t, mIndex, zIndex, "M_VAR should come before Z_VAR")
}

func TestAuthEnvCommand_Structure(t *testing.T) {
	assert.Equal(t, "env", authEnvCmd.Use)
	assert.NotEmpty(t, authEnvCmd.Short)
	assert.NotEmpty(t, authEnvCmd.Long)
	assert.NotNil(t, authEnvCmd.RunE)

	// Check format flag exists.
	formatFlag := authEnvCmd.Flags().Lookup("format")
	assert.NotNil(t, formatFlag)
	assert.Equal(t, "f", formatFlag.Shorthand)

	// Check login flag exists.
	loginFlag := authEnvCmd.Flags().Lookup("login")
	assert.NotNil(t, loginFlag)
}

func TestSupportedFormats(t *testing.T) {
	// Verify the supported formats constant.
	assert.Contains(t, SupportedFormats, "json")
	assert.Contains(t, SupportedFormats, "bash")
	assert.Contains(t, SupportedFormats, "dotenv")
	assert.Len(t, SupportedFormats, 3)
}

func TestFormatFlagName(t *testing.T) {
	assert.Equal(t, "format", FormatFlagName)
}

func TestEnvCommand_FlagCompletion(t *testing.T) {
	// Test that format flag has completion registered.
	cmd := authEnvCmd

	// Get the flag.
	flag := cmd.Flags().Lookup("format")
	assert.NotNil(t, flag)

	// The flag should exist and have a default.
	assert.Equal(t, "bash", flag.DefValue)
}

func TestEnvParser_Initialization(t *testing.T) {
	// envParser should be initialized in init().
	assert.NotNil(t, envParser)
}

func TestOutputEnvAsExport_MultipleQuotes(t *testing.T) {
	envVars := map[string]string{
		"VAR": "it's got 'multiple' quotes",
	}

	// Capture stdout.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	err := outputEnvAsExport(envVars)
	require.NoError(t, err)

	// Restore stdout and read output.
	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Each single quote should be escaped.
	assert.Contains(t, output, "export VAR=")
	// Count the escape sequences.
	assert.Equal(t, 3, strings.Count(output, "'\\''"))
}

func TestBuildConfigAndStacksInfo_WithEnvCommand(t *testing.T) {
	// Test that BuildConfigAndStacksInfo works with env command context.
	cmd := &cobra.Command{Use: "env"}
	v := viper.New()

	// This shouldn't panic.
	assert.NotPanics(t, func() {
		_ = BuildConfigAndStacksInfo(cmd, v)
	})
}
