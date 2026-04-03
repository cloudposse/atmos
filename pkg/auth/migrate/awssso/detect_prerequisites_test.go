package awssso

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/migrate"
	"github.com/cloudposse/atmos/pkg/auth/migrate/mocks"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestDetectPrerequisites_Name(t *testing.T) {
	t.Parallel()
	migCtx := &migrate.MigrationContext{}
	step := NewDetectPrerequisites(migCtx, nil)
	assert.Equal(t, "detect-prerequisites", step.Name())
}

func TestDetectPrerequisites_Description(t *testing.T) {
	t.Parallel()
	migCtx := &migrate.MigrationContext{}
	step := NewDetectPrerequisites(migCtx, nil)
	assert.Equal(t, "Check migration prerequisites", step.Description())
}

func TestDetectPrerequisites_WithSSOConfig_Complete(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	migCtx := &migrate.MigrationContext{
		SSOConfig: &migrate.SSOConfig{
			StartURL: "https://example.awsapps.com/start",
			AccountAssignments: map[string]map[string][]string{
				"DevOps": {"TerraformApplyAccess": {"dev"}},
			},
		},
	}
	step := NewDetectPrerequisites(migCtx, mockFS)

	status, err := step.Detect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, migrate.StepComplete, status)
}

func TestDetectPrerequisites_WithProvider_Complete(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	migCtx := &migrate.MigrationContext{
		SSOConfig: &migrate.SSOConfig{},
		ExistingAuth: &schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"sso": {Kind: "aws/iam-identity-center"},
			},
		},
	}
	step := NewDetectPrerequisites(migCtx, mockFS)

	status, err := step.Detect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, migrate.StepComplete, status)
}

func TestDetectPrerequisites_NoSSONoProvider_NotApplicable(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	migCtx := &migrate.MigrationContext{
		SSOConfig:    &migrate.SSOConfig{},
		ExistingAuth: &schema.AuthConfig{},
	}
	step := NewDetectPrerequisites(migCtx, mockFS)

	status, err := step.Detect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, migrate.StepNotApplicable, status)
}

func TestDetectPrerequisites_NilSSOConfig_NotApplicable(t *testing.T) {
	t.Parallel()
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	migCtx := &migrate.MigrationContext{}
	step := NewDetectPrerequisites(migCtx, mockFS)

	status, err := step.Detect(context.Background())
	require.NoError(t, err)
	assert.Equal(t, migrate.StepNotApplicable, status)
}

func TestDetectPrerequisites_PlanNoOp(t *testing.T) {
	t.Parallel()
	migCtx := &migrate.MigrationContext{}
	step := NewDetectPrerequisites(migCtx, nil)

	changes, err := step.Plan(context.Background())
	require.NoError(t, err)
	assert.Nil(t, changes)
}

func TestDetectPrerequisites_ApplyNoOp(t *testing.T) {
	t.Parallel()
	migCtx := &migrate.MigrationContext{}
	step := NewDetectPrerequisites(migCtx, nil)

	err := step.Apply(context.Background())
	require.NoError(t, err)
}
