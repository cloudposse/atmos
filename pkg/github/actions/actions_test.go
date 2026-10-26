package actions

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsGitHubActions(t *testing.T) {
	t.Run("returns false when not in CI", func(t *testing.T) {
		// Unset by setting to empty value via t.Setenv.
		t.Setenv("GITHUB_ACTIONS", "")
		os.Unsetenv("GITHUB_ACTIONS")
		assert.False(t, IsGitHubActions())
	})

	t.Run("returns true when GITHUB_ACTIONS=true", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "true")
		assert.True(t, IsGitHubActions())
	})

	t.Run("returns false when GITHUB_ACTIONS has other value", func(t *testing.T) {
		t.Setenv("GITHUB_ACTIONS", "false")
		assert.False(t, IsGitHubActions())
	})
}

func TestGetOutputPath(t *testing.T) {
	t.Run("returns empty when not set", func(t *testing.T) {
		t.Setenv("GITHUB_OUTPUT", "")
		os.Unsetenv("GITHUB_OUTPUT")
		assert.Empty(t, GetOutputPath())
	})

	t.Run("returns path when set", func(t *testing.T) {
		t.Setenv("GITHUB_OUTPUT", "/tmp/github_output")
		assert.Equal(t, "/tmp/github_output", GetOutputPath())
	})
}

func TestGetEnvPath(t *testing.T) {
	t.Run("returns empty when not set", func(t *testing.T) {
		t.Setenv("GITHUB_ENV", "")
		os.Unsetenv("GITHUB_ENV")
		assert.Empty(t, GetEnvPath())
	})

	t.Run("returns path when set", func(t *testing.T) {
		t.Setenv("GITHUB_ENV", "/tmp/github_env")
		assert.Equal(t, "/tmp/github_env", GetEnvPath())
	})
}

func TestGetPathPath(t *testing.T) {
	t.Run("returns empty when not set", func(t *testing.T) {
		t.Setenv("GITHUB_PATH", "")
		os.Unsetenv("GITHUB_PATH")
		assert.Empty(t, GetPathPath())
	})

	t.Run("returns path when set", func(t *testing.T) {
		t.Setenv("GITHUB_PATH", "/tmp/github_path")
		assert.Equal(t, "/tmp/github_path", GetPathPath())
	})
}

func TestGetSummaryPath(t *testing.T) {
	t.Run("returns empty when not set", func(t *testing.T) {
		t.Setenv("GITHUB_STEP_SUMMARY", "")
		os.Unsetenv("GITHUB_STEP_SUMMARY")
		assert.Empty(t, GetSummaryPath())
	})

	t.Run("returns path when set", func(t *testing.T) {
		t.Setenv("GITHUB_STEP_SUMMARY", "/tmp/github_step_summary")
		assert.Equal(t, "/tmp/github_step_summary", GetSummaryPath())
	})
}

func TestFormatValue(t *testing.T) {
	t.Run("single line value", func(t *testing.T) {
		result := FormatValue("KEY", "value")
		assert.Equal(t, "KEY=value\n", result)
	})

	t.Run("multiline value uses heredoc", func(t *testing.T) {
		result := FormatValue("KEY", "line1\nline2")
		expected := "KEY<<ATMOS_EOF_KEY\nline1\nline2\nATMOS_EOF_KEY\n"
		assert.Equal(t, expected, result)
	})

	t.Run("value with trailing newline", func(t *testing.T) {
		result := FormatValue("KEY", "line1\nline2\n")
		expected := "KEY<<ATMOS_EOF_KEY\nline1\nline2\n\nATMOS_EOF_KEY\n"
		assert.Equal(t, expected, result)
	})

	t.Run("empty value", func(t *testing.T) {
		result := FormatValue("KEY", "")
		assert.Equal(t, "KEY=\n", result)
	})

	t.Run("value with special characters", func(t *testing.T) {
		result := FormatValue("KEY", "value with spaces & special=chars")
		assert.Equal(t, "KEY=value with spaces & special=chars\n", result)
	})

	t.Run("delimiter collision avoidance", func(t *testing.T) {
		// Value contains the default delimiter, so it should use a suffixed one.
		valueWithDelimiter := "line1\nATMOS_EOF_KEY\nline2"
		result := FormatValue("KEY", valueWithDelimiter)
		expected := "KEY<<ATMOS_EOF_KEY_0\nline1\nATMOS_EOF_KEY\nline2\nATMOS_EOF_KEY_0\n"
		assert.Equal(t, expected, result)
	})

	t.Run("multiple delimiter collisions", func(t *testing.T) {
		// Value contains both the default delimiter and first suffixed version.
		valueWithDelimiters := "line1\nATMOS_EOF_KEY\nATMOS_EOF_KEY_0\nline2"
		result := FormatValue("KEY", valueWithDelimiters)
		expected := "KEY<<ATMOS_EOF_KEY_1\nline1\nATMOS_EOF_KEY\nATMOS_EOF_KEY_0\nline2\nATMOS_EOF_KEY_1\n"
		assert.Equal(t, expected, result)
	})
}

func TestFormatData(t *testing.T) {
	t.Run("formats and sorts keys", func(t *testing.T) {
		data := map[string]string{"B": "2", "A": "1"}
		result := FormatData(data)
		assert.Equal(t, "A=1\nB=2\n", result)
	})

	t.Run("includes empty values", func(t *testing.T) {
		data := map[string]string{"A": "1", "B": "", "C": "3"}
		result := FormatData(data)
		assert.Equal(t, "A=1\nB=\nC=3\n", result)
	})

	t.Run("all empty values", func(t *testing.T) {
		data := map[string]string{"A": "", "B": ""}
		result := FormatData(data)
		assert.Equal(t, "A=\nB=\n", result)
	})

	t.Run("empty map", func(t *testing.T) {
		data := map[string]string{}
		result := FormatData(data)
		assert.Equal(t, "", result)
	})

	t.Run("handles multiline values", func(t *testing.T) {
		data := map[string]string{"MULTI": "line1\nline2", "SINGLE": "value"}
		result := FormatData(data)
		expected := "MULTI<<ATMOS_EOF_MULTI\nline1\nline2\nATMOS_EOF_MULTI\nSINGLE=value\n"
		assert.Equal(t, expected, result)
	})
}
