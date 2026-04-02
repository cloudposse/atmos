package awssso

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/migrate"
	"github.com/cloudposse/atmos/pkg/auth/migrate/mocks"
)

func TestUpdateTfstateBackend_Detect_AlreadyConfigured(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	base := "/stacks"
	migCtx := &migrate.MigrationContext{
		StacksBasePath: base,
	}

	tfstateFile := filepath.Join(base, "catalog", "tfstate-backend.yaml")

	// File exists at the first candidate path.
	mockFS.EXPECT().Exists(tfstateFile).Return(true)

	contentWithPerms := []byte("vars:\n  enabled: true\nallowed_permission_sets:\n  dev:\n    - AdministratorAccess\n")
	mockFS.EXPECT().ReadFile(tfstateFile).Return(contentWithPerms, nil)

	step := NewUpdateTfstateBackend(migCtx, mockFS)
	status, err := step.Detect(context.Background())

	require.NoError(t, err)
	assert.Equal(t, migrate.StepComplete, status)
}

func TestUpdateTfstateBackend_Detect_NeedsUpdate(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	base := "/stacks"
	migCtx := &migrate.MigrationContext{
		StacksBasePath: base,
	}

	tfstateFile := filepath.Join(base, "catalog", "tfstate-backend.yaml")

	// File exists at the first candidate path.
	mockFS.EXPECT().Exists(tfstateFile).Return(true)

	contentWithout := []byte("vars:\n  enabled: true\n")
	mockFS.EXPECT().ReadFile(tfstateFile).Return(contentWithout, nil)

	step := NewUpdateTfstateBackend(migCtx, mockFS)
	status, err := step.Detect(context.Background())

	require.NoError(t, err)
	assert.Equal(t, migrate.StepNeeded, status)
}

func TestUpdateTfstateBackend_Detect_FileNotFound(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	base := "/stacks"
	migCtx := &migrate.MigrationContext{
		StacksBasePath: base,
	}

	tfstateFile := filepath.Join(base, "catalog", "tfstate-backend.yaml")
	tfstateFileNested := filepath.Join(base, "catalog", "tfstate-backend", "tfstate-backend.yaml")

	// Neither candidate path exists.
	mockFS.EXPECT().Exists(tfstateFile).Return(false)
	mockFS.EXPECT().Exists(tfstateFileNested).Return(false)

	step := NewUpdateTfstateBackend(migCtx, mockFS)
	status, err := step.Detect(context.Background())

	require.NoError(t, err)
	assert.Equal(t, migrate.StepNotApplicable, status)
}

func TestUpdateTfstateBackend_Plan(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	base := "/stacks"
	migCtx := &migrate.MigrationContext{
		StacksBasePath: base,
		AccountMap: map[string]string{
			"staging": "222222222222",
			"dev":     "111111111111",
			"prod":    "333333333333",
		},
	}

	tfstateFile := filepath.Join(base, "catalog", "tfstate-backend.yaml")

	// File exists at the first candidate path.
	mockFS.EXPECT().Exists(tfstateFile).Return(true)

	step := NewUpdateTfstateBackend(migCtx, mockFS)
	changes, err := step.Plan(context.Background())

	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Equal(t, tfstateFile, changes[0].FilePath)
	assert.Contains(t, changes[0].Description, "dev, prod, staging")
	// Verify accounts are sorted alphabetically in the YAML block.
	assert.Contains(t, changes[0].Detail, "allowed_permission_sets:")
	assert.Contains(t, changes[0].Detail, "  dev:")
	assert.Contains(t, changes[0].Detail, "  prod:")
	assert.Contains(t, changes[0].Detail, "  staging:")
	assert.Contains(t, changes[0].Detail, "    - AdministratorAccess")
	assert.Contains(t, changes[0].Detail, "    - Terraform*Access")
}

func TestUpdateTfstateBackend_Apply(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	base := "/stacks"
	migCtx := &migrate.MigrationContext{
		StacksBasePath: base,
		AccountMap: map[string]string{
			"staging": "222222222222",
			"dev":     "111111111111",
		},
	}

	tfstateFile := filepath.Join(base, "catalog", "tfstate-backend.yaml")

	// File exists at the first candidate path.
	mockFS.EXPECT().Exists(tfstateFile).Return(true)

	existingContent := []byte("vars:\n  enabled: true\n")
	mockFS.EXPECT().ReadFile(tfstateFile).Return(existingContent, nil)

	mockFS.EXPECT().WriteFile(tfstateFile, gomock.Any(), os.FileMode(0o644)).DoAndReturn(
		func(path string, data []byte, perm os.FileMode) error {
			content := string(data)
			// Verify existing content is preserved.
			assert.Contains(t, content, "vars:\n  enabled: true\n")
			// Verify allowed_permission_sets block is appended.
			assert.Contains(t, content, "allowed_permission_sets:")
			assert.Contains(t, content, "  dev:")
			assert.Contains(t, content, "  staging:")
			assert.Contains(t, content, "    - AdministratorAccess")
			assert.Contains(t, content, "    - Terraform*Access")
			return nil
		},
	)

	step := NewUpdateTfstateBackend(migCtx, mockFS)
	err := step.Apply(context.Background())

	require.NoError(t, err)
}

func TestUpdateTfstateBackend_Detect_NestedPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	base := "/stacks"
	migCtx := &migrate.MigrationContext{
		StacksBasePath: base,
	}

	tfstateFile := filepath.Join(base, "catalog", "tfstate-backend.yaml")
	tfstateFileNested := filepath.Join(base, "catalog", "tfstate-backend", "tfstate-backend.yaml")

	// First candidate does not exist, second does.
	mockFS.EXPECT().Exists(tfstateFile).Return(false)
	mockFS.EXPECT().Exists(tfstateFileNested).Return(true)

	contentWithout := []byte("vars:\n  enabled: true\n")
	mockFS.EXPECT().ReadFile(tfstateFileNested).Return(contentWithout, nil)

	step := NewUpdateTfstateBackend(migCtx, mockFS)
	status, err := step.Detect(context.Background())

	require.NoError(t, err)
	assert.Equal(t, migrate.StepNeeded, status)
}

func TestUpdateTfstateBackend_NameAndDescription(t *testing.T) {
	step := NewUpdateTfstateBackend(&migrate.MigrationContext{}, nil)

	assert.Equal(t, "update-tfstate-backend", step.Name())
	assert.Equal(t, "Add allowed_permission_sets to tfstate-backend.yaml for SSO access", step.Description())
}
