package secret

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/secrets"
)

// TestEnumerateGeneratedDeclarationsAfterFailure exercises raw discovery after an unrelated
// component fails. Generated declarations must still be evaluated before checking SOPS files.
func TestEnumerateGeneratedDeclarationsAfterFailure(t *testing.T) {
	cases := []struct {
		name        string
		declaration string
		file        string
		broken      bool
		wantErr     error
	}{
		{"generated SOPS collision", `{"TOKEN":{"sops":"local"}}`, "shared.enc.yaml", false, secrets.ErrSopsCollision},
		{"generated SOPS distinct file", `{"TOKEN":{"sops":"local"}}`, "generated.enc.yaml", false, nil},
		{"generated store declaration", `{"TOKEN":{"store":"vault"}}`, "shared.enc.yaml", false, nil},
		{"unevaluable generated declaration", `{"TOKEN":{"sops":"local","description":"{{ .vars }}"}}`, "shared.enc.yaml", true, ErrSecretScopeEvaluation},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := writeProjectWithGeneratedDeclaration(t, tc.declaration, tc.file, tc.broken)
			t.Chdir(dir)
			entries, _, err := enumerateSecretScopes(secretScope{Stack: "dev"})
			require.ErrorIs(t, err, ErrSecretScopeEvaluation, "the unrelated dynamic component always fails")
			if tc.broken {
				assert.Equal(t, []string{"anchor", "static"}, entryComponents(entries))
				assert.Contains(t, err.Error(), `component "generated" in stack "dev"`)
			} else {
				assert.Equal(t, []string{"anchor", "generated", "static"}, entryComponents(entries))
				require.Len(t, entries, 3)
				assert.Contains(t, secrets.ExtractDeclarations(entries[1].Section), "TOKEN")
				assert.NotContains(t, err.Error(), `component "generated"`)
			}
			err = checkStackSopsCollisions(secretScope{Stack: "dev"})
			if tc.wantErr == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.wantErr)
				assert.Contains(t, err.Error(), "generated")
			}
		})
	}
}

// writeProjectWithGeneratedDeclaration adds a generated declaration and a fixed SOPS declaration
// to a project with an unrelated broken store-backed component, forcing per-component retries.
func writeProjectWithGeneratedDeclaration(t *testing.T, declaration, file string, broken bool) string {
	t.Helper()
	dir := writeProjectWithDynamicDeclaration(t, "store: secrets/ssm")
	manifestPath := filepath.Join(dir, "stacks", "deploy", "dev.yaml")
	manifest, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	brokenVars := ""
	if broken {
		brokenVars = `      vars:
        other: '{{ (atmos.Component "missing-generated-dependency" "dev").outputs.id }}'
`
	}
	manifest = append(manifest, []byte(`    anchor:
      secrets:
        providers:
          local:
            kind: sops/age
            spec:
              file: shared.enc.yaml
        vars:
          ANCHOR:
            sops: local
    generated:
`+brokenVars+`      secrets:
        providers:
          local:
            kind: sops/age
            spec:
              file: `+file+`
        vars: !template '`+declaration+`'
`)...)
	require.NoError(t, os.WriteFile(manifestPath, manifest, 0o644))
	for _, component := range []string{"anchor", "generated"} {
		componentDir := filepath.Join(dir, "components", "terraform", component)
		require.NoError(t, os.MkdirAll(componentDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(componentDir, "main.tf"), []byte("# Component.\n"), 0o644))
	}
	return dir
}

// TestCollectSecretScopesUnresolvedShapes separates retry candidates from resolved declarations.
func TestCollectSecretScopesUnresolvedShapes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		section map[string]any
		want    []string
	}{
		{"no secrets", map[string]any{}, nil},
		{"no vars", map[string]any{"secrets": map[string]any{}}, nil},
		{"empty map", map[string]any{"secrets": map[string]any{"vars": map[string]any{}}}, nil},
		{"null vars", map[string]any{"secrets": map[string]any{"vars": nil}}, []string{"generated"}},
		{"template", map[string]any{"secrets": map[string]any{"vars": "!template {}"}}, []string{"generated"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stacks := map[string]any{"dev": map[string]any{"components": map[string]any{
				"terraform": map[string]any{"generated": tc.section},
			}}}
			candidates := collectSecretScopeCandidates(stacks, "")
			if tc.want == nil {
				assert.Empty(t, candidates)
			} else {
				assert.Equal(t, tc.want, entryComponents(candidates))
			}
			assert.Empty(t, collectSecretScopeEntries(stacks, ""), "unresolved values are not listed as actual declarations")
		})
	}
}
