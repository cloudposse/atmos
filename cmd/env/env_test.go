package env

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
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
			expected: "export FOO='bar'\n",
		},
		{
			name: "multiple variables sorted",
			envVars: map[string]string{
				"ZZZ": "last",
				"AAA": "first",
				"MMM": "middle",
			},
			expected: "export AAA='first'\nexport MMM='middle'\nexport ZZZ='last'\n",
		},
		{
			name: "value with single quotes",
			envVars: map[string]string{
				"QUOTED": "it's a test",
			},
			expected: "export QUOTED='it'\\''s a test'\n",
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
			result := formatBash(tt.envVars)
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
			expected: "FOO='bar'\n",
		},
		{
			name: "value with single quotes",
			envVars: map[string]string{
				"QUOTED": "it's a test",
			},
			expected: "QUOTED='it'\\''s a test'\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDotenv(tt.envVars)
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
			result := formatGitHub(tt.envVars)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestWriteEnvToFile(t *testing.T) {
	t.Run("creates file if not exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.env")

		envVars := map[string]string{"FOO": "bar"}
		err := writeEnvToFile(envVars, filePath, formatBash)
		require.NoError(t, err)

		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, "export FOO='bar'\n", string(content))
	})

	t.Run("appends to existing file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "test.env")

		// Write initial content.
		err := os.WriteFile(filePath, []byte("# existing content\n"), 0o644)
		require.NoError(t, err)

		// Append env vars.
		envVars := map[string]string{"FOO": "bar"}
		err = writeEnvToFile(envVars, filePath, formatBash)
		require.NoError(t, err)

		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, "# existing content\nexport FOO='bar'\n", string(content))
	})

	t.Run("github format to file", func(t *testing.T) {
		tmpDir := t.TempDir()
		filePath := filepath.Join(tmpDir, "github_env")

		envVars := map[string]string{
			"GITHUB_TOKEN": "ghp_xxxx",
			"AWS_REGION":   "us-east-1",
		}
		err := writeEnvToFile(envVars, filePath, formatGitHub)
		require.NoError(t, err)

		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, "AWS_REGION=us-east-1\nGITHUB_TOKEN=ghp_xxxx\n", string(content))
	})
}

func TestSortedKeys(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]string
		expected []string
	}{
		{
			name:     "empty map",
			input:    map[string]string{},
			expected: []string{},
		},
		{
			name:     "single key",
			input:    map[string]string{"FOO": "bar"},
			expected: []string{"FOO"},
		},
		{
			name: "multiple keys sorted",
			input: map[string]string{
				"ZZZ": "last",
				"AAA": "first",
				"MMM": "middle",
			},
			expected: []string{"AAA", "MMM", "ZZZ"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := sortedKeys(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
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

func TestOutputEnvAsBash(t *testing.T) {
	// Initialize I/O context for data package.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)

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
			name: "multiple variables",
			envVars: map[string]string{
				"FOO": "bar",
				"BAZ": "qux",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// outputEnvAsBash writes to stdout via data.Write.
			// We just verify it doesn't error.
			err := outputEnvAsBash(tt.envVars)
			assert.NoError(t, err)
		})
	}
}

func TestOutputEnvAsDotenv(t *testing.T) {
	// Initialize I/O context for data package.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)

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
			name: "multiple variables",
			envVars: map[string]string{
				"FOO": "bar",
				"BAZ": "qux",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// outputEnvAsDotenv writes to stdout via data.Write.
			// We just verify it doesn't error.
			err := outputEnvAsDotenv(tt.envVars)
			assert.NoError(t, err)
		})
	}
}

func TestOutputEnvAsJSON(t *testing.T) {
	// Initialize I/O context for data package.
	ioCtx, err := iolib.NewContext()
	require.NoError(t, err)
	data.InitWriter(ioCtx)

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
			// outputEnvAsJSON writes to stdout via u.PrintAsJSON.
			// We just verify it doesn't error.
			atmosConfig := &schema.AtmosConfiguration{}
			err := outputEnvAsJSON(atmosConfig, tt.envVars)
			assert.NoError(t, err)
		})
	}
}

func TestWriteEnvToFile_ErrorCases(t *testing.T) {
	t.Run("fails with invalid path", func(t *testing.T) {
		// Try to write to a path that doesn't exist and can't be created.
		envVars := map[string]string{"FOO": "bar"}
		err := writeEnvToFile(envVars, "/nonexistent/directory/file.env", formatBash)
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

		envVars := map[string]string{"FOO": "bar"}
		err = writeEnvToFile(envVars, filepath.Join(readOnlyDir, "test.env"), formatBash)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "failed to open file")
	})
}
