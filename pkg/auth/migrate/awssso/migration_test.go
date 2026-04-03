package awssso

import (
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

func TestExtractAccountMap_FullAccountMap(t *testing.T) {
	t.Parallel()

	componentSection := map[string]any{
		"vars": map[string]any{
			"full_account_map": map[string]any{
				"core-root":   "111111111111",
				"core-audit":  "222222222222",
				"dev-sandbox": "333333333333",
			},
		},
	}

	result, err := extractAccountMap(componentSection)
	require.NoError(t, err)
	require.Len(t, result, 3)
	assert.Equal(t, "111111111111", result["core-root"])
	assert.Equal(t, "222222222222", result["core-audit"])
	assert.Equal(t, "333333333333", result["dev-sandbox"])
}

func TestExtractAccountMap_AccountMapFallback(t *testing.T) {
	t.Parallel()

	componentSection := map[string]any{
		"vars": map[string]any{
			"account_map": map[string]any{
				"prod":    "444444444444",
				"staging": "555555555555",
			},
		},
	}

	result, err := extractAccountMap(componentSection)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, "444444444444", result["prod"])
	assert.Equal(t, "555555555555", result["staging"])
}

func TestExtractAccountMap_FullAccountMapTakesPrecedence(t *testing.T) {
	t.Parallel()

	componentSection := map[string]any{
		"vars": map[string]any{
			"full_account_map": map[string]any{
				"primary": "111111111111",
			},
			"account_map": map[string]any{
				"fallback": "999999999999",
			},
		},
	}

	result, err := extractAccountMap(componentSection)
	require.NoError(t, err)
	require.Len(t, result, 1)
	assert.Equal(t, "111111111111", result["primary"])
}

func TestExtractAccountMap_NoVars(t *testing.T) {
	t.Parallel()

	componentSection := map[string]any{
		"settings": map[string]any{},
	}

	_, err := extractAccountMap(componentSection)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no vars")
}

func TestExtractAccountMap_MissingKeys(t *testing.T) {
	t.Parallel()

	componentSection := map[string]any{
		"vars": map[string]any{
			"other_key": "value",
		},
	}

	_, err := extractAccountMap(componentSection)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing")
}

func TestExtractSSOFromComponent(t *testing.T) {
	t.Parallel()

	componentSection := map[string]any{
		"vars": map[string]any{
			"start_url": "https://example.awsapps.com/start",
			"region":    "us-west-2",
			"account_assignments": map[string]any{
				"DevOps": map[string]any{
					"TerraformApplyAccess": []any{"core-root", "core-audit"},
					"AdministratorAccess":  []any{"core-root"},
				},
				"Developers": map[string]any{
					"TerraformPlanAccess": []any{"core-root"},
				},
			},
		},
	}

	ssoCfg := &migrate.SSOConfig{
		AccountAssignments: make(map[string]map[string][]string),
	}

	extractSSOFromComponent(componentSection, ssoCfg)

	assert.Equal(t, "https://example.awsapps.com/start", ssoCfg.StartURL)
	assert.Equal(t, "us-west-2", ssoCfg.Region)
	require.Len(t, ssoCfg.AccountAssignments, 2)
	require.Len(t, ssoCfg.AccountAssignments["DevOps"]["TerraformApplyAccess"], 2)
	assert.Equal(t, "core-root", ssoCfg.AccountAssignments["DevOps"]["TerraformApplyAccess"][0])
	assert.Equal(t, "core-audit", ssoCfg.AccountAssignments["DevOps"]["TerraformApplyAccess"][1])
	require.Len(t, ssoCfg.AccountAssignments["DevOps"]["AdministratorAccess"], 1)
	require.Len(t, ssoCfg.AccountAssignments["Developers"]["TerraformPlanAccess"], 1)
}

func TestExtractSSOFromComponent_NoAssignments(t *testing.T) {
	t.Parallel()

	componentSection := map[string]any{
		"vars": map[string]any{
			"start_url": "https://example.awsapps.com/start",
		},
	}

	ssoCfg := &migrate.SSOConfig{
		AccountAssignments: make(map[string]map[string][]string),
	}

	extractSSOFromComponent(componentSection, ssoCfg)

	assert.Equal(t, "https://example.awsapps.com/start", ssoCfg.StartURL)
	assert.Empty(t, ssoCfg.AccountAssignments)
}

func TestExtractSSOFromComponent_PreservesExistingValues(t *testing.T) {
	t.Parallel()

	componentSection := map[string]any{
		"vars": map[string]any{
			"start_url": "https://new.awsapps.com/start",
			"region":    "us-east-1",
		},
	}

	// SSO config already has values from atmos.yaml auth config.
	ssoCfg := &migrate.SSOConfig{
		StartURL:           "https://existing.awsapps.com/start",
		Region:             "eu-west-1",
		AccountAssignments: make(map[string]map[string][]string),
	}

	extractSSOFromComponent(componentSection, ssoCfg)

	// Existing values should NOT be overwritten.
	assert.Equal(t, "https://existing.awsapps.com/start", ssoCfg.StartURL)
	assert.Equal(t, "eu-west-1", ssoCfg.Region)
}

func TestExtractSSOFromComponent_NoVars(t *testing.T) {
	t.Parallel()

	componentSection := map[string]any{
		"settings": map[string]any{},
	}

	ssoCfg := &migrate.SSOConfig{
		AccountAssignments: make(map[string]map[string][]string),
	}

	// Should not panic or error — just log and return.
	extractSSOFromComponent(componentSection, ssoCfg)
	assert.Empty(t, ssoCfg.StartURL)
}

func TestParseAccountAssignments(t *testing.T) {
	t.Parallel()

	assignData := map[string]interface{}{
		"DevOps": map[string]interface{}{
			"TerraformApplyAccess": []interface{}{"core-root", "dev"},
		},
		"Readers": map[string]interface{}{
			"TerraformPlanAccess": []interface{}{"prod"},
		},
	}

	result, err := parseAccountAssignments(assignData)
	require.NoError(t, err)
	require.Len(t, result, 2)
	assert.Equal(t, []string{"core-root", "dev"}, result["DevOps"]["TerraformApplyAccess"])
	assert.Equal(t, []string{"prod"}, result["Readers"]["TerraformPlanAccess"])
}

func TestParseAccountAssignments_NotAMap(t *testing.T) {
	t.Parallel()

	_, err := parseAccountAssignments("not a map")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a map")
}
