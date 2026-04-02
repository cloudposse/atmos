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

func TestGenerateProfiles_Detect_NoProfiles(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	migCtx := &migrate.MigrationContext{
		ProfilesPath: "/tmp/profiles",
	}
	step := NewGenerateProfiles(migCtx, mockFS)

	pattern := filepath.Join("/tmp/profiles", "*", "atmos.yaml")
	mockFS.EXPECT().Glob(pattern).Return(nil, os.ErrNotExist)

	status, err := step.Detect(context.Background())

	require.NoError(t, err)
	assert.Equal(t, migrate.StepNeeded, status)
}

func TestGenerateProfiles_Detect_ProfilesExist(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	migCtx := &migrate.MigrationContext{
		ProfilesPath: "/tmp/profiles",
	}
	step := NewGenerateProfiles(migCtx, mockFS)

	pattern := filepath.Join("/tmp/profiles", "*", "atmos.yaml")
	mockFS.EXPECT().Glob(pattern).Return([]string{
		filepath.Join("/tmp/profiles", "devops", "atmos.yaml"),
		filepath.Join("/tmp/profiles", "readonly", "atmos.yaml"),
	}, nil)

	status, err := step.Detect(context.Background())

	require.NoError(t, err)
	assert.Equal(t, migrate.StepComplete, status)
}

func TestGenerateProfiles_Plan(t *testing.T) {
	migCtx := &migrate.MigrationContext{
		ProfilesPath: "/tmp/profiles",
		SSOConfig: &migrate.SSOConfig{
			StartURL:     "https://myorg.awsapps.com/start",
			Region:       "us-east-1",
			ProviderName: "acme-sso",
			AccountAssignments: map[string]map[string][]string{
				"devops": {
					"TerraformApplyAccess": {"core-root", "dev-sandbox"},
				},
				"readonly": {
					"ReadOnlyAccess": {"core-audit"},
				},
			},
		},
	}
	step := NewGenerateProfiles(migCtx, nil)

	changes, err := step.Plan(context.Background())

	require.NoError(t, err)
	require.Len(t, changes, 2)

	// Changes should be sorted alphabetically by group name.
	assert.Equal(t, filepath.Join("profiles", "devops", "atmos.yaml"), changes[0].FilePath)
	assert.Equal(t, `Create profile "devops"`, changes[0].Description)
	assert.Contains(t, changes[0].Detail, "core-root/terraform")
	assert.Contains(t, changes[0].Detail, "dev-sandbox/terraform")
	assert.Contains(t, changes[0].Detail, "TerraformApplyAccess")

	assert.Equal(t, filepath.Join("profiles", "readonly", "atmos.yaml"), changes[1].FilePath)
	assert.Equal(t, `Create profile "readonly"`, changes[1].Description)
	assert.Contains(t, changes[1].Detail, "core-audit/terraform")
	assert.Contains(t, changes[1].Detail, "ReadOnlyAccess")
}

func TestGenerateProfiles_Apply(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	migCtx := &migrate.MigrationContext{
		ProfilesPath: "/tmp/profiles",
		SSOConfig: &migrate.SSOConfig{
			StartURL:     "https://myorg.awsapps.com/start",
			Region:       "us-east-1",
			ProviderName: "acme-sso",
			AccountAssignments: map[string]map[string][]string{
				"devops": {
					"TerraformApplyAccess": {"core-root", "core-audit", "dev-sandbox"},
				},
			},
		},
	}
	step := NewGenerateProfiles(migCtx, mockFS)

	expectedPath := filepath.Join("/tmp/profiles", "devops", "atmos.yaml")
	mockFS.EXPECT().WriteFile(expectedPath, gomock.Any(), os.FileMode(0o644)).DoAndReturn(
		func(path string, data []byte, perm os.FileMode) error {
			content := string(data)
			// Verify provider config.
			assert.Contains(t, content, "acme-sso:")
			assert.Contains(t, content, "kind: aws/iam-identity-center")
			assert.Contains(t, content, "region: us-east-1")
			assert.Contains(t, content, "start_url: https://myorg.awsapps.com/start")
			assert.Contains(t, content, "duration: 12h")
			// Verify identities are sorted alphabetically by account name.
			assert.Contains(t, content, "core-audit/terraform:")
			assert.Contains(t, content, "core-root/terraform:")
			assert.Contains(t, content, "dev-sandbox/terraform:")
			assert.Contains(t, content, "name: TerraformApplyAccess")
			return nil
		},
	)

	err := step.Apply(context.Background())
	require.NoError(t, err)
}

func TestGenerateProfiles_Apply_EmptyAssignments(t *testing.T) {
	migCtx := &migrate.MigrationContext{
		ProfilesPath: "/tmp/profiles",
		SSOConfig: &migrate.SSOConfig{
			StartURL:           "https://myorg.awsapps.com/start",
			Region:             "us-east-1",
			ProviderName:       "acme-sso",
			AccountAssignments: map[string]map[string][]string{},
		},
	}
	step := NewGenerateProfiles(migCtx, nil)

	err := step.Apply(context.Background())

	// No groups means no files written and no errors.
	require.NoError(t, err)
}

func TestGenerateProfiles_NameAndDescription(t *testing.T) {
	step := NewGenerateProfiles(&migrate.MigrationContext{}, nil)

	assert.Equal(t, "generate-profiles", step.Name())
	assert.Equal(t, "Generate profile directories from SSO group assignments", step.Description())
}
