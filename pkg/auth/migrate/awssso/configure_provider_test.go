package awssso

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/migrate"
	"github.com/cloudposse/atmos/pkg/auth/migrate/mocks"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinel to detect field renames.
var _ = schema.Provider{Kind: "aws/iam-identity-center"}

func TestConfigureProvider_Detect_AlreadyConfigured(t *testing.T) {
	migCtx := &migrate.MigrationContext{
		ExistingAuth: &schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"my-sso": {Kind: "aws/iam-identity-center"},
			},
		},
	}
	step := NewConfigureProvider(migCtx, nil)

	status, err := step.Detect(context.Background())

	require.NoError(t, err)
	assert.Equal(t, migrate.StepComplete, status)
}

func TestConfigureProvider_Detect_NoAuth(t *testing.T) {
	migCtx := &migrate.MigrationContext{
		ExistingAuth: nil,
	}
	step := NewConfigureProvider(migCtx, nil)

	status, err := step.Detect(context.Background())

	require.NoError(t, err)
	assert.Equal(t, migrate.StepNeeded, status)
}

func TestConfigureProvider_Detect_NoSSOProvider(t *testing.T) {
	migCtx := &migrate.MigrationContext{
		ExistingAuth: &schema.AuthConfig{
			Providers: map[string]schema.Provider{
				"okta": {Kind: "okta/saml"},
			},
		},
	}
	step := NewConfigureProvider(migCtx, nil)

	status, err := step.Detect(context.Background())

	require.NoError(t, err)
	assert.Equal(t, migrate.StepNeeded, status)
}

func TestConfigureProvider_Plan(t *testing.T) {
	migCtx := &migrate.MigrationContext{
		AtmosConfigPath: "/tmp/atmos.yaml",
		SSOConfig: &migrate.SSOConfig{
			StartURL:     "https://myorg.awsapps.com/start",
			Region:       "us-east-1",
			ProviderName: "acme-sso",
		},
	}
	step := NewConfigureProvider(migCtx, nil)

	changes, err := step.Plan(context.Background())

	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Equal(t, "/tmp/atmos.yaml", changes[0].FilePath)
	assert.Equal(t, "Add SSO auth provider configuration", changes[0].Description)
	assert.Contains(t, changes[0].Detail, "kind: aws/iam-identity-center")
	assert.Contains(t, changes[0].Detail, "region: us-east-1")
	assert.Contains(t, changes[0].Detail, "start_url: https://myorg.awsapps.com/start")
	assert.Contains(t, changes[0].Detail, "acme-sso:")
	assert.Contains(t, changes[0].Detail, "auto_provision_identities: true")
	assert.Contains(t, changes[0].Detail, "duration: 12h")
	assert.Contains(t, changes[0].Detail, "session_duration: 12h")
}

func TestConfigureProvider_Plan_NoSSOConfig(t *testing.T) {
	migCtx := &migrate.MigrationContext{
		AtmosConfigPath: "/tmp/atmos.yaml",
		SSOConfig:       nil,
	}
	step := NewConfigureProvider(migCtx, nil)

	_, err := step.Plan(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "SSO configuration required")
}

func TestConfigureProvider_Apply(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	migCtx := &migrate.MigrationContext{
		AtmosConfigPath: "/tmp/atmos.yaml",
		SSOConfig: &migrate.SSOConfig{
			StartURL:     "https://myorg.awsapps.com/start",
			Region:       "us-west-2",
			ProviderName: "corp-sso",
		},
	}
	step := NewConfigureProvider(migCtx, mockFS)

	existingContent := []byte("# Atmos configuration\nbase_path: ./\n")

	mockFS.EXPECT().ReadFile("/tmp/atmos.yaml").Return(existingContent, nil)
	mockFS.EXPECT().WriteFile("/tmp/atmos.yaml", gomock.Any(), os.FileMode(0o644)).DoAndReturn(
		func(path string, data []byte, perm os.FileMode) error {
			content := string(data)
			// Verify existing content is preserved.
			assert.Contains(t, content, "# Atmos configuration")
			assert.Contains(t, content, "base_path: ./")
			// Verify auth block is appended.
			assert.Contains(t, content, "kind: aws/iam-identity-center")
			assert.Contains(t, content, "region: us-west-2")
			assert.Contains(t, content, "start_url: https://myorg.awsapps.com/start")
			assert.Contains(t, content, "corp-sso:")
			return nil
		},
	)

	err := step.Apply(context.Background())
	require.NoError(t, err)
}

func TestConfigureProvider_Apply_ReadError(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	migCtx := &migrate.MigrationContext{
		AtmosConfigPath: "/tmp/atmos.yaml",
		SSOConfig: &migrate.SSOConfig{
			StartURL:     "https://myorg.awsapps.com/start",
			Region:       "us-east-1",
			ProviderName: "sso",
		},
	}
	step := NewConfigureProvider(migCtx, mockFS)

	mockFS.EXPECT().ReadFile("/tmp/atmos.yaml").Return(nil, os.ErrNotExist)

	err := step.Apply(context.Background())

	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read atmos.yaml")
}

func TestConfigureProvider_NameAndDescription(t *testing.T) {
	step := NewConfigureProvider(&migrate.MigrationContext{}, nil)

	assert.Equal(t, "configure-provider", step.Name())
	assert.Equal(t, "Configure SSO provider in atmos.yaml", step.Description())
}
