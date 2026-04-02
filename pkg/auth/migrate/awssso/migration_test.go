package awssso

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/migrate"
	"github.com/cloudposse/atmos/pkg/auth/migrate/mocks"
)

func TestNewAWSSSOSteps(t *testing.T) {
	t.Parallel()

	migCtx := &migrate.MigrationContext{
		StacksBasePath: "/stacks",
	}
	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)

	steps := NewAWSSSOSteps(migCtx, mockFS)

	require.Len(t, steps, 6)
	assert.Equal(t, "detect-prerequisites", steps[0].Name())
	assert.Equal(t, "configure-provider", steps[1].Name())
	assert.Equal(t, "generate-profiles", steps[2].Name())
	assert.Equal(t, "update-stack-defaults", steps[3].Name())
	assert.Equal(t, "update-tfstate-backend", steps[4].Name())
	assert.Equal(t, "cleanup-legacy-auth", steps[5].Name())
}

func TestParseAccountMap(t *testing.T) {
	t.Parallel()

	data := []byte(`vars:
  full_account_map:
    core-root: "111111111111"
    core-audit: "222222222222"
    dev-sandbox: "333333333333"
`)

	result, err := parseAccountMap(data)
	require.NoError(t, err)
	require.Len(t, result, 3)
	assert.Equal(t, "111111111111", result["core-root"])
	assert.Equal(t, "222222222222", result["core-audit"])
	assert.Equal(t, "333333333333", result["dev-sandbox"])
}

func TestParseAccountMap_AlternateKey(t *testing.T) {
	t.Parallel()

	data := []byte(`vars:
  account_map:
    prod: "444444444444"
    staging: "555555555555"
`)

	result, err := parseAccountMap(data)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "444444444444", result["prod"])
	assert.Equal(t, "555555555555", result["staging"])
}

func TestParseAccountMap_FullAccountMapTakesPrecedence(t *testing.T) {
	t.Parallel()

	data := []byte(`vars:
  full_account_map:
    primary: "111111111111"
  account_map:
    fallback: "999999999999"
`)

	result, err := parseAccountMap(data)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "111111111111", result["primary"])
}

func TestParseAccountMap_EmptyData(t *testing.T) {
	t.Parallel()

	_, err := parseAccountMap([]byte{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestParseAccountMap_MissingVars(t *testing.T) {
	t.Parallel()

	data := []byte(`settings:
  foo: bar
`)

	_, err := parseAccountMap(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'vars'")
}

func TestParseAccountMap_MissingKeys(t *testing.T) {
	t.Parallel()

	data := []byte(`vars:
  other_key:
    foo: bar
`)

	_, err := parseAccountMap(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'vars.full_account_map' or 'vars.account_map'")
}

func TestParseAccountMap_MalformedYAML(t *testing.T) {
	t.Parallel()

	data := []byte(`{invalid yaml:::`)

	_, err := parseAccountMap(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing account map YAML")
}

func TestParseSSOConfig(t *testing.T) {
	t.Parallel()

	data := []byte(`vars:
  start_url: "https://example.awsapps.com/start"
  region: "us-west-2"
  account_assignments:
    DevOps:
      TerraformApplyAccess:
        - core-root
        - core-audit
      AdministratorAccess:
        - core-root
    Developers:
      TerraformPlanAccess:
        - core-root
`)

	result, err := parseSSOConfig(data)
	require.NoError(t, err)
	assert.Equal(t, "https://example.awsapps.com/start", result.StartURL)
	assert.Equal(t, "us-west-2", result.Region)
	require.Len(t, result.AccountAssignments, 2)
	require.Len(t, result.AccountAssignments["DevOps"]["TerraformApplyAccess"], 2)
	assert.Equal(t, "core-root", result.AccountAssignments["DevOps"]["TerraformApplyAccess"][0])
	assert.Equal(t, "core-audit", result.AccountAssignments["DevOps"]["TerraformApplyAccess"][1])
	require.Len(t, result.AccountAssignments["DevOps"]["AdministratorAccess"], 1)
	assert.Equal(t, "core-root", result.AccountAssignments["DevOps"]["AdministratorAccess"][0])
	require.Len(t, result.AccountAssignments["Developers"]["TerraformPlanAccess"], 1)
	assert.Equal(t, "core-root", result.AccountAssignments["Developers"]["TerraformPlanAccess"][0])
}

func TestParseSSOConfig_NoAssignments(t *testing.T) {
	t.Parallel()

	data := []byte(`vars:
  start_url: "https://example.awsapps.com/start"
`)

	result, err := parseSSOConfig(data)
	require.NoError(t, err)
	assert.Equal(t, "https://example.awsapps.com/start", result.StartURL)
	assert.Empty(t, result.AccountAssignments)
}

func TestParseSSOConfig_EmptyData(t *testing.T) {
	t.Parallel()

	_, err := parseSSOConfig([]byte{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "empty")
}

func TestParseSSOConfig_MissingVars(t *testing.T) {
	t.Parallel()

	data := []byte(`settings:
  foo: bar
`)

	_, err := parseSSOConfig(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing 'vars'")
}

func TestParseSSOConfig_MalformedYAML(t *testing.T) {
	t.Parallel()

	data := []byte(`{invalid yaml:::`)

	_, err := parseSSOConfig(data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "parsing SSO config YAML")
}

func TestDiscoverAccountMap_Found(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)
	mockPrompter := mocks.NewMockPrompter(ctrl)

	base := filepath.Join("stacks")
	expectedPath := filepath.Join(base, "catalog", "account-map.yaml")
	accountData := []byte(`vars:
  full_account_map:
    dev: "123456789012"
    prod: "210987654321"
`)

	// First path does not exist, second path exists.
	mockFS.EXPECT().Exists(filepath.Join(base, "mixins", "account-map.yaml")).Return(false)
	mockFS.EXPECT().Exists(expectedPath).Return(true)
	mockFS.EXPECT().Exists(filepath.Join(base, "catalog", "account-map", "account-map.yaml")).Return(false)
	mockFS.EXPECT().ReadFile(expectedPath).Return(accountData, nil)

	result, err := discoverAccountMap(base, mockFS, mockPrompter)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "123456789012", result["dev"])
	assert.Equal(t, "210987654321", result["prod"])
}

func TestDiscoverAccountMap_NotFound_ReturnsEmptyMap(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)
	mockPrompter := mocks.NewMockPrompter(ctrl)

	base := filepath.Join("stacks")

	// No paths exist — returns empty map gracefully.
	mockFS.EXPECT().Exists(gomock.Any()).Return(false).Times(3)

	result, err := discoverAccountMap(base, mockFS, mockPrompter)
	require.NoError(t, err)
	assert.Empty(t, result)
}

func TestDiscoverSSOConfig_Found(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)
	mockPrompter := mocks.NewMockPrompter(ctrl)

	base := filepath.Join("stacks")
	expectedPath := filepath.Join(base, "catalog", "aws-sso.yaml")
	ssoData := []byte(`vars:
  start_url: "https://myorg.awsapps.com/start"
  region: "eu-west-1"
  account_assignments:
    Engineers:
      ReadOnlyAccess:
        - dev
`)

	// First path exists.
	mockFS.EXPECT().Exists(expectedPath).Return(true)
	mockFS.EXPECT().Exists(filepath.Join(base, "catalog", "aws-sso", "aws-sso.yaml")).Return(false)
	mockFS.EXPECT().ReadFile(expectedPath).Return(ssoData, nil)

	// Provider name prompt.
	mockPrompter.EXPECT().Input("Enter SSO provider name", "sso").Return("sso", nil)

	result, err := discoverSSOConfig(base, nil, mockFS, mockPrompter)
	require.NoError(t, err)
	assert.Equal(t, "https://myorg.awsapps.com/start", result.StartURL)
	assert.Equal(t, "eu-west-1", result.Region)
	assert.Equal(t, "sso", result.ProviderName)
	require.Len(t, result.AccountAssignments, 1)
	require.Len(t, result.AccountAssignments["Engineers"]["ReadOnlyAccess"], 1)
	assert.Equal(t, "dev", result.AccountAssignments["Engineers"]["ReadOnlyAccess"][0])
}

func TestDiscoverSSOConfig_MissingURLAndRegion_PromptsUser(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)
	mockPrompter := mocks.NewMockPrompter(ctrl)

	base := filepath.Join("stacks")
	expectedPath := filepath.Join(base, "catalog", "aws-sso.yaml")
	ssoData := []byte(`vars:
  account_assignments:
    Team:
      ViewAccess:
        - staging
`)

	mockFS.EXPECT().Exists(expectedPath).Return(true)
	mockFS.EXPECT().Exists(filepath.Join(base, "catalog", "aws-sso", "aws-sso.yaml")).Return(false)
	mockFS.EXPECT().ReadFile(expectedPath).Return(ssoData, nil)

	// Prompts for missing start_url and region.
	mockPrompter.EXPECT().Input("Enter your AWS SSO start URL", "").Return("https://prompted.awsapps.com/start", nil)
	mockPrompter.EXPECT().Input("Enter your AWS SSO region", "us-east-1").Return("us-east-1", nil)
	mockPrompter.EXPECT().Input("Enter SSO provider name", "sso").Return("my-sso", nil)

	result, err := discoverSSOConfig(base, nil, mockFS, mockPrompter)
	require.NoError(t, err)
	assert.Equal(t, "https://prompted.awsapps.com/start", result.StartURL)
	assert.Equal(t, "us-east-1", result.Region)
	assert.Equal(t, "my-sso", result.ProviderName)
}

func TestDiscoverSSOConfig_MultipleFound_PromptsSelect(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockFS := mocks.NewMockFileSystem(ctrl)
	mockPrompter := mocks.NewMockPrompter(ctrl)

	base := filepath.Join("stacks")
	path1 := filepath.Join(base, "catalog", "aws-sso.yaml")
	path2 := filepath.Join(base, "catalog", "aws-sso", "aws-sso.yaml")

	// Both paths exist.
	mockFS.EXPECT().Exists(path1).Return(true)
	mockFS.EXPECT().Exists(path2).Return(true)

	// User selects the second path.
	mockPrompter.EXPECT().Select("Multiple aws-sso.yaml files found. Select one", []string{path1, path2}).Return(path2, nil)

	ssoData := []byte(`vars:
  start_url: "https://selected.awsapps.com/start"
  region: "ap-southeast-1"
  account_assignments: {}
`)
	mockFS.EXPECT().ReadFile(path2).Return(ssoData, nil)
	mockPrompter.EXPECT().Input("Enter SSO provider name", "sso").Return("sso", nil)

	result, err := discoverSSOConfig(base, nil, mockFS, mockPrompter)
	require.NoError(t, err)
	assert.Equal(t, "https://selected.awsapps.com/start", result.StartURL)
	assert.Equal(t, "ap-southeast-1", result.Region)
}
