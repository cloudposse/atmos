package yaml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStage_ReturnsEditedBytesWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atmos.yaml")
	original := "# keep this comment\ncomponents:\n  terraform:\n    init_run_reconfigure: false # trailing\n"
	require.NoError(t, os.WriteFile(path, []byte(original), 0o600))

	staged, err := Stage(path, []byte(original), `.components.terraform.init_run_reconfigure = true`)
	require.NoError(t, err)

	require.Contains(t, string(staged), "init_run_reconfigure: true")
	require.Contains(t, string(staged), "# keep this comment")
	require.Contains(t, string(staged), "# trailing")
	onDisk, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, original, string(onDisk), "Stage must never write the source file")
}

func TestStage_DoesNotRequireTheFileToExist(t *testing.T) {
	// The logical path is used only for EditorConfig lookup, so staged edits work on in-memory content.
	path := filepath.Join(t.TempDir(), "missing", "atmos.yaml")
	staged, err := Stage(path, []byte("a: 1\n"), `.a = 2`)
	require.NoError(t, err)
	require.Equal(t, "a: 2\n", string(staged))
	_, statErr := os.Stat(path)
	require.True(t, os.IsNotExist(statErr))
}

func TestStage_PreservesCRLFLineEndings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atmos.yaml")
	content := []byte("a: 1\r\nb: 2\r\n")
	staged, err := Stage(path, content, `.a = 3`)
	require.NoError(t, err)
	require.Contains(t, string(staged), "a: 3\r\n")
	require.NotContains(t, strings.ReplaceAll(string(staged), "\r\n", ""), "\n")
}

func TestStage_InvalidExpressionReturnsError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atmos.yaml")
	staged, err := Stage(path, []byte("a: 1\n"), `.a = (`)
	require.Error(t, err)
	require.Nil(t, staged)
}
