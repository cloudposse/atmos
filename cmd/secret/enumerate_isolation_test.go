package secret

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
)

// writeProjectWithDynamicDeclaration writes a project whose "dynamic" component carries a dynamic
// template in a declaration field (which disables evaluation narrowing for that component) and an
// unrelated atmos.Component reference to a component that does not exist, so evaluating that
// component fails. The "static" component is healthy. The dynamicBackend argument is the dynamic
// component's declaration backend line (for example `store: secrets/ssm` or `sops: sops-a`).
func writeProjectWithDynamicDeclaration(t *testing.T, dynamicBackend string) string {
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
templates:
  settings:
    enabled: true
`)
	write("stacks/deploy/dev.yaml", `vars:
  stage: dev
components:
  terraform:
    static:
      vars:
        name: static
      secrets:
        vars:
          STATIC_KEY:
            store: secrets/ssm
    dynamic:
      vars:
        name: dynamic
        other: '{{ (atmos.Component "nonexistent-zzz" "dev").outputs.id }}'
      secrets:
        vars:
          DYN_KEY:
            `+dynamicBackend+`
            description: '{{ printf "%v" .vars }}'
`)
	write("components/terraform/static/main.tf", "# static.\n")
	write("components/terraform/dynamic/main.tf", "# dynamic.\n")
	return dir
}

func entryComponents(entries []scopeEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Component)
	}
	return names
}

// TestEnumerate_DynamicTemplateIsAttributedAndIsolated reproduces a dynamic template in one
// component's declaration (which makes Atmos evaluate that whole component, including an unrelated
// failing atmos.Component call). The failure must name the component and stack, and must not hide
// the healthy component's declarations.
func TestEnumerate_DynamicTemplateIsAttributedAndIsolated(t *testing.T) {
	t.Chdir(writeProjectWithDynamicDeclaration(t, "store: secrets/ssm"))

	t.Run("stack-wide keeps healthy entries and names the broken component", func(t *testing.T) {
		entries, atmosConfig, err := enumerateSecretScopes(secretScope{Stack: "dev"})
		require.Error(t, err)
		require.ErrorIs(t, err, ErrSecretScopeEvaluation)
		assert.True(t, isPartialEnumeration(err))
		assert.NotNil(t, atmosConfig)
		assert.Contains(t, err.Error(), `component "dynamic" in stack "dev"`)
		assert.Contains(t, err.Error(), "nonexistent-zzz", "the underlying cause is preserved")
		assert.NotContains(t, err.Error(), `component "static"`)
		assert.Equal(t, []string{"static"}, entryComponents(entries))
	})

	t.Run("a requested healthy component is unaffected", func(t *testing.T) {
		entries, _, err := enumerateSecretScopes(secretScope{Stack: "dev", Component: "static"})
		require.NoError(t, err)
		assert.Equal(t, []string{"static"}, entryComponents(entries))
	})

	t.Run("a requested broken component is attributed", func(t *testing.T) {
		_, _, err := enumerateSecretScopes(secretScope{Stack: "dev", Component: "dynamic"})
		require.ErrorIs(t, err, ErrSecretScopeEvaluation)
		assert.False(t, isPartialEnumeration(err))
		assert.Contains(t, err.Error(), `component "dynamic" in stack "dev"`)
	})

	t.Run("collision check tolerates a broken component that declares no SOPS secret", func(t *testing.T) {
		require.NoError(t, checkStackSopsCollisions(secretScope{Stack: "dev"}))
	})

	t.Run("completion still offers the healthy components", func(t *testing.T) {
		names, _ := componentCompletionForStack("dev")(nil, nil, "")
		assert.Equal(t, []string{"static"}, names)
	})
}

// TestCheckStackSopsCollisions_FailsClosedOnBrokenSopsComponent proves an instance that cannot be
// evaluated and declares a SOPS secret blocks the collision guard, naming it: its file is unknown.
func TestCheckStackSopsCollisions_FailsClosedOnBrokenSopsComponent(t *testing.T) {
	t.Chdir(writeProjectWithDynamicDeclaration(t, "sops: sops-a"))

	err := checkStackSopsCollisions(secretScope{Stack: "dev"})
	require.ErrorIs(t, err, ErrSecretScopeEvaluation)
	assert.Contains(t, err.Error(), `component "dynamic" in stack "dev"`)
}

// TestRunSecretList_SkipsBrokenComponentsWithWarning proves a partial enumeration lists the healthy
// instances instead of failing the whole list.
func TestRunSecretList_SkipsBrokenComponentsWithWarning(t *testing.T) {
	setupIO(t)
	orig := enumerateScopesFn
	enumerateScopesFn = func(secretScope) ([]scopeEntry, *schema.AtmosConfiguration, error) {
		return []scopeEntry{{Stack: "dev", Component: "static", Section: sopsSection("secrets/dev.enc.yaml")}},
			&schema.AtmosConfiguration{},
			&enumerationError{failures: []error{attributeScopeFailure("dev", "dynamic", errors.New("boom"))}}
	}
	t.Cleanup(func() { enumerateScopesFn = orig })

	rows, err := enumeratedSecretRows(secretScope{Stack: "dev"}, false)
	require.NoError(t, err)
	require.NotEmpty(t, rows)
	for _, row := range rows {
		assert.Equal(t, "static", row["component"])
	}

	// A non-partial enumeration error still fails the list.
	enumerateScopesFn = func(secretScope) ([]scopeEntry, *schema.AtmosConfiguration, error) {
		return nil, nil, errors.New("hard failure")
	}
	_, err = enumeratedSecretRows(secretScope{Stack: "dev"}, false)
	require.Error(t, err)
}

// TestCheckStackSopsCollisions_SelectorBackedSops covers the stack-wide collision guard with SOPS
// declarations whose `sops:` is a selector: it must resolve them (so a collision is found) and must
// fail closed, naming the declaration, when a selector cannot be resolved.
func TestCheckStackSopsCollisions_SelectorBackedSops(t *testing.T) {
	const selector = "!aws.cloudformation.output producer dev SopsProviderName"
	providers := map[string]any{"sops-a": map[string]any{"kind": "sops/age", "spec": map[string]any{"file": "secrets/shared.enc.yaml"}}}
	entry := func(component, backend string) scopeEntry {
		return scopeEntry{Stack: "dev", Component: component, Section: map[string]any{"secrets": map[string]any{
			"providers": providers,
			"vars":      map[string]any{"DB_PASS": map[string]any{"sops": backend}},
		}}}
	}
	overrideEnumerateScopes(t, []scopeEntry{entry("x", "sops-a"), entry("y", selector)}, nil)

	origEvaluator := entrySelectorEvaluatorFn
	t.Cleanup(func() { entrySelectorEvaluatorFn = origEvaluator })

	t.Run("resolved selector collides", func(t *testing.T) {
		var evaluated []secretScope
		entrySelectorEvaluatorFn = func(scope secretScope) secrets.SelectorEvaluator {
			return func([]string, any) (any, error) {
				evaluated = append(evaluated, scope)
				return "sops-a", nil
			}
		}
		require.ErrorIs(t, checkStackSopsCollisions(secretScope{Stack: "dev", Identity: "me"}), secrets.ErrSopsCollision)
		require.Len(t, evaluated, 1, "only the selector-backed instance evaluates a selector")
		assert.Equal(t, secretScope{Stack: "dev", Component: "y", Identity: "me"}, evaluated[0])
	})

	t.Run("unresolvable selector fails closed", func(t *testing.T) {
		entrySelectorEvaluatorFn = func(secretScope) secrets.SelectorEvaluator {
			return func([]string, any) (any, error) { return nil, errUtils.ErrAwsCloudFormationStackNotFound }
		}
		err := checkStackSopsCollisions(secretScope{Stack: "dev"})
		require.ErrorIs(t, err, secrets.ErrSelectorUnresolved)
		for _, want := range []string{`"DB_PASS"`, `"y"`, `"dev"`, selector} {
			assert.Contains(t, err.Error(), want)
		}
	})
}

// TestScopedTemplateIsRenderedBeforeValidation proves a templated `scope:` is no longer rejected
// before it renders: a rendered scope that is consistent with the declaration's position works, and
// one that violates the one-way rule is rejected after rendering.
func TestScopedTemplateIsRenderedBeforeValidation(t *testing.T) {
	dir := writeMinimalAtmosProject(t)
	configPath := filepath.Join(dir, "atmos.yaml")
	configData, err := os.ReadFile(configPath)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(configPath, append(configData, []byte("templates:\n  settings:\n    enabled: true\n")...), 0o644))
	manifest := `vars:
  stage: dev
components:
  terraform:
    vpc:
      vars:
        shared_scope: global
        bad_scope: stack
      secrets:
        vars:
          SHARED:
            store: vault
            scope: '{{ .vars.shared_scope }}'
          BAD:
            store: vault
            scope: '{{ .vars.bad_scope }}'
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "stacks", "deploy", "dev.yaml"), []byte(manifest), 0o644))
	t.Chdir(dir)

	svc, err := loadService(secretScope{Stack: "dev", Component: "vpc"})
	require.NoError(t, err, "a templated scope must not be rejected before it renders")
	scope, ok := svc.ScopeOf("SHARED")
	require.True(t, ok)
	assert.Equal(t, secrets.ScopeGlobal, scope)

	_, err = svc.Get("BAD", secrets.ResolveOptions{})
	require.ErrorIs(t, err, secrets.ErrScopeConflict, "an instance declaration cannot render to stack scope")
}
