package exec

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// TestHelmfileEnvSecretsStayOffVarfile locks in the security contract for Helmfile secrets:
// values placed in a component's `env` section are passed only through the subprocess environment
// (via ComponentEnvList) and are NEVER written to the on-disk varfile. Only the `vars` section is
// serialized to the varfile (see internal/exec/helmfile.go where ComponentVarsSection is written).
// Unlike Terraform, Helmfile performs no off-disk partitioning of secret vars, so the `env` section
// is the only secure place for a secret.
func TestHelmfileEnvSecretsStayOffVarfile(t *testing.T) {
	const secretValue = "s3cr3t-value"

	info := schema.ConfigAndStacksInfo{
		ComponentVarsSection: map[string]any{
			"chart_version": "1.2.3",
		},
		ComponentEnvSection: map[string]any{
			"DB_PASSWORD": secretValue,
		},
		ComponentEnvList: []string{},
	}

	// Write the varfile exactly as the Helmfile execution path does: only ComponentVarsSection,
	// as YAML, at 0o644.
	varFilePath := filepath.Join(t.TempDir(), "my-app.helmfile.vars.yaml")
	require.NoError(t, u.WriteToFileAsYAML(varFilePath, info.ComponentVarsSection, 0o644))

	// Convert the env section to the subprocess env list using the production helper.
	ConvertComponentEnvSectionToList(&info)

	// (a) The secret reaches the subprocess environment.
	assert.Contains(t, info.ComponentEnvList, "DB_PASSWORD="+secretValue,
		"secret from the env section must be injected into the subprocess environment")

	// (b) The secret never lands in the on-disk varfile.
	varFileBytes, err := os.ReadFile(varFilePath)
	require.NoError(t, err)
	varFileContent := string(varFileBytes)
	assert.NotContains(t, varFileContent, secretValue,
		"secret value must NOT be written to the Helmfile varfile on disk")
	assert.NotContains(t, varFileContent, "DB_PASSWORD",
		"env-section keys must NOT be written to the Helmfile varfile on disk")

	// (c) Non-secret vars are still written to the varfile.
	assert.True(t, strings.Contains(varFileContent, "chart_version"),
		"non-secret vars must be written to the Helmfile varfile")
}

// TestHelmfileEnvSecretResolvesViaSops drives the full stack-processing path for a Helmfile
// component whose `env` section references a SOPS-backed `!secret`. It proves end-to-end that the
// secret resolves in the env section (decrypted, not the raw tag or ciphertext) and is kept out of
// the vars section — which is what Helmfile writes to the on-disk varfile. The SOPS/age key is
// committed in the fixture and referenced inline via `age_key_file`, so no `sops` binary or
// SOPS_AGE_KEY_FILE env var is required.
func TestHelmfileEnvSecretResolvesViaSops(t *testing.T) {
	t.Chdir("../../tests/fixtures/scenarios/secrets-helmfile")
	t.Setenv("ATMOS_CLI_CONFIG_PATH", ".")
	t.Setenv("ATMOS_BASE_PATH", ".")

	res, err := ExecuteDescribeComponent(&ExecuteDescribeComponentParams{
		Component:            "redis",
		Stack:                "dev",
		ComponentType:        "helmfile",
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
	})
	require.NoError(t, err)

	// The secret resolves in the env section.
	envSection, ok := res["env"].(map[string]any)
	require.True(t, ok, "describe result must include an env section")
	resolved, _ := envSection["REDIS_URL"].(string)
	require.NotEmpty(t, resolved, "REDIS_URL must resolve to a value from the SOPS backend")
	assert.NotContains(t, resolved, "ENC[", "value must be decrypted, not SOPS ciphertext")
	assert.NotContains(t, resolved, "!secret", "value must be the resolved secret, not the raw !secret tag")

	// The secret must NOT appear in the vars section (vars is what Helmfile writes to disk).
	varsSection, _ := res["vars"].(map[string]any)
	_, hasInVars := varsSection["REDIS_URL"]
	assert.False(t, hasInVars, "secret declaration key must not leak into the vars section")
	for k, v := range varsSection {
		if s, ok := v.(string); ok {
			assert.NotEqual(t, resolved, s, "resolved secret value must not appear in vars (key %q)", k)
		}
	}
}
