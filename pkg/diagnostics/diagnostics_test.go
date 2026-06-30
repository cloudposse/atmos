package diagnostics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEmitWithConfigWritesJSONL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")

	err := EmitWithConfig(Config{Enabled: true, File: path}, &Event{
		Type:    "process.start",
		ID:      "process-1",
		Command: "terraform",
		Args:    []string{"init"},
		CWD:     "/work",
		TTY:     Bool(true),
	})

	require.NoError(t, err)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), `"level"`)

	var event Event
	require.NoError(t, json.Unmarshal(data, &event))
	assert.Equal(t, "process.start", event.Type)
	assert.Equal(t, "process-1", event.ID)
	assert.Equal(t, "terraform", event.Command)
	assert.Equal(t, []string{"init"}, event.Args)
	assert.Equal(t, "/work", event.CWD)
	require.NotNil(t, event.TTY)
	assert.True(t, *event.TTY)
	assert.False(t, event.Time.IsZero())
}

func TestEmitWithConfigDisabledWithoutFile(t *testing.T) {
	dir := t.TempDir()
	err := EmitWithConfig(Config{Enabled: true}, &Event{Type: "process.start"})
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestEmitWithConfigDisabledByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	err := EmitWithConfig(Config{File: path}, &Event{Type: "process.start"})
	require.NoError(t, err)

	_, statErr := os.Stat(path)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}

func TestEmitWithConfigMasksStructuredFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	iolib.RegisterSecret("super-secret-token")

	err := EmitWithConfig(Config{Enabled: true, File: path}, &Event{
		Type:    "process.start",
		Command: "terraform",
		Args:    []string{"-var", "token=super-secret-token"},
		Error:   "failed with super-secret-token",
	})

	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), "super-secret-token")
	assert.Contains(t, string(data), "<MASKED>")
}

func TestOutputWriterOptInMasksOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")
	iolib.RegisterSecret("super-secret-output")

	writer := NewOutputWriter(Config{Enabled: true, File: path, IncludeOutput: true}, "process-1", "stderr")
	n, err := writer.Write([]byte("line with super-secret-output\n"))
	require.NoError(t, err)
	assert.Equal(t, len("line with super-secret-output\n"), n)

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.NotContains(t, string(data), `"level"`)

	var event Event
	require.NoError(t, json.Unmarshal(data, &event))
	assert.Equal(t, "process.output", event.Type)
	assert.Equal(t, "process-1", event.ID)
	assert.Equal(t, "stderr", event.Stream)
	assert.Equal(t, "line with <MASKED>\n", event.Data)
	require.NotNil(t, event.Bytes)
	assert.Equal(t, len("line with super-secret-output\n"), *event.Bytes)
	require.NotNil(t, event.Sequence)
	assert.Equal(t, uint64(1), *event.Sequence)
}

func TestOutputWriterDisabledByDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "events.jsonl")

	writer := NewOutputWriter(Config{Enabled: true, File: path}, "process-1", "stdout")
	n, err := writer.Write([]byte("ignored\n"))
	require.NoError(t, err)
	assert.Equal(t, len("ignored\n"), n)

	_, statErr := os.Stat(path)
	assert.ErrorIs(t, statErr, os.ErrNotExist)
}
