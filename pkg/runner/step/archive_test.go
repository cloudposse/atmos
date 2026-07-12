package step

import (
	"archive/zip"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func mustGetArchiveHandler(t *testing.T) StepHandler {
	t.Helper()
	h, ok := Get("archive")
	require.True(t, ok, "archive handler must be registered")
	require.NotNil(t, h)
	return h
}

func TestArchiveHandler_Validate(t *testing.T) {
	tests := []struct {
		name    string
		step    *schema.WorkflowStep
		wantErr error
	}{
		{
			name:    "missing source",
			step:    &schema.WorkflowStep{Name: "pkg", Type: "archive", Destination: "out.zip"},
			wantErr: errUtils.ErrArchiveSourceRequired,
		},
		{
			name:    "source not a string",
			step:    &schema.WorkflowStep{Name: "pkg", Type: "archive", Source: map[string]any{"uri": "x"}, Destination: "out.zip"},
			wantErr: errUtils.ErrArchiveStepInvalidSource,
		},
		{
			name:    "missing destination",
			step:    &schema.WorkflowStep{Name: "pkg", Type: "archive", Source: "src/"},
			wantErr: errUtils.ErrStepFieldRequired,
		},
		{
			name:    "invalid action",
			step:    &schema.WorkflowStep{Name: "pkg", Type: "archive", Source: "src/", Destination: "out.zip", Action: "compress"},
			wantErr: errUtils.ErrArchiveStepInvalidAction,
		},
		{
			name: "valid, action defaults to replace",
			step: &schema.WorkflowStep{Name: "pkg", Type: "archive", Source: "src/", Destination: "out.zip"},
		},
		{
			name: "valid explicit action",
			step: &schema.WorkflowStep{Name: "pkg", Type: "archive", Source: "src/", Destination: "out.zip", Action: "update"},
		},
		{
			name:    "empty source string",
			step:    &schema.WorkflowStep{Name: "pkg", Type: "archive", Source: "", Destination: "out.zip"},
			wantErr: errUtils.ErrArchiveSourceRequired,
		},
	}

	handler := mustGetArchiveHandler(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := handler.Validate(tt.step)
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.True(t, errors.Is(err, tt.wantErr), "got %v, want %v", err, tt.wantErr)
		})
	}
}

func TestArchiveHandler_Execute(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "handler.js"), []byte("exports.handler = 1;"), 0o644))
	dest := filepath.Join(dir, "handler.zip")

	handler := mustGetArchiveHandler(t)
	step := &schema.WorkflowStep{Name: "pkg", Type: "archive", Source: src, Destination: dest}

	result, err := handler.Execute(context.Background(), step, NewVariables())
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, dest, result.Value)
	assert.Equal(t, "replace", result.Metadata["action"])

	r, err := zip.OpenReader(dest)
	require.NoError(t, err)
	defer r.Close()
	require.Len(t, r.File, 1)
	rc, err := r.File[0].Open()
	require.NoError(t, err)
	defer rc.Close()
	content, err := io.ReadAll(rc)
	require.NoError(t, err)
	assert.Equal(t, "exports.handler = 1;", string(content))
}

func TestArchiveHandler_Execute_ResolvesTemplatedFields(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "handler.js"), []byte("x"), 0o644))

	vars := NewVariables()
	vars.SetEnv("ATMOS_COMPONENT", "lambda")

	step := &schema.WorkflowStep{
		Name:        "pkg",
		Type:        "archive",
		Source:      src,
		Destination: filepath.Join(dir, "{{ .Env.ATMOS_COMPONENT }}.zip"),
	}

	result, err := mustGetArchiveHandler(t).Execute(context.Background(), step, vars)
	require.NoError(t, err)
	assert.Equal(t, filepath.Join(dir, "lambda.zip"), result.Value)
	assert.FileExists(t, filepath.Join(dir, "lambda.zip"))
}

func TestArchiveHandler_Execute_PropagatesArchiveError(t *testing.T) {
	dir := t.TempDir()
	step := &schema.WorkflowStep{
		Name:        "pkg",
		Type:        "archive",
		Source:      filepath.Join(dir, "does-not-exist"),
		Destination: filepath.Join(dir, "out.zip"),
	}

	_, err := mustGetArchiveHandler(t).Execute(context.Background(), step, NewVariables())
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrArchiveSourceNotFound))
}

func TestArchiveHandler_Execute_WithIncludeExclude(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	require.NoError(t, os.MkdirAll(src, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(src, "handler.js"), []byte("x"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(src, "handler.test.js"), []byte("test"), 0o644))
	dest := filepath.Join(dir, "out.zip")

	step := &schema.WorkflowStep{
		Name:        "pkg",
		Type:        "archive",
		Source:      src,
		Destination: dest,
		Include:     []string{"**/*.js"},
		Exclude:     []string{"**/*.test.js"},
	}

	result, err := mustGetArchiveHandler(t).Execute(context.Background(), step, NewVariables())
	require.NoError(t, err)
	assert.Equal(t, dest, result.Value)

	r, err := zip.OpenReader(dest)
	require.NoError(t, err)
	defer r.Close()
	require.Len(t, r.File, 1)
	assert.Equal(t, "handler.js", r.File[0].Name)
}

// TestArchiveHandler_Execute_ResolveErrors covers every field resolveArchiveOptions
// templates: an error in any one of them must propagate out of Execute, not just
// the ones exercised by other tests (source, destination).
func TestArchiveHandler_Execute_ResolveErrors(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src")
	require.NoError(t, os.MkdirAll(src, 0o755))
	dest := filepath.Join(dir, "out.zip")
	const malformed = "{{ .Bad"

	tests := []struct {
		name string
		step *schema.WorkflowStep
	}{
		{
			name: "source is not a string",
			step: &schema.WorkflowStep{Name: "pkg", Type: "archive", Source: 5, Destination: dest},
		},
		{
			name: "malformed source template",
			step: &schema.WorkflowStep{Name: "pkg", Type: "archive", Source: malformed, Destination: dest},
		},
		{
			name: "malformed destination template",
			step: &schema.WorkflowStep{Name: "pkg", Type: "archive", Source: src, Destination: malformed},
		},
		{
			name: "malformed format template",
			step: &schema.WorkflowStep{Name: "pkg", Type: "archive", Source: src, Destination: dest, Format: malformed},
		},
		{
			name: "malformed subpath template",
			step: &schema.WorkflowStep{Name: "pkg", Type: "archive", Source: src, Destination: dest, Subpath: malformed},
		},
		{
			name: "malformed include template",
			step: &schema.WorkflowStep{Name: "pkg", Type: "archive", Source: src, Destination: dest, Include: []string{malformed}},
		},
		{
			name: "malformed exclude template",
			step: &schema.WorkflowStep{Name: "pkg", Type: "archive", Source: src, Destination: dest, Exclude: []string{malformed}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := mustGetArchiveHandler(t).Execute(context.Background(), tt.step, NewVariables())
			require.Error(t, err)
		})
	}
}
