package planfile

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time guards so a rename of the planfile store configuration fields
// fails the build instead of silently skipping the behavior under test.
var (
	_ = schema.PlanfilesConfig{Default: "", Priority: nil, Stores: nil}
	_ = schema.PlanfileStoreSpec{Type: "", Options: nil}
)

// configWithStores builds an Atmos configuration with the given planfile settings.
func configWithStores(defaultStore string, priority []string, stores map[string]schema.PlanfileStoreSpec) *schema.AtmosConfiguration {
	atmosConfig := &schema.AtmosConfiguration{}
	atmosConfig.Components.Terraform.Planfiles = schema.PlanfilesConfig{
		Default:  defaultStore,
		Priority: priority,
		Stores:   stores,
	}
	return atmosConfig
}

// clearStoreEnv removes the environment variables that drive store detection so a
// developer's or CI runner's environment cannot change the expected resolution.
func clearStoreEnv(t *testing.T) {
	t.Helper()
	t.Setenv("ATMOS_PLANFILE_BUCKET", "")
	t.Setenv("ATMOS_PLANFILE_PREFIX", "")
	t.Setenv("GITHUB_ACTIONS", "")
}

func TestResolveStoreCandidates_Priority(t *testing.T) {
	stores := map[string]schema.PlanfileStoreSpec{
		"s3": {
			Type:    S3StoreType,
			Options: map[string]any{"bucket": "my-planfiles", "region": "us-east-1"},
		},
		"github": {Type: GitHubStoreType},
		"local":  {Type: LocalStoreType, Options: map[string]any{"path": ".atmos/planfiles"}},
	}

	t.Run("priority is honored inside GitHub Actions", func(t *testing.T) {
		// Regression test for the reported bug: an explicitly configured S3 store
		// used to be replaced by GitHub Artifacts because `priority` was ignored
		// and resolution fell through to environment detection.
		clearStoreEnv(t)
		t.Setenv("GITHUB_ACTIONS", "true")

		candidates, err := ResolveStoreCandidates(configWithStores("", []string{"s3"}, stores), "")
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		assert.Equal(t, "s3", candidates[0].Name)
		assert.Equal(t, S3StoreType, candidates[0].Options.Type)
		assert.Equal(t, StoreSourcePriority, candidates[0].Source)
		assert.Equal(t, "my-planfiles", candidates[0].Options.Options["bucket"])
	})

	t.Run("all priority entries are returned in order", func(t *testing.T) {
		clearStoreEnv(t)

		candidates, err := ResolveStoreCandidates(configWithStores("", []string{"github", "s3", "local"}, stores), "")
		require.NoError(t, err)
		require.Len(t, candidates, 3)
		assert.Equal(t, []string{"github", "s3", "local"}, []string{candidates[0].Name, candidates[1].Name, candidates[2].Name})
		assert.Equal(t, GitHubStoreType, candidates[0].Options.Type)
		assert.Equal(t, LocalStoreType, candidates[2].Options.Type)
	})

	t.Run("priority accepts a bare store type", func(t *testing.T) {
		clearStoreEnv(t)

		candidates, err := ResolveStoreCandidates(configWithStores("", []string{LocalStoreType}, nil), "")
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		assert.Equal(t, LocalStoreType, candidates[0].Options.Type)
		assert.Empty(t, candidates[0].Options.Options)
	})

	t.Run("unknown priority entry is a configuration error", func(t *testing.T) {
		clearStoreEnv(t)
		t.Setenv("GITHUB_ACTIONS", "true")

		candidates, err := ResolveStoreCandidates(configWithStores("", []string{"typo"}, stores), "")
		require.Error(t, err)
		assert.Nil(t, candidates)
		require.ErrorIs(t, err, errUtils.ErrPlanfileStoreNotFound)
		assert.Contains(t, err.Error(), "components.terraform.planfiles.priority[0]")
		assert.Contains(t, err.Error(), `"typo"`)
		// The message lists the stores the user actually defined.
		assert.Contains(t, err.Error(), "github, local, s3")
	})

	t.Run("store without a type is a configuration error", func(t *testing.T) {
		clearStoreEnv(t)

		_, err := ResolveStoreCandidates(configWithStores("", []string{"plans"}, map[string]schema.PlanfileStoreSpec{
			"plans": {Options: map[string]any{"bucket": "b"}},
		}), "")
		require.ErrorIs(t, err, errUtils.ErrPlanfileStoreInvalidArgs)
		assert.Contains(t, err.Error(), `store "plans"`)
	})

	t.Run("priority reports the failing index", func(t *testing.T) {
		clearStoreEnv(t)

		_, err := ResolveStoreCandidates(configWithStores("", []string{"s3", "nope"}, stores), "")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "components.terraform.planfiles.priority[1]")
	})
}

func TestResolveStoreCandidates_Default(t *testing.T) {
	stores := map[string]schema.PlanfileStoreSpec{
		"s3":     {Type: S3StoreType, Options: map[string]any{"bucket": "b"}},
		"github": {Type: GitHubStoreType},
	}

	t.Run("default selects the named store", func(t *testing.T) {
		clearStoreEnv(t)
		t.Setenv("GITHUB_ACTIONS", "true")

		candidates, err := ResolveStoreCandidates(configWithStores("s3", nil, stores), "")
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		assert.Equal(t, "s3", candidates[0].Name)
		assert.Equal(t, StoreSourceDefault, candidates[0].Source)
	})

	t.Run("default takes precedence over priority", func(t *testing.T) {
		clearStoreEnv(t)

		candidates, err := ResolveStoreCandidates(configWithStores("s3", []string{"github"}, stores), "")
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		assert.Equal(t, "s3", candidates[0].Name)
		assert.Equal(t, StoreSourceDefault, candidates[0].Source)
	})

	t.Run("unknown default is a configuration error", func(t *testing.T) {
		clearStoreEnv(t)
		t.Setenv("GITHUB_ACTIONS", "true")

		_, err := ResolveStoreCandidates(configWithStores("missing", nil, stores), "")
		require.ErrorIs(t, err, errUtils.ErrPlanfileStoreNotFound)
		assert.Contains(t, err.Error(), "components.terraform.planfiles.default")
	})
}

func TestResolveStoreCandidates_SingleStore(t *testing.T) {
	t.Run("a lone named store is used without default or priority", func(t *testing.T) {
		clearStoreEnv(t)
		t.Setenv("GITHUB_ACTIONS", "true")

		stores := map[string]schema.PlanfileStoreSpec{
			"s3": {Type: S3StoreType, Options: map[string]any{"bucket": "b"}},
		}

		candidates, err := ResolveStoreCandidates(configWithStores("", nil, stores), "")
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		assert.Equal(t, "s3", candidates[0].Name)
		assert.Equal(t, StoreSourceOnlyStore, candidates[0].Source)
	})

	t.Run("several stores with no selection falls back to environment detection", func(t *testing.T) {
		clearStoreEnv(t)
		t.Setenv("GITHUB_ACTIONS", "true")

		stores := map[string]schema.PlanfileStoreSpec{
			"s3":     {Type: S3StoreType, Options: map[string]any{"bucket": "b"}},
			"github": {Type: GitHubStoreType},
		}

		candidates, err := ResolveStoreCandidates(configWithStores("", nil, stores), "")
		require.NoError(t, err)
		require.Len(t, candidates, 2)
		assert.Equal(t, GitHubStoreType, candidates[0].Options.Type)
		assert.Equal(t, StoreSourceEnvironment, candidates[0].Source)
		assert.Equal(t, LocalStoreType, candidates[1].Options.Type)
		assert.Equal(t, StoreSourceFallback, candidates[1].Source)
	})
}

func TestResolveStoreCandidates_Explicit(t *testing.T) {
	stores := map[string]schema.PlanfileStoreSpec{
		"s3": {Type: S3StoreType, Options: map[string]any{"bucket": "b"}},
	}

	t.Run("explicit name overrides configuration and environment", func(t *testing.T) {
		clearStoreEnv(t)
		t.Setenv("ATMOS_PLANFILE_BUCKET", "env-bucket")

		candidates, err := ResolveStoreCandidates(configWithStores("s3", []string{"s3"}, stores), LocalStoreType)
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		assert.Equal(t, LocalStoreType, candidates[0].Options.Type)
		assert.Equal(t, StoreSourceExplicit, candidates[0].Source)
	})

	t.Run("explicit name resolves a configured store", func(t *testing.T) {
		clearStoreEnv(t)

		candidates, err := ResolveStoreCandidates(configWithStores("", nil, stores), "s3")
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		assert.Equal(t, S3StoreType, candidates[0].Options.Type)
		assert.Equal(t, "b", candidates[0].Options.Options["bucket"])
	})

	t.Run("unknown explicit name errors", func(t *testing.T) {
		clearStoreEnv(t)

		_, err := ResolveStoreCandidates(configWithStores("", nil, stores), "nope")
		require.ErrorIs(t, err, errUtils.ErrPlanfileStoreNotFound)
		assert.Contains(t, err.Error(), "--store")
	})
}

func TestResolveStoreCandidates_Environment(t *testing.T) {
	t.Run("S3 environment variables are detected first", func(t *testing.T) {
		clearStoreEnv(t)
		t.Setenv("ATMOS_PLANFILE_BUCKET", "env-bucket")
		t.Setenv("ATMOS_PLANFILE_PREFIX", "plans/")
		t.Setenv("AWS_REGION", "us-west-2")
		t.Setenv("GITHUB_ACTIONS", "true")

		candidates, err := ResolveStoreCandidates(&schema.AtmosConfiguration{}, "")
		require.NoError(t, err)
		require.Len(t, candidates, 3)
		assert.Equal(t, S3StoreType, candidates[0].Options.Type)
		assert.Equal(t, "env-bucket", candidates[0].Options.Options["bucket"])
		assert.Equal(t, "plans/", candidates[0].Options.Options["prefix"])
		assert.Equal(t, "us-west-2", candidates[0].Options.Options["region"])
		assert.Equal(t, GitHubStoreType, candidates[1].Options.Type)
		assert.Equal(t, LocalStoreType, candidates[2].Options.Type)
	})

	t.Run("GitHub Actions is detected", func(t *testing.T) {
		clearStoreEnv(t)
		t.Setenv("GITHUB_ACTIONS", "true")

		candidates, err := ResolveStoreCandidates(&schema.AtmosConfiguration{}, "")
		require.NoError(t, err)
		require.Len(t, candidates, 2)
		assert.Equal(t, GitHubStoreType, candidates[0].Options.Type)
		assert.Equal(t, DefaultGitHubPrefix, candidates[0].Options.Options["prefix"])
		assert.Equal(t, LocalStoreType, candidates[1].Options.Type)
	})

	t.Run("no configuration and no environment falls back to local", func(t *testing.T) {
		clearStoreEnv(t)

		candidates, err := ResolveStoreCandidates(&schema.AtmosConfiguration{}, "")
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		assert.Equal(t, LocalStoreType, candidates[0].Options.Type)
		assert.Equal(t, DefaultLocalPath, candidates[0].Options.Options["path"])
		assert.Equal(t, StoreSourceFallback, candidates[0].Source)
	})

	t.Run("nil configuration falls back to local", func(t *testing.T) {
		clearStoreEnv(t)

		candidates, err := ResolveStoreCandidates(nil, "")
		require.NoError(t, err)
		require.Len(t, candidates, 1)
		assert.Equal(t, LocalStoreType, candidates[0].Options.Type)
		assert.Nil(t, candidates[0].Options.AtmosConfig)
	})
}

func TestResolveStoreCandidates_AtmosConfigAttached(t *testing.T) {
	clearStoreEnv(t)
	t.Setenv("GITHUB_ACTIONS", "true")

	atmosConfig := configWithStores("", []string{"github", LocalStoreType}, map[string]schema.PlanfileStoreSpec{
		"github": {Type: GitHubStoreType},
	})

	candidates, err := ResolveStoreCandidates(atmosConfig, "")
	require.NoError(t, err)
	require.Len(t, candidates, 2)
	for _, candidate := range candidates {
		assert.Same(t, atmosConfig, candidate.Options.AtmosConfig)
	}
}

func TestStoreCandidateDescription(t *testing.T) {
	tests := []struct {
		name      string
		candidate StoreCandidate
		expected  string
	}{
		{
			name:      "named store includes its type",
			candidate: StoreCandidate{Name: "plans", Options: StoreOptions{Type: S3StoreType}},
			expected:  "plans (aws/s3)",
		},
		{
			name:      "store named by type is not repeated",
			candidate: StoreCandidate{Name: S3StoreType, Options: StoreOptions{Type: S3StoreType}},
			expected:  S3StoreType,
		},
		{
			name:      "unnamed store uses its type",
			candidate: StoreCandidate{Options: StoreOptions{Type: LocalStoreType}},
			expected:  LocalStoreType,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expected, test.candidate.Description())
		})
	}
}
