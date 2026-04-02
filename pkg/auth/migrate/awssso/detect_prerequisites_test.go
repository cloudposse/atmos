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

// newTestStep creates a DetectPrerequisites step with the given base path and mock fs.
func newTestStep(base string, fs migrate.FileSystem) *DetectPrerequisites {
	migCtx := &migrate.MigrationContext{
		StacksBasePath: base,
	}
	return NewDetectPrerequisites(migCtx, fs)
}

func TestDetectPrerequisites_Name(t *testing.T) {
	t.Parallel()
	step := newTestStep("/stacks", nil)
	assert.Equal(t, "detect-prerequisites", step.Name())
}

func TestDetectPrerequisites_Description(t *testing.T) {
	t.Parallel()
	step := newTestStep("/stacks", nil)
	assert.Equal(t, "Check migration prerequisites", step.Description())
}

func TestDetectPrerequisites_AwsTeamsFound(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	base := filepath.Join("/", "project", "stacks")
	step := newTestStep(base, mockFS)

	// aws-teams.yaml found at catalog root.
	mockFS.EXPECT().Exists(filepath.Join(base, "catalog", "aws-teams.yaml")).Return(true)

	status, err := step.Detect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, migrate.StepNotApplicable, status)
}

func TestDetectPrerequisites_AwsTeamRolesFound(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	base := filepath.Join("/", "project", "stacks")
	step := newTestStep(base, mockFS)

	// aws-teams.yaml not found at catalog root.
	mockFS.EXPECT().Exists(filepath.Join(base, "catalog", "aws-teams.yaml")).Return(false)
	// aws-teams.yaml not found in subdirectories.
	mockFS.EXPECT().Glob(filepath.Join(base, "catalog", "*", "aws-teams.yaml")).Return(nil, nil)
	// aws-team-roles.yaml found at catalog root.
	mockFS.EXPECT().Exists(filepath.Join(base, "catalog", "aws-team-roles.yaml")).Return(true)

	status, err := step.Detect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, migrate.StepNotApplicable, status)
}

func TestDetectPrerequisites_AwsTeamsFoundInSubdir(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	base := filepath.Join("/", "project", "stacks")
	step := newTestStep(base, mockFS)

	// aws-teams.yaml not at catalog root, but found in subdirectory.
	mockFS.EXPECT().Exists(filepath.Join(base, "catalog", "aws-teams.yaml")).Return(false)
	mockFS.EXPECT().Glob(filepath.Join(base, "catalog", "*", "aws-teams.yaml")).Return(
		[]string{filepath.Join(base, "catalog", "iam", "aws-teams.yaml")}, nil,
	)

	status, err := step.Detect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, migrate.StepNotApplicable, status)
}

func TestDetectPrerequisites_NoTeamsNoSSO(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	base := filepath.Join("/", "project", "stacks")
	step := newTestStep(base, mockFS)

	// Neither aws-teams.yaml nor aws-team-roles.yaml found anywhere.
	mockFS.EXPECT().Exists(filepath.Join(base, "catalog", "aws-teams.yaml")).Return(false)
	mockFS.EXPECT().Glob(filepath.Join(base, "catalog", "*", "aws-teams.yaml")).Return(nil, nil)
	mockFS.EXPECT().Exists(filepath.Join(base, "catalog", "aws-team-roles.yaml")).Return(false)
	mockFS.EXPECT().Glob(filepath.Join(base, "catalog", "*", "aws-team-roles.yaml")).Return(nil, nil)

	status, err := step.Detect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, migrate.StepNeeded, status)
}

func TestDetectPrerequisites_GlobError(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	base := filepath.Join("/", "project", "stacks")
	step := newTestStep(base, mockFS)

	// Exists returns false, then Glob returns an error.
	mockFS.EXPECT().Exists(filepath.Join(base, "catalog", "aws-teams.yaml")).Return(false)
	mockFS.EXPECT().Glob(filepath.Join(base, "catalog", "*", "aws-teams.yaml")).Return(nil, assert.AnError)

	status, err := step.Detect(context.Background())
	require.Error(t, err)
	assert.Equal(t, migrate.StepNotApplicable, status)
	assert.ErrorContains(t, err, "migration prerequisites not met")
}

func TestDetectPrerequisites_PlanNoOp(t *testing.T) {
	t.Parallel()
	step := newTestStep("/stacks", nil)

	changes, err := step.Plan(context.Background())
	require.NoError(t, err)
	assert.Nil(t, changes)
}

func TestDetectPrerequisites_ApplyNoOp(t *testing.T) {
	t.Parallel()
	step := newTestStep("/stacks", nil)

	err := step.Apply(context.Background())
	require.NoError(t, err)
}
