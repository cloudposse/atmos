package awssso

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/migrate"
	"github.com/cloudposse/atmos/pkg/auth/migrate/mocks"
)

func TestCleanupLegacy_Detect_NoLegacy(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	migCtx := &migrate.MigrationContext{
		AtmosConfigPath: filepath.Join("/tmp", "project", "atmos.yaml"),
	}
	step := NewCleanupLegacyAuth(migCtx, mockFS)

	authDir := filepath.Join("/tmp", "project", ".atmos.d", "auth")
	mockFS.EXPECT().Exists(authDir).Return(false)
	mockFS.EXPECT().ReadFile(migCtx.AtmosConfigPath).Return([]byte("base_path: ./\n"), nil)

	status, err := step.Detect(context.Background())

	require.NoError(t, err)
	assert.Equal(t, migrate.StepComplete, status)
}

func TestCleanupLegacy_Detect_AtmosDirExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	migCtx := &migrate.MigrationContext{
		AtmosConfigPath: filepath.Join("/tmp", "project", "atmos.yaml"),
	}
	step := NewCleanupLegacyAuth(migCtx, mockFS)

	authDir := filepath.Join("/tmp", "project", ".atmos.d", "auth")
	mockFS.EXPECT().Exists(authDir).Return(true)

	status, err := step.Detect(context.Background())

	require.NoError(t, err)
	assert.Equal(t, migrate.StepNeeded, status)
}

func TestCleanupLegacy_Detect_AssumeRoleFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	migCtx := &migrate.MigrationContext{
		AtmosConfigPath: filepath.Join("/tmp", "project", "atmos.yaml"),
	}
	step := NewCleanupLegacyAuth(migCtx, mockFS)

	authDir := filepath.Join("/tmp", "project", ".atmos.d", "auth")
	mockFS.EXPECT().Exists(authDir).Return(false)

	atmosYAML := []byte(`auth:
  identities:
    legacy:
      kind: aws/assume-role
      role_arn: arn:aws:iam::123456789012:role/admin
`)
	mockFS.EXPECT().ReadFile(migCtx.AtmosConfigPath).Return(atmosYAML, nil)

	status, err := step.Detect(context.Background())

	require.NoError(t, err)
	assert.Equal(t, migrate.StepNeeded, status)
}

func TestCleanupLegacy_Plan_BothFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	migCtx := &migrate.MigrationContext{
		AtmosConfigPath: filepath.Join("/tmp", "project", "atmos.yaml"),
	}
	step := NewCleanupLegacyAuth(migCtx, mockFS)

	authDir := filepath.Join("/tmp", "project", ".atmos.d", "auth")
	mockFS.EXPECT().Exists(authDir).Return(true)

	atmosYAML := []byte(`auth:
  identities:
    legacy:
      kind: aws/assume-role
`)
	mockFS.EXPECT().ReadFile(migCtx.AtmosConfigPath).Return(atmosYAML, nil)

	changes, err := step.Plan(context.Background())

	require.NoError(t, err)
	require.Len(t, changes, 2)

	// First change: directory removal.
	assert.Equal(t, authDir, changes[0].FilePath)
	assert.Contains(t, changes[0].Description, "Remove legacy .atmos.d/auth/ directory")

	// Second change: atmos.yaml cleanup.
	assert.Equal(t, migCtx.AtmosConfigPath, changes[1].FilePath)
	assert.Contains(t, changes[1].Description, "aws/assume-role")
}

func TestCleanupLegacy_Apply(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	migCtx := &migrate.MigrationContext{
		AtmosConfigPath: filepath.Join("/tmp", "project", "atmos.yaml"),
	}
	step := NewCleanupLegacyAuth(migCtx, mockFS)

	authDir := filepath.Join("/tmp", "project", ".atmos.d", "auth")
	mockFS.EXPECT().Exists(authDir).Return(false)
	mockFS.EXPECT().ReadFile(migCtx.AtmosConfigPath).Return([]byte("base_path: ./\n"), nil)

	// Apply is advisory-only and should return nil.
	err := step.Apply(context.Background())

	require.NoError(t, err)
}

func TestCleanupLegacy_NameAndDescription(t *testing.T) {
	step := NewCleanupLegacyAuth(&migrate.MigrationContext{}, nil)

	assert.Equal(t, "cleanup-legacy-auth", step.Name())
	assert.Contains(t, step.Description(), "legacy auth")
}
