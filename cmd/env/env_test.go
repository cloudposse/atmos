package env

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	envfmt "github.com/cloudposse/atmos/pkg/env"
)

func TestFormatBash(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "empty map",
			envVars:  map[string]string{},
			expected: "",
		},
		{
			name: "single variable",
			envVars: map[string]string{
				"FOO": "bar",
			},
			expected: "export FOO=bar\n",
		},
		{
			name: "multiple variables sorted",
			envVars: map[string]string{
				"ZZZ": "last",
				"AAA": "first",
				"MMM": "middle",
			},
			expected: "export AAA=first\nexport MMM=middle\nexport ZZZ=last\n",
		},
		{
			name: "value with single quotes",
			envVars: map[string]string{
				"QUOTED": "it's a test",
			},
			expected: "export QUOTED='it'\"'\"'s a test'\n",
		},
		{
			name: "value with spaces",
			envVars: map[string]string{
				"SPACED": "hello world",
			},
			expected: "export SPACED='hello world'\n",
		},
		{
			name: "value with special characters",
			envVars: map[string]string{
				"SPECIAL": "foo$bar`baz",
			},
			expected: "export SPECIAL='foo$bar`baz'\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataMap := envfmt.ConvertMapStringToAny(tt.envVars)
			result, err := envfmt.FormatData(dataMap, envfmt.FormatBash)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatDotenv(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "empty map",
			envVars:  map[string]string{},
			expected: "",
		},
		{
			name: "single variable",
			envVars: map[string]string{
				"FOO": "bar",
			},
			expected: "FOO=bar\n",
		},
		{
			name: "value with single quotes",
			envVars: map[string]string{
				"QUOTED": "it's a test",
			},
			expected: "QUOTED='it'\"'\"'s a test'\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataMap := envfmt.ConvertMapStringToAny(tt.envVars)
			result, err := envfmt.FormatData(dataMap, envfmt.FormatDotenv)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatGitHub(t *testing.T) {
	tests := []struct {
		name     string
		envVars  map[string]string
		expected string
	}{
		{
			name:     "empty map",
			envVars:  map[string]string{},
			expected: "",
		},
		{
			name: "single variable",
			envVars: map[string]string{
				"FOO": "bar",
			},
			expected: "FOO=bar\n",
		},
		{
			name: "value with spaces",
			envVars: map[string]string{
				"SPACED": "hello world",
			},
			expected: "SPACED=hello world\n",
		},
		{
			name: "multiline value uses heredoc",
			envVars: map[string]string{
				"MULTILINE": "line1\nline2\nline3",
			},
			expected: "MULTILINE<<ATMOS_EOF_MULTILINE\nline1\nline2\nline3\nATMOS_EOF_MULTILINE\n",
		},
		{
			name: "multiple variables sorted",
			envVars: map[string]string{
				"ZZZ": "last",
				"AAA": "first",
			},
			expected: "AAA=first\nZZZ=last\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataMap := envfmt.ConvertMapStringToAny(tt.envVars)
			result, err := envfmt.FormatData(dataMap, envfmt.FormatGitHub)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWriteEnvToFile(t *testing.T) {
	t.Run("creates file if not exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.env")

		dataMap := envfmt.ConvertMapStringToAny(map[string]string{"FOO": "bar"})
		formatted, err := envfmt.FormatData(dataMap, envfmt.FormatBash)
		require.NoError(t, err)

		err = envfmt.WriteToFile(filePath, formatted)
		require.NoError(t, err)

		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, "export FOO=bar\n", string(content))
	})

	t.Run("appends to existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.env")

		// Write initial content.
		err := os.WriteFile(filePath, []byte("# existing content\n"), 0o644)
		require.NoError(t, err)

		// Append env vars.
		dataMap := envfmt.ConvertMapStringToAny(map[string]string{"FOO": "bar"})
		formatted, err := envfmt.FormatData(dataMap, envfmt.FormatBash)
		require.NoError(t, err)

		err = envfmt.WriteToFile(filePath, formatted)
		require.NoError(t, err)

		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, "# existing content\nexport FOO=bar\n", string(content))
	})

	t.Run("github format to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "github_env")

		dataMap := envfmt.ConvertMapStringToAny(map[string]string{
			"GITHUB_TOKEN": "ghp_xxxx",
			"AWS_REGION":   "us-east-1",
		})
		formatted, err := envfmt.FormatData(dataMap, envfmt.FormatGitHub)
		require.NoError(t, err)

		err = envfmt.WriteToFile(filePath, formatted)
		require.NoError(t, err)

		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, "AWS_REGION=us-east-1\nGITHUB_TOKEN=ghp_xxxx\n", string(content))
	})
}

func TestSupportedFormats(t *testing.T) {
	// Ensure all expected formats are supported.
	expected := []string{"bash", "json", "dotenv", "github"}
	assert.Equal(t, expected, SupportedFormats)
}

func TestEnvCommandProvider(t *testing.T) {
	provider := &EnvCommandProvider{}

	t.Run("GetName returns env", func(t *testing.T) {
		assert.Equal(t, "env", provider.GetName())
	})

	t.Run("GetGroup returns Configuration Management", func(t *testing.T) {
		assert.Equal(t, "Configuration Management", provider.GetGroup())
	})

	t.Run("GetCommand returns non-nil command", func(t *testing.T) {
		cmd := provider.GetCommand()
		require.NotNil(t, cmd)
		assert.Equal(t, "env", cmd.Use)
	})

	t.Run("GetFlagsBuilder returns parser", func(t *testing.T) {
		builder := provider.GetFlagsBuilder()
		assert.NotNil(t, builder)
	})

	t.Run("GetPositionalArgsBuilder returns nil", func(t *testing.T) {
		builder := provider.GetPositionalArgsBuilder()
		assert.Nil(t, builder)
	})

	t.Run("GetCompatibilityFlags returns nil", func(t *testing.T) {
		flags := provider.GetCompatibilityFlags()
		assert.Nil(t, flags)
	})

	t.Run("GetAliases returns nil", func(t *testing.T) {
		aliases := provider.GetAliases()
		assert.Nil(t, aliases)
	})
}

func TestOutputEnvAsJSON(t *testing.T) {
	tests := []struct {
		name    string
		envVars map[string]string
	}{
		{
			name:    "empty map",
			envVars: map[string]string{},
		},
		{
			name: "single variable",
			envVars: map[string]string{
				"FOO": "bar",
			},
		},
		{
			name: "multiple variables with special chars",
			envVars: map[string]string{
				"FOO":     "bar",
				"SPECIAL": "value with spaces",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test JSON format output to file (stdout tests are in pkg/env).
			tmpDir := t.TempDir()
			filePath := filepath.Join(tmpDir, "test.json")

			err := envfmt.Output(tt.envVars, "json", filePath)
			require.NoError(t, err)

			// Verify file was created and contains valid JSON.
			content, err := os.ReadFile(filePath)
			require.NoError(t, err)

			// Verify JSON structure matches input.
			for key, value := range tt.envVars {
				assert.Contains(t, string(content), `"`+key+`"`)
				assert.Contains(t, string(content), `"`+value+`"`)
			}
		})
	}
}

func TestWriteEnvToFile_ErrorCases(t *testing.T) {
	t.Run("fails with invalid path", func(t *testing.T) {
		// Try to write to a path that doesn't exist and can't be created.
		err := envfmt.WriteToFile("/nonexistent/directory/file.env", "content")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open file")
	})

	t.Run("fails with read-only directory", func(t *testing.T) {
		// Skip on Windows: Windows uses ACLs instead of Unix-style permissions,
		// so directory mode 0o555 doesn't prevent file creation.
		if runtime.GOOS == "windows" {
			t.Skip("skipping on Windows: directory permissions work differently")
		}

		tmpDir := t.TempDir()
		readOnlyDir := filepath.Join(tmpDir, "readonly")
		err := os.Mkdir(readOnlyDir, 0o555)
		require.NoError(t, err)

		// Ensure cleanup even if test fails.
		defer func() {
			_ = os.Chmod(readOnlyDir, 0o755)
		}()

		err = envfmt.WriteToFile(filepath.Join(readOnlyDir, "test.env"), "content")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open file")
	})
}

func TestConvertMapStringToAny(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected map[string]any
	}{
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: map[string]any{},
		},
		{
			name:     "single entry",
			input:    map[string]string{"FOO": "bar"},
			expected: map[string]any{"FOO": "bar"},
		},
		{
			name:     "multiple entries",
			input:    map[string]string{"FOO": "bar", "BAZ": "qux"},
			expected: map[string]any{"FOO": "bar", "BAZ": "qux"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := envfmt.ConvertMapStringToAny(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatBashWithExportOption(t *testing.T) {
	tests := []struct {
		name         string
		envVars      map[string]string
		exportPrefix bool
		expected     string
	}{
		{
			name: "with export=true (default)",
			envVars: map[string]string{
				"FOO": "bar",
			},
			exportPrefix: true,
			expected:     "export FOO=bar\n",
		},
		{
			name: "with export=false",
			envVars: map[string]string{
				"FOO": "bar",
			},
			exportPrefix: false,
			expected:     "FOO=bar\n",
		},
		{
			name: "with export=false and single quotes",
			envVars: map[string]string{
				"QUOTED": "it's a test",
			},
			exportPrefix: false,
			expected:     "QUOTED='it'\"'\"'s a test'\n",
		},
		{
			name: "with export=false and multiple variables",
			envVars: map[string]string{
				"ZZZ": "last",
				"AAA": "first",
			},
			exportPrefix: false,
			expected:     "AAA=first\nZZZ=last\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dataMap := envfmt.ConvertMapStringToAny(tt.envVars)
			result, err := envfmt.FormatData(dataMap, envfmt.FormatBash, envfmt.WithExport(tt.exportPrefix))
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
