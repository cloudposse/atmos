package output

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDefaultWorkspaceManager_CleanWorkspace(t *testing.T) {
	t.Run("removes environment file when it exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		tfDir := filepath.Join(tmpDir, ".terraform")
		err := os.MkdirAll(tfDir, 0o755)
		require.NoError(t, err)

		envFile := filepath.Join(tfDir, "environment")
		err = os.WriteFile(envFile, []byte("test-workspace"), 0o644)
		require.NoError(t, err)

		mgr := &defaultWorkspaceManager{}
		atmosConfig := &schema.AtmosConfiguration{}

		mgr.CleanWorkspace(atmosConfig, tmpDir)

		// Verify file is removed.
		_, err = os.Stat(envFile)
		assert.True(t, os.IsNotExist(err))
	})

	t.Run("handles non-existent environment file", func(t *testing.T) {
		tmpDir := t.TempDir()

		mgr := &defaultWorkspaceManager{}
		atmosConfig := &schema.AtmosConfiguration{}

		// Should not panic.
		mgr.CleanWorkspace(atmosConfig, tmpDir)
	})

	t.Run("uses custom TF_DATA_DIR", func(t *testing.T) {
		tmpDir := t.TempDir()
		customTfDir := filepath.Join(tmpDir, ".custom-terraform")
		err := os.MkdirAll(customTfDir, 0o755)
		require.NoError(t, err)

		envFile := filepath.Join(customTfDir, "environment")
		err = os.WriteFile(envFile, []byte("custom-workspace"), 0o644)
		require.NoError(t, err)

		// Set custom TF_DATA_DIR.
		t.Setenv("TF_DATA_DIR", ".custom-terraform")

		mgr := &defaultWorkspaceManager{}
		atmosConfig := &schema.AtmosConfiguration{}

		mgr.CleanWorkspace(atmosConfig, tmpDir)

		// Verify file is removed.
		_, err = os.Stat(envFile)
		assert.True(t, os.IsNotExist(err))
	})
}

func TestDefaultWorkspaceManager_EnsureWorkspace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	t.Run("skips for http backend", func(t *testing.T) {
		mockRunner := NewMockTerraformRunner(ctrl)
		mgr := &defaultWorkspaceManager{}

		err := mgr.EnsureWorkspace(context.Background(), mockRunner, "workspace", "http", "component", "stack", nil)
		assert.NoError(t, err)
	})

	t.Run("selects existing workspace successfully", func(t *testing.T) {
		mockRunner := NewMockTerraformRunner(ctrl)
		mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "existing-workspace").Return(nil)

		mgr := &defaultWorkspaceManager{}

		err := mgr.EnsureWorkspace(context.Background(), mockRunner, "existing-workspace", "s3", "component", "stack", nil)
		assert.NoError(t, err)
	})

	t.Run("creates new workspace when select fails with missing workspace error", func(t *testing.T) {
		mockRunner := NewMockTerraformRunner(ctrl)
		// Use realistic Terraform error message for a missing workspace.
		mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(errors.New(`Workspace "test-workspace" doesn't exist.`))
		mockRunner.EXPECT().WorkspaceNew(gomock.Any(), "test-workspace").Return(nil)

		mgr := &defaultWorkspaceManager{}

		err := mgr.EnsureWorkspace(context.Background(), mockRunner, "test-workspace", "s3", "component", "stack", nil)
		assert.NoError(t, err)
	})

	t.Run("fails when missing workspace select fails and create also fails", func(t *testing.T) {
		mockRunner := NewMockTerraformRunner(ctrl)
		// Select fails with missing workspace → falls back to WorkspaceNew → which also fails.
		mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "workspace").Return(errors.New(`workspace "workspace" does not exist`))
		mockRunner.EXPECT().WorkspaceNew(gomock.Any(), "workspace").Return(errors.New("create failed"))

		mgr := &defaultWorkspaceManager{}

		err := mgr.EnsureWorkspace(context.Background(), mockRunner, "workspace", "s3", "component", "stack", nil)
		assert.Error(t, err)
	})

	t.Run("fails fast on non-missing select error without trying create", func(t *testing.T) {
		mockRunner := NewMockTerraformRunner(ctrl)
		// Select fails with a permission error — NOT a missing workspace.
		// WorkspaceNew should NOT be called.
		mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "workspace").Return(errors.New("permission denied"))
		// No WorkspaceNew expectation — gomock will fail if it's called.

		mgr := &defaultWorkspaceManager{}

		err := mgr.EnsureWorkspace(context.Background(), mockRunner, "workspace", "s3", "component", "stack", nil)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "permission denied")
	})
}

func TestIsWorkspaceExistsError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "workspace already exists error",
			err:      errors.New("Workspace test already exists"),
			expected: true,
		},
		{
			name:     "lowercase already exists",
			err:      errors.New("workspace already exists"),
			expected: true,
		},
		{
			name:     "permission denied",
			err:      errors.New("permission denied"),
			expected: false,
		},
		{
			name:     "network error",
			err:      errors.New("network unreachable"),
			expected: false,
		},
		{
			name:     "contains already exists in message",
			err:      errors.New("Error: workspace 'test' already exists"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWorkspaceExistsError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestIsWorkspaceMissingError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "nil error",
			err:      nil,
			expected: false,
		},
		{
			name:     "terraform missing workspace",
			err:      errors.New(`Workspace "dev" doesn't exist.`),
			expected: true,
		},
		{
			name:     "opentofu missing workspace",
			err:      errors.New(`workspace "dev" does not exist`),
			expected: true,
		},
		{
			name:     "permission denied — not a missing error",
			err:      errors.New("permission denied"),
			expected: false,
		},
		{
			name:     "backend error — not a missing error",
			err:      errors.New("Error loading state: AccessDenied"),
			expected: false,
		},
		{
			name:     "network error — not a missing error",
			err:      errors.New("dial tcp: connection refused"),
			expected: false,
		},
		{
			name:     "bucket does not exist — not a workspace missing error",
			err:      errors.New("Error loading state: bucket does not exist"),
			expected: false,
		},
		{
			name:     "resource does not exist — not a workspace missing error",
			err:      errors.New("the resource does not exist"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isWorkspaceMissingError(tt.err)
			assert.Equal(t, tt.expected, result)
		})
	}
}
