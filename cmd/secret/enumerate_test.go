package secret

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/secrets"
)

// declaringSection returns a resolved component section declaring a single store-backed secret,
// so secrets.ExtractDeclarations reports one declaration for it.
func declaringSection(name string) map[string]any {
	return map[string]any{
		"secrets": map[string]any{
			"vars": map[string]any{
				name: map[string]any{"store": "app-secrets"},
			},
		},
	}
}

// sopsSection returns a component section wiring a SOPS provider (resolving to file) and one
// instance-scoped SOPS-backed secret. Used to exercise the write-time collision guard.
func sopsSection(file string) map[string]any {
	const (
		provider   = "v"
		secretName = "K"
	)
	return map[string]any{
		"secrets": map[string]any{
			"providers": map[string]any{
				provider: map[string]any{"kind": "sops/age", "spec": map[string]any{"file": file}},
			},
			"vars": map[string]any{
				secretName: map[string]any{"sops": provider},
			},
		},
	}
}

// TestCollectSecretScopeEntries proves the describe-stacks traversal keeps only secret-declaring
// instances, skips non-map nodes, and returns entries sorted by stack then component.
func TestCollectSecretScopeEntries(t *testing.T) {
	stacksMap := map[string]any{
		"prod": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				"terraform": map[string]any{
					"web": declaringSection("WEB_KEY"),
					"api": declaringSection("API_KEY"),
					// A component with no secrets is excluded.
					"empty": map[string]any{"vars": map[string]any{"x": 1}},
				},
			},
		},
		"dev": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				"terraform": map[string]any{
					"vpc": declaringSection("VPC_KEY"),
				},
			},
		},
		// Non-map stack value is skipped without panicking.
		"broken": "not-a-map",
	}

	entries := collectSecretScopeEntries(stacksMap, "")
	require.Len(t, entries, 3)

	// Sorted by stack, then component: dev/vpc, prod/api, prod/web.
	assert.Equal(t, "dev", entries[0].Stack)
	assert.Equal(t, "vpc", entries[0].Component)
	assert.Equal(t, "prod", entries[1].Stack)
	assert.Equal(t, "api", entries[1].Component)
	assert.Equal(t, "prod", entries[2].Stack)
	assert.Equal(t, "web", entries[2].Component)
}

// TestCollectSecretScopeEntries_ComponentFilter narrows the result to a single component.
func TestCollectSecretScopeEntries_ComponentFilter(t *testing.T) {
	stacksMap := map[string]any{
		"prod": map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				"terraform": map[string]any{
					"web": declaringSection("WEB_KEY"),
					"api": declaringSection("API_KEY"),
				},
			},
		},
	}

	entries := collectSecretScopeEntries(stacksMap, "api")
	require.Len(t, entries, 1)
	assert.Equal(t, "api", entries[0].Component)
}

// TestSecretEntriesInStack covers the per-stack edge cases: a missing components section, a
// non-map component-type node, and a section that is not a map.
func TestSecretEntriesInStack(t *testing.T) {
	t.Run("missing components section", func(t *testing.T) {
		assert.Nil(t, secretEntriesInStack("prod", map[string]any{}, ""))
	})

	t.Run("non-map nodes are skipped", func(t *testing.T) {
		stackMap := map[string]any{
			cfg.ComponentsSectionName: map[string]any{
				"terraform": "not-a-map", // type node is not a map
				"helmfile":  map[string]any{"chart": "not-a-map"},
				"packer":    map[string]any{"ami": declaringSection("AMI_KEY")},
			},
		}
		entries := secretEntriesInStack("prod", stackMap, "")
		require.Len(t, entries, 1)
		assert.Equal(t, "ami", entries[0].Component)
	})
}

// TestDistinct proves distinct de-duplicates, drops empty values, and sorts.
func TestDistinct(t *testing.T) {
	entries := []scopeEntry{
		{Stack: "prod", Component: "web"},
		{Stack: "dev", Component: "api"},
		{Stack: "prod", Component: "api"}, // duplicate stack "prod"
		{Stack: "", Component: ""},        // empty values dropped
	}

	stacks := distinct(entries, func(e scopeEntry) string { return e.Stack })
	assert.Equal(t, []string{"dev", "prod"}, stacks)

	components := distinct(entries, func(e scopeEntry) string { return e.Component })
	assert.Equal(t, []string{"api", "web"}, components)
}

// TestStackCompletion exercises the shell-completion seam for --stack in both the success and
// error directions.
func TestStackCompletion(t *testing.T) {
	t.Run("returns distinct stacks", func(t *testing.T) {
		overrideEnumerateScopes(t, []scopeEntry{
			{Stack: "prod", Component: "api"},
			{Stack: "prod", Component: "web"},
			{Stack: "dev", Component: "api"},
		}, nil)

		got, directive := stackCompletion(&cobra.Command{}, nil, "")
		assert.Equal(t, []string{"dev", "prod"}, got)
		assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	})

	t.Run("enumerate error yields no completions", func(t *testing.T) {
		overrideEnumerateScopes(t, nil, errors.New("boom"))

		got, directive := stackCompletion(&cobra.Command{}, nil, "")
		assert.Nil(t, got)
		assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	})
}

// TestComponentCompletion exercises the shell-completion seam for --component, including that it
// reads the currently-selected stack from viper.
func TestComponentCompletion(t *testing.T) {
	t.Run("returns distinct components", func(t *testing.T) {
		viper.Reset()
		t.Cleanup(viper.Reset)
		viper.Set("stack", "prod")
		overrideEnumerateScopes(t, []scopeEntry{
			{Stack: "prod", Component: "web"},
			{Stack: "prod", Component: "api"},
		}, nil)

		got, directive := componentCompletion(&cobra.Command{}, nil, "")
		assert.Equal(t, []string{"api", "web"}, got)
		assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	})

	t.Run("enumerate error yields no completions", func(t *testing.T) {
		overrideEnumerateScopes(t, nil, errors.New("boom"))

		got, directive := componentCompletion(&cobra.Command{}, nil, "")
		assert.Nil(t, got)
		assert.Equal(t, cobra.ShellCompDirectiveNoFileComp, directive)
	})
}

// TestCheckStackSopsCollisions covers the write-time SOPS collision guard: the no-collision path,
// a real instance-level collision (two components sharing one non-discriminating file), and the
// error pass-through from enumeration.
func TestCheckStackSopsCollisions(t *testing.T) {
	t.Run("distinct files pass", func(t *testing.T) {
		overrideEnumerateScopes(t, []scopeEntry{
			{Stack: "prod", Component: "api", Section: sopsSection("secrets/prod.api.enc.yaml")},
			{Stack: "prod", Component: "web", Section: sopsSection("secrets/prod.web.enc.yaml")},
		}, nil)

		require.NoError(t, checkStackSopsCollisions("prod"))
	})

	t.Run("shared instance file collides", func(t *testing.T) {
		overrideEnumerateScopes(t, []scopeEntry{
			{Stack: "prod", Component: "api", Section: sopsSection("secrets/shared.enc.yaml")},
			{Stack: "prod", Component: "web", Section: sopsSection("secrets/shared.enc.yaml")},
		}, nil)

		require.ErrorIs(t, checkStackSopsCollisions("prod"), secrets.ErrSopsCollision)
	})

	t.Run("enumerate error is returned", func(t *testing.T) {
		sentinel := errors.New("enumerate failed")
		overrideEnumerateScopes(t, nil, sentinel)

		require.ErrorIs(t, checkStackSopsCollisions("prod"), sentinel)
	})
}
