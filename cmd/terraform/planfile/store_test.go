package planfile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/plugins/terraform/planfile"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guard so a rename of the planfile store configuration fields fails
// the build instead of silently skipping the behavior under test.
var _ = schema.PlanfilesConfig{Default: "", Priority: nil, Stores: nil}

// clearStoreEnv removes the environment variables that drive store detection.
func clearStoreEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ATMOS_PLANFILE_BUCKET", "")
	t.Setenv("ATMOS_PLANFILE_PREFIX", "")
	t.Setenv("GITHUB_ACTIONS", "")
}

// planfileConfig builds an Atmos configuration with planfile storage settings.
func planfileConfig(defaultStore string, priority []string, stores map[string]schema.PlanfileStoreSpec) *schema.AtmosConfiguration {
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Planfiles = schema.PlanfilesConfig{
		Default:  defaultStore,
		Priority: priority,
		Stores:   stores,
	}
	return atmosConfig
}

func TestResolveStore(t *testing.T) {
	t.Run("explicit store name is used", func(t *testing.T) {
		clearStoreEnv(t)
		t.Setenv("GITHUB_ACTIONS", "true")

		store, selected, err := resolveStore(planfileConfig("", nil, map[string]schema.PlanfileStoreSpec{
			"plans": {Type: planfile.LocalStoreType, Options: map[string]any{"path": t.TempDir()}},
		}), "plans")
		require.NoError(t, err)
		require.NotNil(t, store)
		assert.Equal(t, planfile.LocalStoreType, store.Name())
		assert.Equal(t, "plans", selected.Name)
		assert.Equal(t, planfile.StoreSourceExplicit, selected.Source)
	})

	t.Run("explicit store type is used", func(t *testing.T) {
		clearStoreEnv(t)

		store, selected, err := resolveStore(&schema.AtmosConfiguration{}, planfile.LocalStoreType)
		require.NoError(t, err)
		require.NotNil(t, store)
		assert.Equal(t, planfile.LocalStoreType, store.Name())
		assert.Equal(t, planfile.LocalStoreType, selected.Options.Type)
	})

	t.Run("unknown explicit store errors", func(t *testing.T) {
		clearStoreEnv(t)

		store, _, err := resolveStore(&schema.AtmosConfiguration{}, "does-not-exist")
		require.ErrorIs(t, err, errUtils.ErrPlanfileStoreNotFound)
		assert.Nil(t, store)
	})

	t.Run("configured priority is honored without --store", func(t *testing.T) {
		// The CLI used to consult only --store and the environment, so a
		// configured priority list was ignored here too.
		clearStoreEnv(t)
		t.Setenv("GITHUB_ACTIONS", "true")

		store, selected, err := resolveStore(planfileConfig("", []string{"plans"}, map[string]schema.PlanfileStoreSpec{
			"plans": {Type: planfile.LocalStoreType, Options: map[string]any{"path": t.TempDir()}},
		}), "")
		require.NoError(t, err)
		require.NotNil(t, store)
		assert.Equal(t, planfile.LocalStoreType, store.Name())
		assert.Equal(t, "plans", selected.Name)
		assert.Equal(t, planfile.StoreSourcePriority, selected.Source)
	})

	t.Run("configured default is honored without --store", func(t *testing.T) {
		clearStoreEnv(t)

		store, selected, err := resolveStore(planfileConfig("plans", nil, map[string]schema.PlanfileStoreSpec{
			"plans": {Type: planfile.LocalStoreType, Options: map[string]any{"path": t.TempDir()}},
		}), "")
		require.NoError(t, err)
		require.NotNil(t, store)
		assert.Equal(t, planfile.StoreSourceDefault, selected.Source)
	})

	t.Run("falls back to local storage", func(t *testing.T) {
		clearStoreEnv(t)

		store, selected, err := resolveStore(&schema.AtmosConfiguration{}, "")
		require.NoError(t, err)
		require.NotNil(t, store)
		assert.Equal(t, planfile.LocalStoreType, store.Name())
		assert.Equal(t, planfile.StoreSourceFallback, selected.Source)
		assert.Equal(t, planfile.DefaultLocalPath, selected.Options.Options["path"])
	})
}

func TestCreateStore(t *testing.T) {
	clearStoreEnv(t)

	store, err := createStore(planfileConfig("", []string{"plans"}, map[string]schema.PlanfileStoreSpec{
		"plans": {Type: planfile.LocalStoreType, Options: map[string]any{"path": t.TempDir()}},
	}), "")
	require.NoError(t, err)
	require.NotNil(t, store)
	assert.Equal(t, planfile.LocalStoreType, store.Name())
}

func TestDefaultKeyPattern(t *testing.T) {
	pattern := planfile.DefaultKeyPattern()
	assert.Contains(t, pattern.Pattern, "{{ .Stack }}")
	assert.Contains(t, pattern.Pattern, "{{ .Component }}")
	assert.Contains(t, pattern.Pattern, "{{ .SHA }}")
}
