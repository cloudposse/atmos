package secret

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/secrets"
)

// writeSecretDeclaringProject writes a self-contained Atmos project whose single terraform
// component (vpc in stack dev) declares one store-backed secret. The enumerator runs the real
// config + describe-stacks pipeline in-process with `!secret` resolution skipped, so the
// instance is discovered without any cloud credentials or a real store backend.
func writeSecretDeclaringProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	write := func(rel, content string) {
		full := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
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
      secrets:
        vars:
          API_KEY:
            store: app-secrets
`)
	write("components/terraform/vpc/main.tf", "# vpc component.\n")
	return dir
}

// writeCustomDelimiterSecretProject writes two components with the same raw global declaration.
// Its reference uses custom Go-template delimiters and resolves to a different backend coordinate
// for each component.
func writeCustomDelimiterSecretProject(t *testing.T) string {
	t.Helper()

	dir := t.TempDir()
	write := func(rel, content string) {
		full := filepath.Join(dir, rel)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o644))
	}

	write("atmos.yaml", `base_path: "."
components:
  terraform:
    base_path: "components/terraform"
stacks:
  base_path: "stacks"
  included_paths:
    - "deploy/**/*"
  name_pattern: "{stage}"
templates:
  settings:
    enabled: true
    delimiters:
      - "[["
      - "]]"
`)
	write("stacks/deploy/dev.yaml", `vars:
  stage: dev
components:
  terraform:
    example-service-a:
      secrets:
        vars:
          SHARED_TOKEN:
            store: example-secrets
            scope: global
            reference: "op://shared/[[ .atmos_component ]]/password"
    example-service-b:
      secrets:
        vars:
          SHARED_TOKEN:
            store: example-secrets
            scope: global
            reference: "op://shared/[[ .atmos_component ]]/password"
`)
	write("components/terraform/example-service-a/main.tf", "# Example component A.\n")
	write("components/terraform/example-service-b/main.tf", "# Example component B.\n")
	return dir
}

// TestEnumerateSecretScopes_Success drives the real stack-resolution path: it discovers the single
// secret-declaring instance and returns a resolved config, with the declaration surviving the merge.
func TestEnumerateSecretScopes_Success(t *testing.T) {
	t.Chdir(writeSecretDeclaringProject(t))

	entries, atmosConfig, err := enumerateSecretScopes(secretScope{Stack: "dev"})
	require.NoError(t, err)
	require.NotNil(t, atmosConfig)
	require.Len(t, entries, 1)
	assert.Equal(t, "dev", entries[0].Stack)
	assert.Equal(t, "vpc", entries[0].Component)
	assert.Contains(t, secrets.ExtractDeclarations(entries[0].Section), "API_KEY",
		"the resolved section must still carry the declared secret")
}

// TestEnumerateSecretScopes_ComponentFilter exercises the --component narrowing branch (the
// components slice passed to describe-stacks) end-to-end.
func TestEnumerateSecretScopes_ComponentFilter(t *testing.T) {
	t.Chdir(writeSecretDeclaringProject(t))

	entries, _, err := enumerateSecretScopes(secretScope{Stack: "dev", Component: "vpc"})
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "vpc", entries[0].Component)
}

// TestFindGlobalSetContext_CustomDelimiters proves component-less global writes validate the
// resolved declaration in every component context rather than inspecting default template syntax.
func TestFindGlobalSetContext_CustomDelimiters(t *testing.T) {
	t.Chdir(writeCustomDelimiterSecretProject(t))

	entries, _, err := enumerateSecretScopes(secretScope{Stack: "dev"})
	require.NoError(t, err)
	require.Len(t, entries, 2)
	references := make(map[string]string, len(entries))
	for _, entry := range entries {
		references[entry.Component] = secrets.ExtractDeclarations(entry.Section)["SHARED_TOKEN"].Reference
	}
	require.Equal(t, map[string]string{
		"example-service-a": "op://shared/example-service-a/password",
		"example-service-b": "op://shared/example-service-b/password",
	}, references)

	_, _, err = findGlobalSetContext(secretScope{Stack: "dev"}, "SHARED_TOKEN")
	require.ErrorIs(t, err, errUtils.ErrRequiredFlagNotProvided)
}

// TestEnumerateSecretScopes_InitConfigError covers the early error branch: an empty dir has no
// Atmos config, so InitCliConfig fails before any stack processing.
func TestEnumerateSecretScopes_InitConfigError(t *testing.T) {
	t.Chdir(t.TempDir())

	_, _, err := enumerateSecretScopes(secretScope{Stack: "dev"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)
}
