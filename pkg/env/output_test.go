package env

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOutput(t *testing.T) {
	t.Run("bash format to stdout", func(t *testing.T) {
		data := map[string]string{
			"VAR1": "value1",
			"VAR2": "value2",
		}

		// Capture stdout.
		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := Output(data, "bash", "")
		w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		buf := make([]byte, 1024)
		n, _ := r.Read(buf)
		output := string(buf[:n])

		// Note: shellescape.Quote doesn't add quotes for simple alphanumeric values.
		assert.Contains(t, output, "export VAR1=value1")
		assert.Contains(t, output, "export VAR2=value2")
	})

	t.Run("dotenv format to stdout", func(t *testing.T) {
		data := map[string]string{
			"VAR1": "value1",
		}

		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := Output(data, "dotenv", "")
		w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		buf := make([]byte, 1024)
		n, _ := r.Read(buf)
		output := string(buf[:n])

		// Note: shellescape.Quote doesn't add quotes for simple alphanumeric values.
		assert.Contains(t, output, "VAR1=value1")
		assert.NotContains(t, output, "export")
	})

	t.Run("github format to file", func(t *testing.T) {
		tempDir := t.TempDir()
		outputFile := filepath.Join(tempDir, "github_env")

		data := map[string]string{
			"VAR1": "value1",
		}

		err := Output(data, "github", outputFile)
		require.NoError(t, err)

		content, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		assert.Equal(t, "VAR1=value1\n", string(content))
	})

	t.Run("github format with multiline value", func(t *testing.T) {
		tempDir := t.TempDir()
		outputFile := filepath.Join(tempDir, "github_env")

		data := map[string]string{
			"CERT": "line1\nline2",
		}

		err := Output(data, "github", outputFile)
		require.NoError(t, err)

		content, err := os.ReadFile(outputFile)
		require.NoError(t, err)

		// Heredoc format for multiline values.
		assert.Contains(t, string(content), "CERT<<ATMOS_EOF_CERT")
		assert.Contains(t, string(content), "line1\nline2\n")
		assert.Contains(t, string(content), "ATMOS_EOF_CERT\n")
	})

	t.Run("json format to stdout", func(t *testing.T) {
		data := map[string]string{
			"VAR1": "value1",
		}

		oldStdout := os.Stdout
		r, w, _ := os.Pipe()
		os.Stdout = w

		err := Output(data, "json", "")
		w.Close()
		os.Stdout = oldStdout

		require.NoError(t, err)

		buf := make([]byte, 1024)
		n, _ := r.Read(buf)
		output := string(buf[:n])

		assert.Contains(t, output, `"VAR1": "value1"`)
	})

	t.Run("json format to file", func(t *testing.T) {
		tempDir := t.TempDir()
		outputFile := filepath.Join(tempDir, "env.json")

		data := map[string]string{
			"VAR1": "value1",
		}

		err := Output(data, "json", outputFile, WithFileMode(CredentialFileMode))
		require.NoError(t, err)

		content, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		assert.Contains(t, string(content), `"VAR1": "value1"`)

		// Verify file permissions.
		// Skip permission check on Windows: Windows uses ACLs instead of Unix-style permissions.
		if runtime.GOOS != "windows" {
			info, err := os.Stat(outputFile)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(CredentialFileMode), info.Mode().Perm())
		}
	})

	t.Run("invalid format returns error", func(t *testing.T) {
		data := map[string]string{
			"VAR1": "value1",
		}

		err := Output(data, "invalid-format", "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid format")
	})

	t.Run("file with custom permissions", func(t *testing.T) {
		tempDir := t.TempDir()
		outputFile := filepath.Join(tempDir, "env")

		data := map[string]string{
			"VAR1": "value1",
		}

		err := Output(data, "bash", outputFile, WithFileMode(CredentialFileMode))
		require.NoError(t, err)

		// Skip permission check on Windows: Windows uses ACLs instead of Unix-style permissions.
		if runtime.GOOS != "windows" {
			info, err := os.Stat(outputFile)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(CredentialFileMode), info.Mode().Perm())
		}
	})

	t.Run("appends to existing file", func(t *testing.T) {
		tempDir := t.TempDir()
		outputFile := filepath.Join(tempDir, "env")

		// Write initial content.
		err := os.WriteFile(outputFile, []byte("EXISTING=value\n"), 0o644)
		require.NoError(t, err)

		data := map[string]string{
			"NEW_VAR": "new-value",
		}

		err = Output(data, "github", outputFile)
		require.NoError(t, err)

		content, err := os.ReadFile(outputFile)
		require.NoError(t, err)
		assert.Equal(t, "EXISTING=value\nNEW_VAR=new-value\n", string(content))
	})
}

func TestWriteToFileWithMode(t *testing.T) {
	t.Run("creates file with specified mode", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "test_file")

		err := writeToFileWithMode(filePath, "content", CredentialFileMode)
		require.NoError(t, err)

		// Skip permission check on Windows: Windows uses ACLs instead of Unix-style permissions.
		if runtime.GOOS != "windows" {
			info, err := os.Stat(filePath)
			require.NoError(t, err)
			assert.Equal(t, os.FileMode(CredentialFileMode), info.Mode().Perm())
		}

		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, "content", string(content))
	})

	t.Run("appends to existing file", func(t *testing.T) {
		tempDir := t.TempDir()
		filePath := filepath.Join(tempDir, "test_file")

		err := os.WriteFile(filePath, []byte("existing\n"), 0o644)
		require.NoError(t, err)

		err = writeToFileWithMode(filePath, "new\n", CredentialFileMode)
		require.NoError(t, err)

		content, err := os.ReadFile(filePath)
		require.NoError(t, err)
		assert.Equal(t, "existing\nnew\n", string(content))
	})
}

func TestOutputOptions(t *testing.T) {
	t.Run("WithFileMode sets file mode", func(t *testing.T) {
		cfg := &outputConfig{}
		opt := WithFileMode(0o600)
		opt(cfg)
		assert.Equal(t, os.FileMode(0o600), cfg.fileMode)
	})

	t.Run("WithAtmosConfig sets config", func(t *testing.T) {
		cfg := &outputConfig{}
		// Just verify the option function works without panicking.
		opt := WithAtmosConfig(nil)
		opt(cfg)
		assert.Nil(t, cfg.atmosConfig)
	})
}

func TestOutputConstants(t *testing.T) {
	t.Run("DefaultOutputFileMode is 0644", func(t *testing.T) {
		assert.Equal(t, os.FileMode(0o644), os.FileMode(DefaultOutputFileMode))
	})

	t.Run("CredentialFileMode is 0600", func(t *testing.T) {
		assert.Equal(t, os.FileMode(0o600), os.FileMode(CredentialFileMode))
	})
}
