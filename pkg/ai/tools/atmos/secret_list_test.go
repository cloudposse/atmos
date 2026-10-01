package atmos

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
)

// writeSecretListFixture writes a self-contained Atmos project with one stack ("dev") and two
// terraform components: "vpc" (no secrets) and "app" (declares one SOPS-backed secret, API_KEY,
// with no encrypted file yet — so its status is deterministically "missing" without any age key,
// network access, or decryption). Returns the project directory.
func writeSecretListFixture(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	write := func(rel, content string) {
		full := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
	}

	write("atmos.yaml", `base_path: "."
components:
  terraform:
    base_path: "components/terraform"
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  name_template: "{{.vars.stage}}"
`)
	write("stacks/deploy/dev.yaml", `vars:
  stage: dev
components:
  terraform:
    vpc:
      vars:
        name: myvpc
    app:
      vars:
        name: myapp
      secrets:
        vars:
          API_KEY:
            sops: myvault
            description: application API key
        providers:
          myvault: {}
`)
	write("components/terraform/vpc/main.tf", "# vpc component.\n")
	write("components/terraform/app/main.tf", "# app component.\n")
	return dir
}

func TestSecretListTool_Interface(t *testing.T) {
	tool := NewSecretListTool(&schema.AtmosConfiguration{})

	assert.Equal(t, "atmos_secret_list", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.False(t, tool.RequiresPermission())
	assert.False(t, tool.IsRestricted())

	params := tool.Parameters()
	require.Len(t, params, 2)
	assert.Equal(t, "stack", params[0].Name)
	assert.Equal(t, "component", params[1].Name)
	assert.False(t, params[0].Required)
	assert.False(t, params[1].Required)
}

func TestSecretListTool_NewSecretListTool(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	tool := NewSecretListTool(config)

	assert.NotNil(t, tool)
	assert.Equal(t, config, tool.atmosConfig)
}

func TestSecretListTool_Execute_NoSecretsDeclared(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "stacks"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "components", "terraform"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "atmos.yaml"), []byte(`
base_path: .
components:
  terraform:
    base_path: components/terraform
stacks:
  base_path: stacks
  included_paths:
    - "**/*.yaml"
  name_pattern: "{stage}"
`), 0o600))

	t.Chdir(dir)
	exec.ClearFindStacksMapCache()
	t.Cleanup(exec.ClearFindStacksMapCache)

	// A nil atmosConfig makes currentStackConfig fully (re)initialize from the current directory,
	// mirroring the live stack-refresh path an MCP server takes (see stack_config.go).
	tool := NewSecretListTool(nil)
	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "Declared Secrets (0)")

	rows, ok := result.Data["secrets"].([]secretStatusRow)
	require.True(t, ok)
	assert.Empty(t, rows)
}

func TestSecretListTool_Execute_ListsDeclaredSecret(t *testing.T) {
	dir := writeSecretListFixture(t)
	t.Chdir(dir)
	exec.ClearFindStacksMapCache()
	t.Cleanup(exec.ClearFindStacksMapCache)

	tool := NewSecretListTool(nil)
	result, err := tool.Execute(context.Background(), map[string]interface{}{})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	rows, ok := result.Data["secrets"].([]secretStatusRow)
	require.True(t, ok)
	require.Len(t, rows, 1)
	assert.Equal(t, "dev", rows[0].Stack)
	assert.Equal(t, "app", rows[0].Component)
	assert.Equal(t, "API_KEY", rows[0].Secret)
	assert.Equal(t, "instance", rows[0].Scope)
	assert.Equal(t, "sops:myvault", rows[0].Provider)
	// No encrypted file exists yet, so the credential-free local status check reports "missing".
	assert.Equal(t, "missing", rows[0].Status)
	assert.Contains(t, result.Output, "API_KEY")

	// Never leaks a decrypted value, only a status label.
	assert.NotContains(t, result.Output, "value")
}

func TestSecretListTool_Execute_ScopedToComponent(t *testing.T) {
	dir := writeSecretListFixture(t)
	t.Chdir(dir)
	exec.ClearFindStacksMapCache()
	t.Cleanup(exec.ClearFindStacksMapCache)

	tool := NewSecretListTool(nil)
	result, err := tool.Execute(context.Background(), map[string]interface{}{
		"stack":     "dev",
		"component": "vpc",
	})

	require.NoError(t, err)
	require.NotNil(t, result)
	assert.True(t, result.Success)

	rows, ok := result.Data["secrets"].([]secretStatusRow)
	require.True(t, ok)
	assert.Empty(t, rows, "vpc declares no secrets")
}

func TestCredentialFreeSkipTags(t *testing.T) {
	tags := credentialFreeSkipTags()
	assert.Contains(t, tags, "secret")
	assert.Contains(t, tags, "store")
	assert.Contains(t, tags, "store.get")
	assert.Contains(t, tags, "terraform.output")
	assert.Contains(t, tags, "terraform.state")
	for _, tag := range tags {
		assert.NotContains(t, tag, "!", "skip tags must have the leading ! trimmed")
	}
}

func TestCollectSecretScopeEntries(t *testing.T) {
	stacksMap := map[string]any{
		"dev": map[string]any{
			"components": map[string]any{
				"terraform": map[string]any{
					"app": map[string]any{
						"secrets": map[string]any{
							"vars": map[string]any{
								"API_KEY": map[string]any{"sops": "vault"},
							},
						},
					},
					"vpc": map[string]any{
						"vars": map[string]any{"name": "myvpc"},
					},
				},
			},
		},
	}

	entries := collectSecretScopeEntries(stacksMap, "")
	require.Len(t, entries, 1)
	assert.Equal(t, "dev", entries[0].Stack)
	assert.Equal(t, "app", entries[0].Component)

	filtered := collectSecretScopeEntries(stacksMap, "vpc")
	assert.Empty(t, filtered, "vpc declares no secrets")
}

func TestSecretStatusLabel(t *testing.T) {
	assert.Equal(t, "error", secretStatusLabel(&secrets.Status{Err: assert.AnError}))
	assert.Equal(t, "unknown", secretStatusLabel(&secrets.Status{Unknown: true}))
	assert.Equal(t, "initialized", secretStatusLabel(&secrets.Status{Initialized: true}))
	assert.Equal(t, "missing", secretStatusLabel(&secrets.Status{}))
}

func TestBuildSecretListResult_Empty(t *testing.T) {
	result := buildSecretListResult("dev", "", nil)

	assert.True(t, result.Success)
	assert.Contains(t, result.Output, "Declared Secrets (0)")
	assert.Equal(t, "dev", result.Data["stack"])
	assert.Equal(t, "", result.Data["component"])
}
