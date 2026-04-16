package exec

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestGenerateProviderOverridesForAliases is an end-to-end integration test that
// reproduces cloudposse/atmos#2208: provider keys using the `<base>.<alias>`
// shorthand (e.g. `aws.use1`) must be grouped into a Terraform-JSON array under
// the base key, not emitted as separate top-level keys.
//
// The test loads a real stack config via ProcessStacks, invokes the same
// generateProviderOverrides() function that ExecuteTerraform calls in production
// (terraform.go:394), reads the generated providers_override.tf.json from disk,
// and asserts its JSON structure. This exercises the full path:
//
//	stack YAML -> stack processor -> ComponentProvidersSection ->
//	  generateComponentProviderOverrides -> ProcessProviderAliases ->
//	    WriteToFileAsJSON -> providers_override.tf.json
//
// Two components cover both forms of the shorthand:
//   - `eip-explicit-alias`: the user writes `alias: use1` inside the block.
//   - `eip-derived-alias`:  the user omits `alias:` and Atmos derives `use1`
//     from the key suffix.
func TestGenerateProviderOverridesForAliases(t *testing.T) {
	// Unset env vars that would otherwise point Atmos at a different config.
	require.NoError(t, os.Unsetenv("ATMOS_CLI_CONFIG_PATH"))
	require.NoError(t, os.Unsetenv("ATMOS_BASE_PATH"))

	fixture := "../../tests/fixtures/scenarios/atmos-providers-aliases"
	t.Chdir(fixture)

	tests := []struct {
		name      string
		component string
	}{
		{
			name:      "explicit alias in block",
			component: "eip-explicit-alias",
		},
		{
			// Regression + enhancement: alias is derived from the key suffix
			// when the block does not define it explicitly.
			name:      "alias auto-derived from key",
			component: "eip-derived-alias",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info := schema.ConfigAndStacksInfo{
				ComponentFromArg: tt.component,
				Stack:            "nonprod",
				ComponentType:    cfg.TerraformComponentType,
			}
			atmosConfig, err := cfg.InitCliConfig(info, true)
			require.NoError(t, err)

			result, err := ProcessStacks(&atmosConfig, info, true, false, false, nil, nil)
			require.NoError(t, err, "ProcessStacks must load the fixture stack")
			require.NotEmpty(t, result.ComponentProvidersSection, "ComponentProvidersSection should be populated from the stack")

			// Write the provider override file to a test-scoped tempdir so the
			// real filesystem is exercised but we leave no residue behind.
			tempDir := t.TempDir()
			require.NoError(t, generateProviderOverrides(&atmosConfig, &result, tempDir))

			providersFile := filepath.Join(tempDir, "providers_override.tf.json")
			raw, err := os.ReadFile(providersFile)
			require.NoError(t, err, "providers_override.tf.json must be generated")

			var parsed map[string]any
			require.NoError(t, json.Unmarshal(raw, &parsed), "generated file must be valid JSON")

			// Top-level shape: {"provider": {...}}.
			providerSection, ok := parsed["provider"].(map[string]any)
			require.True(t, ok, "top-level `provider` must be an object, got %T", parsed["provider"])

			// Regression signal: `aws.use1` MUST NOT appear as a top-level provider key.
			_, hasDotKey := providerSection["aws.use1"]
			assert.False(t, hasDotKey, "generated JSON must not contain `aws.use1` as a top-level provider key; got: %s", string(raw))

			// The fix: `provider.aws` MUST be a JSON array of length 2.
			awsEntries, ok := providerSection["aws"].([]any)
			require.True(t, ok, "`provider.aws` must be a JSON array, got %T in: %s", providerSection["aws"], string(raw))
			require.Len(t, awsEntries, 2, "`provider.aws` must contain 2 entries (base + 1 alias)")

			// First entry is the bare base provider; must not carry an `alias`.
			base, ok := awsEntries[0].(map[string]any)
			require.True(t, ok, "first entry must be an object")
			assert.Equal(t, "us-east-2", base["region"])
			_, baseHasAlias := base["alias"]
			assert.False(t, baseHasAlias, "base provider entry must not have an `alias` key")

			// Second entry is the alias; must carry `alias: use1` regardless of
			// whether it was explicit or auto-derived.
			aliased, ok := awsEntries[1].(map[string]any)
			require.True(t, ok, "second entry must be an object")
			assert.Equal(t, "us-east-1", aliased["region"])
			assert.Equal(t, "use1", aliased["alias"], "alias must be set (explicit or auto-derived)")
		})
	}
}
