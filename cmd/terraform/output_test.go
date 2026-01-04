package terraform

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tfoutput "github.com/cloudposse/atmos/pkg/terraform/output"
)

// TestOutputCommandSetup verifies that the output command is properly configured.
func TestOutputCommandSetup(t *testing.T) {
	// Verify command is registered.
	require.NotNil(t, outputCmd)

	// Verify it's attached to terraformCmd.
	found := false
	for _, cmd := range terraformCmd.Commands() {
		if cmd.Name() == "output" {
			found = true
			break
		}
	}
	assert.True(t, found, "output should be registered as a subcommand of terraformCmd")

	// Verify command short and long descriptions.
	assert.Contains(t, outputCmd.Short, "output")
	assert.Contains(t, outputCmd.Long, "Terraform")
}

// TestOutputParserSetup verifies that the output parser is properly configured.
func TestOutputParserSetup(t *testing.T) {
	require.NotNil(t, outputParser, "outputParser should be initialized")

	// Verify the parser has the output-specific flags.
	registry := outputParser.Registry()

	expectedFlags := []string{
		"format",
		"output-file",
		"uppercase",
		"flatten",
	}

	for _, flagName := range expectedFlags {
		assert.True(t, registry.Has(flagName), "outputParser should have %s flag registered", flagName)
	}
}

// TestOutputFlagSetup verifies that output command has correct flags registered.
func TestOutputFlagSetup(t *testing.T) {
	// Verify output-specific flags are registered on the command.
	outputFlags := []string{
		"format",
		"output-file",
		"uppercase",
		"flatten",
	}

	for _, flagName := range outputFlags {
		flag := outputCmd.Flags().Lookup(flagName)
		assert.NotNil(t, flag, "%s flag should be registered on output command", flagName)
	}
}

// TestOutputFlagDefaults verifies that output command flags have correct default values.
func TestOutputFlagDefaults(t *testing.T) {
	v := viper.New()

	// Bind parser to fresh viper instance.
	err := outputParser.BindToViper(v)
	require.NoError(t, err)

	// Verify default values.
	assert.Equal(t, "", v.GetString("format"), "format should default to empty string")
	assert.Equal(t, "", v.GetString("output-file"), "output-file should default to empty string")
	assert.False(t, v.GetBool("uppercase"), "uppercase should default to false")
	assert.False(t, v.GetBool("flatten"), "flatten should default to false")
}

// TestOutputFlagEnvVars verifies that output command flags have environment variable bindings.
func TestOutputFlagEnvVars(t *testing.T) {
	registry := outputParser.Registry()

	// Expected env var bindings.
	expectedEnvVars := map[string]string{
		"format":      "ATMOS_TERRAFORM_OUTPUT_FORMAT",
		"output-file": "ATMOS_TERRAFORM_OUTPUT_FILE",
		"uppercase":   "ATMOS_TERRAFORM_OUTPUT_UPPERCASE",
		"flatten":     "ATMOS_TERRAFORM_OUTPUT_FLATTEN",
	}

	for flagName, expectedEnvVar := range expectedEnvVars {
		require.True(t, registry.Has(flagName), "outputParser should have %s flag registered", flagName)
		flag := registry.Get(flagName)
		require.NotNil(t, flag, "outputParser should have info for %s flag", flagName)
		envVars := flag.GetEnvVars()
		assert.Contains(t, envVars, expectedEnvVar, "%s should be bound to %s", flagName, expectedEnvVar)
	}
}

// TestValidateOutputFormat tests the format validation function.
func TestValidateOutputFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		wantErr bool
	}{
		{
			name:    "valid json format",
			format:  "json",
			wantErr: false,
		},
		{
			name:    "valid yaml format",
			format:  "yaml",
			wantErr: false,
		},
		{
			name:    "valid hcl format",
			format:  "hcl",
			wantErr: false,
		},
		{
			name:    "valid env format",
			format:  "env",
			wantErr: false,
		},
		{
			name:    "valid dotenv format",
			format:  "dotenv",
			wantErr: false,
		},
		{
			name:    "valid bash format",
			format:  "bash",
			wantErr: false,
		},
		{
			name:    "valid csv format",
			format:  "csv",
			wantErr: false,
		},
		{
			name:    "valid tsv format",
			format:  "tsv",
			wantErr: false,
		},
		{
			name:    "invalid format",
			format:  "invalid",
			wantErr: true,
		},
		{
			name:    "empty format",
			format:  "",
			wantErr: true,
		},
		{
			name:    "xml format not supported",
			format:  "xml",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateOutputFormat(tt.format)
			if tt.wantErr {
				assert.Error(t, err, "validateOutputFormat(%q) should return error", tt.format)
			} else {
				assert.NoError(t, err, "validateOutputFormat(%q) should not return error", tt.format)
			}
		})
	}
}

// TestExtractOutputName tests the output name extraction function.
func TestExtractOutputName(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{
			name:     "no args",
			args:     []string{},
			expected: "",
		},
		{
			name:     "single positional arg",
			args:     []string{"vpc_id"},
			expected: "vpc_id",
		},
		{
			name:     "positional arg with flags before",
			args:     []string{"-json", "vpc_id"},
			expected: "vpc_id",
		},
		{
			name:     "positional arg with flags after",
			args:     []string{"vpc_id", "-json"},
			expected: "vpc_id",
		},
		{
			name:     "only flags",
			args:     []string{"-json", "--raw"},
			expected: "",
		},
		{
			name:     "flag with value",
			args:     []string{"-state=terraform.tfstate"},
			expected: "",
		},
		{
			name:     "mixed flags and positional",
			args:     []string{"-json", "my_output", "--raw"},
			expected: "my_output",
		},
		{
			name:     "long flag format",
			args:     []string{"--json", "output_name"},
			expected: "output_name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractOutputName(tt.args)
			assert.Equal(t, tt.expected, result, "extractOutputName(%v) should return %q", tt.args, tt.expected)
		})
	}
}

// TestFormatSingleOutput tests the single output formatting function.
func TestFormatSingleOutput(t *testing.T) {
	tests := []struct {
		name       string
		outputs    map[string]any
		outputName string
		format     string
		opts       tfoutput.FormatOptions
		wantErr    bool
		contains   string
	}{
		{
			name:       "existing scalar output json",
			outputs:    map[string]any{"vpc_id": "vpc-12345"},
			outputName: "vpc_id",
			format:     "json",
			opts:       tfoutput.FormatOptions{},
			wantErr:    false,
			contains:   "vpc-12345",
		},
		{
			name:       "existing scalar output yaml",
			outputs:    map[string]any{"vpc_id": "vpc-12345"},
			outputName: "vpc_id",
			format:     "yaml",
			opts:       tfoutput.FormatOptions{},
			wantErr:    false,
			contains:   "vpc-12345",
		},
		{
			name:       "non-existing output",
			outputs:    map[string]any{"vpc_id": "vpc-12345"},
			outputName: "non_existent",
			format:     "json",
			opts:       tfoutput.FormatOptions{},
			wantErr:    true,
		},
		{
			name:       "output with uppercase option",
			outputs:    map[string]any{"vpc_id": "vpc-12345"},
			outputName: "vpc_id",
			format:     "env",
			opts:       tfoutput.FormatOptions{Uppercase: true},
			wantErr:    false,
			contains:   "VPC_ID",
		},
		{
			name:       "complex output with json format",
			outputs:    map[string]any{"config": map[string]any{"host": "localhost"}},
			outputName: "config",
			format:     "json",
			opts:       tfoutput.FormatOptions{},
			wantErr:    false,
			contains:   "host",
		},
		{
			name:       "complex output with env format fails",
			outputs:    map[string]any{"config": map[string]any{"host": "localhost"}},
			outputName: "config",
			format:     "env",
			opts:       tfoutput.FormatOptions{},
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := formatSingleOutput(tt.outputs, tt.outputName, tt.format, tt.opts)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Contains(t, result, tt.contains)
			}
		})
	}
}

// TestOutputFlagShortcuts verifies that flags have the correct shortcuts.
func TestOutputFlagShortcuts(t *testing.T) {
	tests := []struct {
		flagName string
		shortcut string
	}{
		{"format", "f"},
		{"output-file", "o"},
		{"uppercase", "u"},
	}

	for _, tt := range tests {
		t.Run(tt.flagName, func(t *testing.T) {
			flag := outputCmd.Flags().Lookup(tt.flagName)
			require.NotNil(t, flag, "%s flag should exist", tt.flagName)
			assert.Equal(t, tt.shortcut, flag.Shorthand, "%s flag should have shortcut %s", tt.flagName, tt.shortcut)
		})
	}
}
