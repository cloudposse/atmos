package yaml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeEditorConfig(t *testing.T, dir, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".editorconfig"), []byte("root = true\n"+body), 0o644))
}

func readFileString(t *testing.T, filePath string) string {
	t.Helper()
	got, err := os.ReadFile(filePath)
	require.NoError(t, err)
	return string(got)
}

func TestSetFile_UsesEditorConfigIndentWhenFileHasNoDetectedIndent(t *testing.T) {
	dir := t.TempDir()
	writeEditorConfig(t, dir, "\n[*.yaml]\nindent_size = 4\n")
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, nil, 0o644))

	require.NoError(t, SetFile(file, "a.b", "v"))

	assert.Equal(t, "a:\n    b: v\n", readFileString(t, file))
}

func TestFileEditsPreserveDetectedIndentButFormatNormalizesToEditorConfig(t *testing.T) {
	dir := t.TempDir()
	writeEditorConfig(t, dir, "\n[*.yaml]\nindent_size = 4\n")
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte("a:\n  b: 1\nc: 2\n"), 0o644))

	require.NoError(t, SetFile(file, "c", "3"))
	assert.Contains(t, readFileString(t, file), "  b: 1", "set should preserve detected 2-space indent")

	require.NoError(t, FormatFile(file))
	assert.Contains(t, readFileString(t, file), "    b: 1", "format should normalize to configured 4-space indent")
}

func TestFileEditsPreserveDetectedCRLFButFormatNormalizesToEditorConfig(t *testing.T) {
	dir := t.TempDir()
	writeEditorConfig(t, dir, "\n[*.yaml]\nend_of_line = lf\n")
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte("a: 1\r\nb: 2\r\n"), 0o644))

	require.NoError(t, SetFile(file, "a", "x"))
	assert.Equal(t, "a: x\r\nb: 2\r\n", readFileString(t, file))

	require.NoError(t, FormatFile(file))
	assert.Equal(t, "a: x\nb: 2\n", readFileString(t, file))
}

func TestSetFile_UsesEditorConfigLineEndingWhenFileHasNoDetectedEnding(t *testing.T) {
	dir := t.TempDir()
	writeEditorConfig(t, dir, "\n[*.yaml]\nend_of_line = crlf\n")
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, nil, 0o644))

	require.NoError(t, SetFile(file, "a.b", "v"))

	assert.Equal(t, "a:\r\n  b: v\r\n", readFileString(t, file))
}

func TestFormatFile_RespectsEditorConfigFinalNewline(t *testing.T) {
	dir := t.TempDir()
	writeEditorConfig(t, dir, "\n[*.yaml]\ninsert_final_newline = false\n")
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte("a: 1\n"), 0o644))

	require.NoError(t, FormatFile(file))

	assert.Equal(t, "a: 1", readFileString(t, file))
}

func TestFormatFile_RespectsEditorConfigTrimTrailingWhitespace(t *testing.T) {
	dir := t.TempDir()
	writeEditorConfig(t, dir, "\n[*.yaml]\ntrim_trailing_whitespace = true\n")
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, []byte("script: |+\n  echo hi  \nname: x\n"), 0o644))

	require.NoError(t, FormatFile(file))

	got := readFileString(t, file)
	assert.NotContains(t, got, "  \n")
	assert.Contains(t, got, "echo hi")
}

func TestSetFile_InvalidEditorConfigFallsBackToCurrentBehavior(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".editorconfig"), []byte("root = true\n[*.yaml\nindent_size = 4\n"), 0o644))
	file := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(file, nil, 0o644))

	require.NoError(t, SetFile(file, "a.b", "v"))

	got := readFileString(t, file)
	assert.Equal(t, "a:\n  b: v\n", got)
	assert.False(t, strings.Contains(got, "    b: v"), "invalid .editorconfig must not force 4-space indent")
}
