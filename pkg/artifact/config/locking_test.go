package config

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/schema"
)

func ptr[T any](v T) *T { return &v }

func withLocking(repo *schema.ArtifactRepositoryConfig, scope, delegate string) schema.ArtifactRepositoryConfig {
	out := *repo
	out.Locking = &schema.ArtifactLockingConfig{RequiredScope: scope, Repository: delegate}
	return out
}

func TestValidate_PRDLockingExample(t *testing.T) {
	cfg := &schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{
		"shared":       withLocking(ptr(registryRepo()), "repository", "coordination"),
		"coordination": s3Repo(),
	}}
	require.NoError(t, Validate(cfg))

	plan, err := ResolveLockPlan(cfg, "shared")
	require.NoError(t, err)
	assert.Equal(t, LockPlan{Provider: "coordination", Required: artifact.LockScopeRepository, Chain: []string{"shared", "coordination"}}, plan)
}

func TestValidate_LockingScopes(t *testing.T) {
	tests := []struct {
		name    string
		repos   map[string]schema.ArtifactRepositoryConfig
		alias   string
		wantErr string
		wantCap bool
	}{
		{
			name:  "host scope is the default and always available",
			repos: map[string]schema.ArtifactRepositoryConfig{"r": withLocking(ptr(registryRepo()), "", "")},
			alias: "r",
		},
		{
			name:    "registry cannot provide repository scope itself",
			repos:   map[string]schema.ArtifactRepositoryConfig{"r": withLocking(ptr(registryRepo()), "repository", "")},
			alias:   "r",
			wantErr: "not provided by",
			wantCap: true,
		},
		{
			name:    "local directory cannot provide repository scope",
			repos:   map[string]schema.ArtifactRepositoryConfig{"r": withLocking(ptr(localRepo(t.TempDir())), "repository", "")},
			alias:   "r",
			wantErr: "not provided by",
			wantCap: true,
		},
		{
			name:  "s3 provides repository scope itself",
			repos: map[string]schema.ArtifactRepositoryConfig{"r": withLocking(ptr(s3Repo()), "repository", "")},
			alias: "r",
		},
		{
			name: "delegate lacking repository scope is insufficient",
			repos: map[string]schema.ArtifactRepositoryConfig{
				"r": withLocking(ptr(registryRepo()), "repository", "other"),
				"other": func() schema.ArtifactRepositoryConfig {
					repo := registryRepo()
					repo.Options["prefix"] = "other"
					return repo
				}(),
			},
			alias:   "r",
			wantErr: `not provided by "other"`,
			wantCap: true,
		},
		{
			name:    "unknown scope",
			repos:   map[string]schema.ArtifactRepositoryConfig{"r": withLocking(ptr(s3Repo()), "global", "")},
			alias:   "r",
			wantErr: "required_scope",
		},
		{
			name:    "unknown delegate",
			repos:   map[string]schema.ArtifactRepositoryConfig{"r": withLocking(ptr(registryRepo()), "repository", "missing")},
			alias:   "r",
			wantErr: `delegates locking to "missing", which is not configured`,
		},
		{
			name:  "host scope with a stronger delegate is fine",
			repos: map[string]schema.ArtifactRepositoryConfig{"r": withLocking(ptr(registryRepo()), "host", "c"), "c": s3Repo()},
			alias: "r",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := NewValidator().ValidateRepository(&schema.ArtifactConfig{Repositories: tt.repos}, tt.alias)
			if tt.wantErr == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
			assert.Contains(t, err.Error(), tt.wantErr)
			assert.Equal(t, tt.wantCap, errors.Is(err, errUtils.ErrArtifactUnsupportedCapability))
		})
	}
}

func TestValidate_DelegatedLockCyclesAreRejected(t *testing.T) {
	tests := []struct {
		name  string
		repos map[string]schema.ArtifactRepositoryConfig
		want  string
	}{
		{
			name:  "self delegation",
			repos: map[string]schema.ArtifactRepositoryConfig{"a": withLocking(ptr(s3Repo()), "repository", "a")},
			want:  "a -> a",
		},
		{
			name: "two repositories",
			repos: map[string]schema.ArtifactRepositoryConfig{
				"a": withLocking(ptr(s3Repo()), "repository", "b"),
				"b": withLocking(ptr(s3Repo()), "repository", "a"),
			},
			want: "a -> b -> a",
		},
		{
			name: "three repositories",
			repos: map[string]schema.ArtifactRepositoryConfig{
				"a": withLocking(ptr(s3Repo()), "repository", "b"),
				"b": withLocking(ptr(s3Repo()), "repository", "c"),
				"c": withLocking(ptr(s3Repo()), "repository", "a"),
			},
			want: "a -> b -> c -> a",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := Validate(&schema.ArtifactConfig{Repositories: tt.repos})
			require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
			assert.Contains(t, err.Error(), "lock delegation cycle")
			assert.Contains(t, err.Error(), tt.want)
		})
	}
}

func TestResolveLockPlan_FollowsChainToTheProvider(t *testing.T) {
	cfg := &schema.ArtifactConfig{Repositories: map[string]schema.ArtifactRepositoryConfig{
		"registry": withLocking(ptr(registryRepo()), "repository", "middle"),
		"middle":   withLocking(ptr(s3Repo()), "repository", "final"),
		"final":    s3Repo(),
	}}
	plan, err := ResolveLockPlan(cfg, "registry")
	require.NoError(t, err)
	assert.Equal(t, "final", plan.Provider)
	assert.Equal(t, []string{"registry", "middle", "final"}, plan.Chain)

	plain, err := ResolveLockPlan(cfg, "final")
	require.NoError(t, err)
	assert.Equal(t, LockPlan{Provider: "final", Required: artifact.LockScopeHost, Chain: []string{"final"}}, plain)

	_, err = ResolveLockPlan(cfg, "nope")
	require.ErrorIs(t, err, errUtils.ErrArtifactConfigInvalid)
}
