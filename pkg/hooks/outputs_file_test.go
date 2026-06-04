package hooks

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestReadOutputsFile_EmptyPath(t *testing.T) {
	out, err := ReadOutputsFile("")
	require.NoError(t, err)
	assert.Equal(t, map[string]any{}, out)
}

func TestReadOutputsFile_MissingFile(t *testing.T) {
	out, err := ReadOutputsFile("/nonexistent/path/to/file")
	require.NoError(t, err, "missing file should be treated as empty, not an error")
	assert.Equal(t, map[string]any{}, out)
}

func TestReadOutputsFile_EmptyFile(t *testing.T) {
	path := writeTemp(t, "")
	out, err := ReadOutputsFile(path)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{}, out)
}

func TestReadOutputsFile_WhitespaceOnly(t *testing.T) {
	path := writeTemp(t, "   \n\t\n  ")
	out, err := ReadOutputsFile(path)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{}, out)
}

func TestReadOutputsFile_JSONObject(t *testing.T) {
	path := writeTemp(t, `{"agent_id": "agent_abc", "env_id": "env_xyz", "count": 3}`)
	out, err := ReadOutputsFile(path)
	require.NoError(t, err)
	assert.Equal(t, "agent_abc", out["agent_id"])
	assert.Equal(t, "env_xyz", out["env_id"])
	assert.Equal(t, float64(3), out["count"], "JSON numbers come back as float64")
}

func TestReadOutputsFile_JSONWithLeadingWhitespace(t *testing.T) {
	path := writeTemp(t, "\n\n  {\"k\": \"v\"}")
	out, err := ReadOutputsFile(path)
	require.NoError(t, err)
	assert.Equal(t, "v", out["k"])
}

func TestReadOutputsFile_InvalidJSON(t *testing.T) {
	path := writeTemp(t, `{"agent_id": "missing closing brace"`)
	_, err := ReadOutputsFile(path)
	require.Error(t, err)
}

func TestReadOutputsFile_KeyValueLines(t *testing.T) {
	path := writeTemp(t, "agent_id=agent_abc\nenv_id=env_xyz\n")
	out, err := ReadOutputsFile(path)
	require.NoError(t, err)
	assert.Equal(t, "agent_abc", out["agent_id"])
	assert.Equal(t, "env_xyz", out["env_id"])
}

func TestReadOutputsFile_KeyValueWithQuotes(t *testing.T) {
	path := writeTemp(t, `agent_id="agent with spaces"`+"\n"+`env_id='single-quoted'`)
	out, err := ReadOutputsFile(path)
	require.NoError(t, err)
	assert.Equal(t, "agent with spaces", out["agent_id"])
	assert.Equal(t, "single-quoted", out["env_id"])
}

func TestReadOutputsFile_KeyValueWithCommentsAndBlanks(t *testing.T) {
	path := writeTemp(t, "# this is a comment\n\nagent_id=abc\n\n# another\nenv_id=xyz\n")
	out, err := ReadOutputsFile(path)
	require.NoError(t, err)
	assert.Equal(t, "abc", out["agent_id"])
	assert.Equal(t, "xyz", out["env_id"])
}

func TestReadOutputsFile_KeyValueMalformedLine(t *testing.T) {
	path := writeTemp(t, "agent_id=ok\nthis line has no equals\nenv_id=also_ok\n")
	_, err := ReadOutputsFile(path)
	require.Error(t, err, "lines without '=' should be a parse error")
}

func TestReadOutputsFile_KeyValueWithEqualsInValue(t *testing.T) {
	path := writeTemp(t, "url=https://example.com/path?a=1&b=2\n")
	out, err := ReadOutputsFile(path)
	require.NoError(t, err)
	assert.Equal(t, "https://example.com/path?a=1&b=2", out["url"],
		"only the first '=' should split key from value")
}

func TestReadOutputsFile_ReadErrorNotMissing(t *testing.T) {
	// Pointing ATMOS_OUTPUTS at a directory is a real misconfiguration. os.ReadFile
	// returns EISDIR (not os.IsNotExist), so this must surface a wrapped error
	// rather than being swallowed as "empty outputs".
	dir := t.TempDir()
	_, err := ReadOutputsFile(dir)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrReadOutputsFile)
}

func TestIsJSONObject(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want bool
	}{
		{name: "leading brace", raw: `{"k":"v"}`, want: true},
		{name: "brace after whitespace", raw: "  \n\t{}", want: true},
		{name: "key=value first char", raw: "key=value", want: false},
		{name: "empty input", raw: "", want: false},
		{name: "whitespace only", raw: "   \n\t ", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isJSONObject([]byte(tt.raw)))
		})
	}
}

func writeTemp(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "outputs")
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
	return path
}
