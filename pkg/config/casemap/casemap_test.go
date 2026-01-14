package casemap

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractFromYAML(t *testing.T) {
	yamlContent := `
env:
  GITHUB_TOKEN: token123
  AWS_REGION: us-east-1
  myCustomVar: custom_value
  TF_VAR_mixedCase: mixed
auth:
  identities:
    SuperAdmin:
      kind: aws/user
`
	caseMaps, err := ExtractFromYAML([]byte(yamlContent), []string{"env", "auth.identities"})
	require.NoError(t, err)

	// Test env case map - UPPERCASE.
	envMap := caseMaps.Get("env")
	assert.Equal(t, "GITHUB_TOKEN", envMap["github_token"])
	assert.Equal(t, "AWS_REGION", envMap["aws_region"])

	// Test env case map - camelCase.
	assert.Equal(t, "myCustomVar", envMap["mycustomvar"])
	assert.Equal(t, "TF_VAR_mixedCase", envMap["tf_var_mixedcase"])

	// Test auth.identities case map - PascalCase.
	identityMap := caseMaps.Get("auth.identities")
	assert.Equal(t, "SuperAdmin", identityMap["superadmin"])
}

func TestExtractFromYAML_MissingPath(t *testing.T) {
	yamlContent := `
env:
  GITHUB_TOKEN: token123
`
	caseMaps, err := ExtractFromYAML([]byte(yamlContent), []string{"env", "nonexistent.path"})
	require.NoError(t, err)

	// env should exist.
	envMap := caseMaps.Get("env")
	assert.NotNil(t, envMap)
	assert.Equal(t, "GITHUB_TOKEN", envMap["github_token"])

	// nonexistent.path should be nil.
	nonexistent := caseMaps.Get("nonexistent.path")
	assert.Nil(t, nonexistent)
}

func TestExtractFromYAML_InvalidYAML(t *testing.T) {
	invalidYAML := []byte(`{invalid yaml content`)
	_, err := ExtractFromYAML(invalidYAML, []string{"env"})
	assert.Error(t, err)
}

func TestApplyCase(t *testing.T) {
	caseMaps := New()
	caseMaps.Set("env", CaseMap{
		"github_token":     "GITHUB_TOKEN",
		"mycustomvar":      "myCustomVar",
		"tf_var_mixedcase": "TF_VAR_mixedCase",
	})

	lowercased := map[string]string{
		"github_token":     "secret",
		"mycustomvar":      "custom_value",
		"tf_var_mixedcase": "mixed",
	}
	result := caseMaps.ApplyCase("env", lowercased)

	// UPPERCASE preserved.
	assert.Equal(t, "secret", result["GITHUB_TOKEN"])
	assert.NotContains(t, result, "github_token")

	// camelCase preserved.
	assert.Equal(t, "custom_value", result["myCustomVar"])
	assert.NotContains(t, result, "mycustomvar")

	// Mixed case preserved.
	assert.Equal(t, "mixed", result["TF_VAR_mixedCase"])
	assert.NotContains(t, result, "tf_var_mixedcase")
}

func TestApplyCase_NilCaseMaps(t *testing.T) {
	var caseMaps *CaseMaps
	lowercased := map[string]string{"github_token": "secret"}

	result := caseMaps.ApplyCase("env", lowercased)

	// Should return original map unchanged.
	assert.Equal(t, lowercased, result)
}

func TestApplyCase_MissingPath(t *testing.T) {
	caseMaps := New()
	caseMaps.Set("env", CaseMap{"github_token": "GITHUB_TOKEN"})

	lowercased := map[string]string{"some_key": "value"}
	result := caseMaps.ApplyCase("nonexistent", lowercased)

	// Should return original map unchanged.
	assert.Equal(t, lowercased, result)
}

func TestApplyCase_MixedCaseStyles(t *testing.T) {
	// Table-driven test for various case styles.
	tests := []struct {
		name         string
		originalKey  string
		lowercaseKey string
		value        string
	}{
		{"SCREAMING_SNAKE", "GITHUB_TOKEN", "github_token", "token123"},
		{"camelCase", "myVariable", "myvariable", "value1"},
		{"PascalCase", "MyVariable", "myvariable", "value2"},
		{"snake_case", "my_variable", "my_variable", "value3"},
		{"MixedCase", "TF_VAR_myVar", "tf_var_myvar", "value4"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caseMaps := New()
			caseMaps.Set("env", CaseMap{tt.lowercaseKey: tt.originalKey})

			lowercased := map[string]string{tt.lowercaseKey: tt.value}
			result := caseMaps.ApplyCase("env", lowercased)

			assert.Equal(t, tt.value, result[tt.originalKey])
			if tt.originalKey != tt.lowercaseKey {
				assert.NotContains(t, result, tt.lowercaseKey)
			}
		})
	}
}

func TestGet_NilCaseMaps(t *testing.T) {
	var caseMaps *CaseMaps
	result := caseMaps.Get("env")
	assert.Nil(t, result)
}

func TestNavigateToPath(t *testing.T) {
	m := map[string]interface{}{
		"auth": map[string]interface{}{
			"identities": map[string]interface{}{
				"SuperAdmin": map[string]interface{}{
					"kind": "aws/user",
				},
			},
		},
		"env": map[string]interface{}{
			"GITHUB_TOKEN": "token",
		},
	}

	// Test single-level path.
	env := navigateToPath(m, "env")
	assert.NotNil(t, env)
	assert.Contains(t, env, "GITHUB_TOKEN")

	// Test nested path.
	identities := navigateToPath(m, "auth.identities")
	assert.NotNil(t, identities)
	assert.Contains(t, identities, "SuperAdmin")

	// Test nonexistent path.
	nonexistent := navigateToPath(m, "nonexistent.path")
	assert.Nil(t, nonexistent)

	// Test partially valid path.
	partial := navigateToPath(m, "auth.nonexistent")
	assert.Nil(t, partial)
}
