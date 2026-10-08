package secret

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
)

const (
	selectorEnvVar   = "ATMOS_TEST_SECRET_STORE"
	selectorPassword = "ATMOS_TEST_SECRET_KEYRING_PASSWORD"
	ghostSelector    = "!aws.cloudformation.output ghost dev SecretStoreName"
)

// writeProjectWithSelectors writes a project whose component declares one secret per backend shape: a
// literal store, a store chosen by a selector that evaluates (`!env`), and a store chosen by a
// CloudFormation output selector whose producer does not exist. The store is a file-backed keychain,
// so values persist across the independently loaded configurations each command builds.
func writeProjectWithSelectors(t *testing.T) string {
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
stores:
  kc:
    kind: keychain
    options:
      backend: file
      file_dir: "`+filepath.ToSlash(filepath.Join(dir, "keyring"))+`"
      password_env: `+selectorPassword+`
`)
	write("stacks/deploy/dev.yaml", `vars:
  stage: dev
components:
  terraform:
    app:
      vars:
        from_selected: !secret SELECTED_KEY
      secrets:
        vars:
          SELECTED_KEY:
            store: !env `+selectorEnvVar+`
          LITERAL_KEY:
            store: kc
          GHOST_KEY:
            store: `+ghostSelector+`
    ghostly:
      vars:
        from_ghost: !secret GHOST_KEY
      secrets:
        vars:
          GHOST_KEY:
            store: `+ghostSelector+`
`)
	write("components/terraform/app/main.tf", "# app.\n")
	write("components/terraform/ghostly/main.tf", "# ghostly.\n")
	t.Setenv(selectorEnvVar, "kc")
	t.Setenv(selectorPassword, "test-password")
	return dir
}

// describeVarsFrom describes a component with evaluation narrowed to one vars field, exactly like
// `atmos describe component --query .vars.<field>` does. The component's own declarations are then
// not evaluated by the describe pass, so a `!secret` must resolve its declaration's selector itself.
func describeVarsFrom(t *testing.T, component, field string) (map[string]any, error) {
	t.Helper()
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{ComponentFromArg: component, Stack: "dev"}, true)
	require.NoError(t, err)
	return e.ExecuteDescribeComponent(&e.ExecuteDescribeComponentParams{
		AtmosConfig:          &atmosConfig,
		Component:            component,
		Stack:                "dev",
		ComponentType:        cfg.TerraformComponentType,
		ProcessTemplates:     true,
		ProcessYamlFunctions: true,
		ResolveSecrets:       true,
		ErrorOptions:         e.DescribeStacksErrorOptions{EvaluationPaths: [][]string{{"vars", field}}},
	})
}

// TestSecretFunction_ConsumesSelectedStore is the `!secret` half of the selector support: describing
// a component whose `!secret` points at a declaration with a selector-chosen store resolves the
// selector the same way the CLI does, instead of failing with "referenced store is not configured".
func TestSecretFunction_ConsumesSelectedStore(t *testing.T) {
	t.Chdir(writeProjectWithSelectors(t))

	svc, err := loadService(secretScope{Stack: "dev", Component: "app"})
	require.NoError(t, err)
	require.NoError(t, svc.Set("SELECTED_KEY", "hello"))

	section, err := describeVarsFrom(t, "app", "from_selected")
	require.NoError(t, err)
	vars, ok := section["vars"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "hello", vars["from_selected"])
}

// TestSecretFunction_UnresolvableSelectorNamesDeclaration proves the `!secret` path reports an
// unresolvable selector against its declaration rather than treating the selector text as a store.
func TestSecretFunction_UnresolvableSelectorNamesDeclaration(t *testing.T) {
	t.Chdir(writeProjectWithSelectors(t))

	_, err := describeVarsFrom(t, "ghostly", "from_ghost")
	require.ErrorIs(t, err, secrets.ErrSelectorUnresolved)
	for _, want := range []string{`"GHOST_KEY"`, `"ghostly"`, `"dev"`, ghostSelector} {
		assert.Contains(t, err.Error(), want)
	}
	assert.NotContains(t, err.Error(), errUtils.ErrStoreNotFound.Error())
}

// TestLoadService_UnresolvableSelectorOnlyFailsItsDeclaration is the CLI half of the lazy-selector
// contract: one declaration whose selector cannot be resolved (here a CloudFormation producer that
// was never deployed or was deleted) neither blocks loading the component nor the declarations that
// use literal stores; only the unresolvable declaration fails, naming itself.
func TestLoadService_UnresolvableSelectorOnlyFailsItsDeclaration(t *testing.T) {
	t.Chdir(writeProjectWithSelectors(t))

	svc, err := loadService(secretScope{Stack: "dev", Component: "app"})
	require.NoError(t, err, "an unresolvable sibling selector must not block loading the component")

	// Literal declaration: full lifecycle works while the sibling selector is unresolvable.
	_, err = svc.Get("LITERAL_KEY", secrets.ResolveOptions{})
	require.ErrorIs(t, err, secrets.ErrSecretMissing)
	require.NotErrorIs(t, err, secrets.ErrSelectorUnresolved)
	require.NoError(t, svc.Set("LITERAL_KEY", "literal-value"))
	got, err := svc.Get("LITERAL_KEY", secrets.ResolveOptions{})
	require.NoError(t, err)
	assert.Equal(t, "literal-value", got)
	require.NoError(t, svc.Delete("LITERAL_KEY"), "a literal secret stays deletable while a sibling selector is unresolvable")

	// The unresolvable declaration fails by name, with the producer hint when the stack is missing.
	_, err = svc.Get("GHOST_KEY", secrets.ResolveOptions{})
	require.ErrorIs(t, err, secrets.ErrSelectorUnresolved)
	for _, want := range []string{`"GHOST_KEY"`, `"app"`, `"dev"`, ghostSelector, "`ghost`"} {
		assert.Contains(t, err.Error(), want, "the error names the declaration and the real missing producer")
	}

	// Credential-free listing reports the selector as unresolved with a reason, not a bare error.
	listed, err := loadServiceForList(secretScope{Stack: "dev", Component: "app"}, false)
	require.NoError(t, err)
	for _, st := range listed.Status(false) {
		if st.Declaration.Name != "GHOST_KEY" {
			continue
		}
		require.NoError(t, st.Err)
		assert.True(t, st.Unresolved)
		assert.Equal(t, "unresolved", statusLabel(&st))
		assert.Contains(t, st.Reason, ghostSelector)
	}

	// DeleteAll removes what it can and reports the one it could not.
	require.NoError(t, svc.Set("LITERAL_KEY", "again"))
	deleted, err := svc.DeleteAll()
	require.ErrorIs(t, err, secrets.ErrSelectorUnresolved)
	assert.Equal(t, 2, deleted, "the literal and the env-selected declarations are deleted")
}

func TestValueAtPath(t *testing.T) {
	section := map[string]any{"secrets": map[string]any{"vars": map[string]any{"K": map[string]any{"store": "kc"}}}}

	got, err := valueAtPath(section, []string{"secrets", "vars", "K", "store"})
	require.NoError(t, err)
	assert.Equal(t, "kc", got)

	for _, path := range [][]string{{"secrets", "vars", "MISSING", "store"}, {"secrets", "vars", "K", "store", "deeper"}} {
		_, err := valueAtPath(section, path)
		require.ErrorIs(t, err, errUtils.ErrInvalidConfig, "%v", path)
	}
}
