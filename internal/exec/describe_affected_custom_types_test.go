package exec

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	comp "github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	customTestType  = "script"
	customTestStack = "dev"
)

// basePathComponentProvider is a fakeComponentProvider with a source base path, like the
// provider that custom commands register for a kind declared with component.base_path.
type basePathComponentProvider struct {
	fakeComponentProvider
	basePath string
}

func (p *basePathComponentProvider) GetBasePath(_ *schema.AtmosConfiguration) string {
	return p.basePath
}

// registerCustomTestComponentType registers customTestType with a source base path. Callers must
// not call t.Parallel() because the component registry is global state.
func registerCustomTestComponentType(t *testing.T) {
	t.Helper()
	comp.Reset()
	require.NoError(t, comp.Register(&basePathComponentProvider{
		fakeComponentProvider: fakeComponentProvider{componentType: customTestType},
		basePath:              filepath.Join("components", customTestType),
	}))
	t.Cleanup(comp.Reset)
}

// customTypeStacks wraps component sections of the given type into the stacks map shape.
func customTypeStacks(componentType string, components map[string]any) map[string]any {
	return map[string]any{
		customTestStack: map[string]any{
			"components": map[string]any{
				componentType: components,
			},
		},
	}
}

func runProcessStackAffected(t *testing.T, atmosConfig *schema.AtmosConfiguration, remoteStacks, currentStacks map[string]any, changedFiles []string) []schema.Affected {
	t.Helper()
	affected, err := processStackAffected(
		customTestStack, currentStacks[customTestStack], &remoteStacks, &currentStacks,
		atmosConfig, newChangedFilesIndex(atmosConfig, changedFiles, ""), newComponentPathPatternCache(),
		false, false, AffectedFilter{},
	)
	require.NoError(t, err)
	return affected
}

// TestProcessStackAffected_RegisteredCustomComponentType covers #3208: describe affected must
// evaluate added and modified components of a registered custom component type, and ignore
// sections of types that are not registered.
//
//nolint:paralleltest // registerCustomTestComponentType mutates the shared component registry; must run serially.
func TestProcessStackAffected_RegisteredCustomComponentType(t *testing.T) {
	registerCustomTestComponentType(t)

	basePath := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: basePath}
	deploy := map[string]any{"vars": map[string]any{"replicas": 2}}

	t.Run("modified vars", func(t *testing.T) {
		remote := customTypeStacks(customTestType, map[string]any{"deploy": map[string]any{"vars": map[string]any{"replicas": 1}}})
		current := customTypeStacks(customTestType, map[string]any{"deploy": deploy})

		affected := runProcessStackAffected(t, atmosConfig, remote, current, nil)
		require.Len(t, affected, 1)
		assert.Equal(t, "deploy", affected[0].Component)
		assert.Equal(t, customTestType, affected[0].ComponentType)
		assert.Equal(t, customTestStack, affected[0].Stack)
		assert.Equal(t, affectedReasonStackVars, affected[0].Affected)
	})

	t.Run("added component", func(t *testing.T) {
		remote := customTypeStacks(customTestType, map[string]any{})
		current := customTypeStacks(customTestType, map[string]any{"deploy": deploy})

		affected := runProcessStackAffected(t, atmosConfig, remote, current, nil)
		require.Len(t, affected, 1)
		assert.Equal(t, "deploy", affected[0].Component)
		assert.Equal(t, customTestType, affected[0].ComponentType)
	})

	t.Run("changed file in the source folder", func(t *testing.T) {
		stacks := customTypeStacks(customTestType, map[string]any{"deploy": deploy})
		changedFile := filepath.Join(basePath, "components", customTestType, "deploy", "run.sh")

		affected := runProcessStackAffected(t, atmosConfig, stacks, stacks, []string{changedFile})
		require.Len(t, affected, 1)
		assert.Equal(t, "deploy", affected[0].Component)
		assert.Equal(t, customTestType, affected[0].ComponentType)
		assert.Equal(t, affectedReasonComponent, affected[0].Affected)
	})

	t.Run("unchanged component is not affected", func(t *testing.T) {
		stacks := customTypeStacks(customTestType, map[string]any{"deploy": deploy})
		otherFile := filepath.Join(basePath, "components", customTestType, "backup", "run.sh")

		affected := runProcessStackAffected(t, atmosConfig, stacks, stacks, []string{otherFile})
		assert.Empty(t, affected)
	})

	t.Run("unregistered type is ignored", func(t *testing.T) {
		remote := customTypeStacks("unregistered", map[string]any{})
		current := customTypeStacks("unregistered", map[string]any{"deploy": deploy})

		affected := runProcessStackAffected(t, atmosConfig, remote, current, nil)
		assert.Empty(t, affected)
	})
}

// TestDetectDeletedComponents_RegisteredCustomComponentType covers #3208: describe affected must
// report deleted components of a registered custom component type, both when the component is
// removed from a stack and when the whole stack is removed, and ignore unregistered types.
//
//nolint:paralleltest // registerCustomTestComponentType mutates the shared component registry; must run serially.
func TestDetectDeletedComponents_RegisteredCustomComponentType(t *testing.T) {
	registerCustomTestComponentType(t)

	atmosConfig := &schema.AtmosConfiguration{}
	remoteStacks := map[string]any{
		customTestStack: map[string]any{
			"components": map[string]any{
				customTestType: map[string]any{
					"deploy": map[string]any{"vars": map[string]any{"stage": "dev"}},
					"backup": map[string]any{"vars": map[string]any{"stage": "dev"}},
				},
				"unregistered": map[string]any{
					"ghost": map[string]any{"vars": map[string]any{"stage": "dev"}},
				},
			},
		},
	}

	t.Run("component deleted", func(t *testing.T) {
		currentStacks := customTypeStacks(customTestType, map[string]any{
			"backup": map[string]any{"vars": map[string]any{"stage": "dev"}},
		})

		deleted, err := detectDeletedComponents(&remoteStacks, &currentStacks, atmosConfig, "", AffectedFilter{})
		require.NoError(t, err)
		require.Len(t, deleted, 1)
		assert.Equal(t, "deploy", deleted[0].Component)
		assert.Equal(t, customTestType, deleted[0].ComponentType)
		assert.Equal(t, affectedReasonDeleted, deleted[0].Affected)
		assert.Equal(t, deletionTypeComponent, deleted[0].DeletionType)
		assert.Equal(t, "dev", deleted[0].Stage)
		assert.True(t, deleted[0].Deleted)
	})

	t.Run("stack deleted", func(t *testing.T) {
		currentStacks := map[string]any{}

		deleted, err := detectDeletedComponents(&remoteStacks, &currentStacks, atmosConfig, "", AffectedFilter{})
		require.NoError(t, err)
		require.Len(t, deleted, 2)
		names := []string{deleted[0].Component, deleted[1].Component}
		assert.ElementsMatch(t, []string{"deploy", "backup"}, names)
		for _, d := range deleted {
			assert.Equal(t, customTestType, d.ComponentType)
			assert.Equal(t, affectedReasonDeletedStack, d.Affected)
			assert.Equal(t, deletionTypeStack, d.DeletionType)
		}
	})
}

// TestGetComponentPathPattern_RegisteredCustomComponentType checks that a registered custom type
// uses the base path of its provider, that a provider without a base path has no source pattern,
// and that an unregistered type still returns ErrUnsupportedComponentType.
//
//nolint:paralleltest // registerFakeComponentTypes mutates the shared component registry; must run serially.
func TestGetComponentPathPattern_RegisteredCustomComponentType(t *testing.T) {
	registerFakeComponentTypes(t, "config-only")
	require.NoError(t, comp.Register(&basePathComponentProvider{
		fakeComponentProvider: fakeComponentProvider{componentType: customTestType},
		basePath:              filepath.Join("components", customTestType),
	}))

	basePath := t.TempDir()
	atmosConfig := &schema.AtmosConfiguration{BasePath: basePath}
	cache := newComponentPathPatternCache()

	pattern, err := cache.getComponentPathPattern("deploy", customTestType, atmosConfig)
	require.NoError(t, err)
	want, err := filepath.Abs(filepath.Join(basePath, "components", customTestType, "deploy"))
	require.NoError(t, err)
	assert.Equal(t, want+"/**", pattern)

	pattern, err = cache.getComponentPathPattern("deploy", "config-only", atmosConfig)
	require.NoError(t, err)
	assert.Empty(t, pattern)

	_, err = cache.getComponentPathPattern("deploy", "unregistered", atmosConfig)
	assert.ErrorIs(t, err, errUtils.ErrUnsupportedComponentType)
}
