package install

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
)

//go:generate go run go.uber.org/mock/mockgen -destination=version_remote_mock_test.go -package=install github.com/cloudposse/atmos/pkg/vendoring/version RemoteLister

func TestResolveEffectiveVersionContext(t *testing.T) {
	for _, mode := range []string{"exact", "range", "unnamed", "canceled"} {
		t.Run(mode, func(t *testing.T) {
			config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
			lister := NewMockRemoteLister(gomock.NewController(t))
			in := &ResolveEffectiveVersionInputs{AtmosConfig: config, Name: "component", Source: "git::https://github.com/example/repo.git?ref={{.Version}}", RawVersion: "v1.0.0", Lister: lister}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode != "exact" {
				in.RawVersion = ">= 1.0.0, < 2.0.0"
				if mode == "unnamed" {
					in.Name = ""
				}
				lister.EXPECT().ListTags(gomock.Any(), "https://github.com/example/repo.git").DoAndReturn(func(got context.Context, _ string) ([]string, error) {
					if mode == "canceled" {
						cancel()
						<-got.Done()
						return nil, got.Err()
					}
					return []string{"v1.0.0", "v1.2.0", "v2.0.0"}, nil
				})
			}
			resolved, raw, err := ResolveEffectiveVersionContext(ctx, in)
			if mode == "canceled" {
				require.ErrorIs(t, err, context.Canceled)
				assert.Empty(t, resolved)
				assert.Empty(t, raw)
				require.NoFileExists(t, lockfile.Path(config))
				return
			}
			require.NoError(t, err)
			if mode == "exact" {
				assert.Equal(t, "v1.0.0", resolved)
				assert.Empty(t, raw)
				require.NoFileExists(t, lockfile.Path(config))
				return
			}
			assert.Equal(t, "v1.2.0", resolved)
			assert.Equal(t, in.RawVersion, raw)
			cached, cachedRaw, err := ResolveEffectiveVersion(in)
			require.NoError(t, err)
			assert.Equal(t, resolved, cached)
			assert.Equal(t, raw, cachedRaw)
		})
	}
}

func TestRecordResolvedVersionHonorsCancellationBeforeLock(t *testing.T) {
	config := &schema.AtmosConfiguration{BasePath: t.TempDir()}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := recordResolvedVersionContext(ctx, config, "id", versionResolutionRecord{name: "component", constraint: "> 1.0.0", version: "v1.1.0"})
	require.ErrorIs(t, err, context.Canceled)
	require.NoFileExists(t, lockfile.Path(config))
}
