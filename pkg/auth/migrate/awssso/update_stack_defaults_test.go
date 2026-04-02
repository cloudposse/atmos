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

func TestUpdateStackDefaults_Detect_AllHaveAuth(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	base := "/stacks"
	migCtx := &migrate.MigrationContext{
		StacksBasePath: base,
	}

	pattern3 := filepath.Join(base, "orgs", "*", "*", "*", "_defaults.yaml")
	pattern2 := filepath.Join(base, "orgs", "*", "*", "_defaults.yaml")

	devFile := filepath.Join(base, "orgs", "acme", "plat", "dev", "_defaults.yaml")
	stagingFile := filepath.Join(base, "orgs", "acme", "plat", "staging", "_defaults.yaml")

	mockFS.EXPECT().Glob(pattern3).Return([]string{devFile, stagingFile}, nil)
	mockFS.EXPECT().Glob(pattern2).Return(nil, nil)

	// Both files have auth identity config.
	contentWithAuth := []byte("terraform:\n  auth:\n    identities:\n      dev/terraform:\n        default: true\n")
	mockFS.EXPECT().ReadFile(devFile).Return(contentWithAuth, nil)
	mockFS.EXPECT().ReadFile(stagingFile).Return(contentWithAuth, nil)

	step := NewUpdateStackDefaults(migCtx, mockFS)
	status, err := step.Detect(context.Background())

	require.NoError(t, err)
	assert.Equal(t, migrate.StepComplete, status)
}

func TestUpdateStackDefaults_Detect_SomeMissing(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	base := "/stacks"
	migCtx := &migrate.MigrationContext{
		StacksBasePath: base,
	}

	pattern3 := filepath.Join(base, "orgs", "*", "*", "*", "_defaults.yaml")
	pattern2 := filepath.Join(base, "orgs", "*", "*", "_defaults.yaml")

	devFile := filepath.Join(base, "orgs", "acme", "plat", "dev", "_defaults.yaml")
	stagingFile := filepath.Join(base, "orgs", "acme", "plat", "staging", "_defaults.yaml")

	mockFS.EXPECT().Glob(pattern3).Return([]string{devFile, stagingFile}, nil)
	mockFS.EXPECT().Glob(pattern2).Return(nil, nil)

	// First file has auth, second does not.
	contentWithAuth := []byte("terraform:\n  auth:\n    identities:\n      dev/terraform:\n        default: true\n")
	contentWithout := []byte("vars:\n  stage: staging\n")

	mockFS.EXPECT().ReadFile(devFile).Return(contentWithAuth, nil)
	mockFS.EXPECT().ReadFile(stagingFile).Return(contentWithout, nil)

	step := NewUpdateStackDefaults(migCtx, mockFS)
	status, err := step.Detect(context.Background())

	require.NoError(t, err)
	assert.Equal(t, migrate.StepNeeded, status)
}

func TestUpdateStackDefaults_Detect_NoFiles(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	base := "/stacks"
	migCtx := &migrate.MigrationContext{
		StacksBasePath: base,
	}

	pattern3 := filepath.Join(base, "orgs", "*", "*", "*", "_defaults.yaml")
	pattern2 := filepath.Join(base, "orgs", "*", "*", "_defaults.yaml")

	mockFS.EXPECT().Glob(pattern3).Return(nil, nil)
	mockFS.EXPECT().Glob(pattern2).Return(nil, nil)

	step := NewUpdateStackDefaults(migCtx, mockFS)
	status, err := step.Detect(context.Background())

	require.NoError(t, err)
	assert.Equal(t, migrate.StepComplete, status)
}

func TestUpdateStackDefaults_Plan(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	base := "/stacks"
	migCtx := &migrate.MigrationContext{
		StacksBasePath: base,
		AccountMap:     map[string]string{"dev": "111111111111", "staging": "222222222222"},
	}

	pattern3 := filepath.Join(base, "orgs", "*", "*", "*", "_defaults.yaml")
	pattern2 := filepath.Join(base, "orgs", "*", "*", "_defaults.yaml")

	devFile := filepath.Join(base, "orgs", "acme", "plat", "dev", "_defaults.yaml")
	stagingFile := filepath.Join(base, "orgs", "acme", "plat", "staging", "_defaults.yaml")

	mockFS.EXPECT().Glob(pattern3).Return([]string{devFile, stagingFile}, nil)
	mockFS.EXPECT().Glob(pattern2).Return(nil, nil)

	// dev has auth, staging does not.
	contentWithAuth := []byte("terraform:\n  auth:\n    identities:\n      dev/terraform:\n        default: true\n")
	contentWithout := []byte("vars:\n  stage: staging\n")

	mockFS.EXPECT().ReadFile(devFile).Return(contentWithAuth, nil)
	mockFS.EXPECT().ReadFile(stagingFile).Return(contentWithout, nil)

	step := NewUpdateStackDefaults(migCtx, mockFS)
	changes, err := step.Plan(context.Background())

	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Equal(t, stagingFile, changes[0].FilePath)
	assert.Equal(t, "Add terraform auth identity for staging", changes[0].Description)
	assert.Contains(t, changes[0].Detail, "staging/terraform:")
	assert.Contains(t, changes[0].Detail, "default: true")
}

func TestUpdateStackDefaults_Apply(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	base := "/stacks"
	migCtx := &migrate.MigrationContext{
		StacksBasePath: base,
	}

	pattern3 := filepath.Join(base, "orgs", "*", "*", "*", "_defaults.yaml")
	pattern2 := filepath.Join(base, "orgs", "*", "*", "_defaults.yaml")

	devFile := filepath.Join(base, "orgs", "acme", "plat", "dev", "_defaults.yaml")

	mockFS.EXPECT().Glob(pattern3).Return([]string{devFile}, nil)
	mockFS.EXPECT().Glob(pattern2).Return(nil, nil)

	existingContent := []byte("vars:\n  stage: dev\n")

	// First ReadFile call is from fileHasAuthIdentity (Detect within Apply).
	mockFS.EXPECT().ReadFile(devFile).Return(existingContent, nil)
	// Second ReadFile call is from Apply reading the file to append.
	mockFS.EXPECT().ReadFile(devFile).Return(existingContent, nil)

	mockFS.EXPECT().WriteFile(devFile, gomock.Any(), os.FileMode(0o644)).DoAndReturn(
		func(path string, data []byte, perm os.FileMode) error {
			content := string(data)
			// Verify existing content is preserved.
			assert.Contains(t, content, "vars:\n  stage: dev\n")
			// Verify auth identity block is appended.
			assert.Contains(t, content, "terraform:")
			assert.Contains(t, content, "identities:")
			assert.Contains(t, content, "dev/terraform:")
			assert.Contains(t, content, "default: true")
			return nil
		},
	)

	step := NewUpdateStackDefaults(migCtx, mockFS)
	err := step.Apply(context.Background())

	require.NoError(t, err)
}

func TestUpdateStackDefaults_NameAndDescription(t *testing.T) {
	step := NewUpdateStackDefaults(&migrate.MigrationContext{}, nil)

	assert.Equal(t, "update-stack-defaults", step.Name())
	assert.Equal(t, "Add terraform auth identity defaults to stack _defaults.yaml files", step.Description())
}
