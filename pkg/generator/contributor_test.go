package generator

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// fakeContributor is a test double for ProviderContributor.
type fakeContributor struct {
	name     string
	fragment map[string]any
	err      error
}

func (f fakeContributor) Name() string { return f.name }

func (f fakeContributor) Contribute(_ context.Context, _ *GeneratorContext) (map[string]any, error) {
	return f.fragment, f.err
}

// withCleanRegistry swaps in an isolated contributor registry for the duration of a test.
func withCleanRegistry(t *testing.T) {
	t.Helper()
	contributorRegistryMu.Lock()
	saved := contributorRegistry
	contributorRegistry = map[string]ProviderContributor{}
	contributorRegistryMu.Unlock()
	t.Cleanup(func() {
		contributorRegistryMu.Lock()
		contributorRegistry = saved
		contributorRegistryMu.Unlock()
	})
}

func newGenCtx(providers map[string]any) *GeneratorContext {
	return &GeneratorContext{
		AtmosConfig:      &schema.AtmosConfiguration{},
		ProvidersSection: providers,
	}
}

func TestApplyProviderContributors_MergesUnderExplicit(t *testing.T) {
	withCleanRegistry(t)

	// Explicit stack providers set region=us-west-2; the contribution proposes
	// region=us-east-1 plus skip flags. Explicit must win on region; the flags merge in.
	RegisterProviderContributor(fakeContributor{
		name: "emulator",
		fragment: map[string]any{
			"aws": map[string]any{
				"region":                     "us-east-1",
				"skip_requesting_account_id": true,
				"s3_use_path_style":          true,
			},
		},
	})

	genCtx := newGenCtx(map[string]any{
		"aws": map[string]any{"region": "us-west-2"},
	})

	merged, err := ApplyProviderContributors(context.Background(), genCtx)
	require.NoError(t, err)

	aws, ok := merged["aws"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "us-west-2", aws["region"], "explicit stack providers win over the contribution")
	assert.Equal(t, true, aws["skip_requesting_account_id"], "contributed flag merged in")
	assert.Equal(t, true, aws["s3_use_path_style"], "contributed flag merged in")
}

func TestApplyProviderContributors_IntoEmptySection(t *testing.T) {
	withCleanRegistry(t)
	RegisterProviderContributor(fakeContributor{
		name:     "emulator",
		fragment: map[string]any{"aws": map[string]any{"skip_metadata_api_check": true}},
	})

	genCtx := newGenCtx(nil)
	merged, err := ApplyProviderContributors(context.Background(), genCtx)
	require.NoError(t, err)

	aws, ok := merged["aws"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, aws["skip_metadata_api_check"])
}

func TestApplyProviderContributors_EmptyAndNilFragmentsSkipped(t *testing.T) {
	withCleanRegistry(t)
	RegisterProviderContributor(fakeContributor{name: "a", fragment: nil})
	RegisterProviderContributor(fakeContributor{name: "b", fragment: map[string]any{}})

	genCtx := newGenCtx(map[string]any{"aws": map[string]any{"region": "us-west-2"}})
	merged, err := ApplyProviderContributors(context.Background(), genCtx)
	require.NoError(t, err)
	assert.Equal(t, map[string]any{"aws": map[string]any{"region": "us-west-2"}}, merged, "no-op contributors leave the section unchanged")
}

func TestApplyProviderContributors_ErrorPropagates(t *testing.T) {
	withCleanRegistry(t)
	sentinel := errors.New("boom")
	RegisterProviderContributor(fakeContributor{name: "a", err: sentinel})

	_, err := ApplyProviderContributors(context.Background(), newGenCtx(nil))
	require.Error(t, err)
	assert.ErrorIs(t, err, sentinel)
}

func TestProviderContributors_SortedAndRegistered(t *testing.T) {
	withCleanRegistry(t)
	RegisterProviderContributor(fakeContributor{name: "zeta"})
	RegisterProviderContributor(fakeContributor{name: "alpha"})

	got := ProviderContributors()
	require.Len(t, got, 2)
	assert.Equal(t, "alpha", got[0].Name())
	assert.Equal(t, "zeta", got[1].Name())
}

func TestApplyProviderContributors_NilContext(t *testing.T) {
	merged, err := ApplyProviderContributors(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, merged)
}
