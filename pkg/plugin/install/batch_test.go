package install

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	atmosoci "github.com/cloudposse/atmos/pkg/oci"
	pluginlock "github.com/cloudposse/atmos/pkg/plugin/lock"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/ui/batch"
)

func batchRegistry(t *testing.T) (*Service, string, string) {
	t.Helper()
	service, release, directory := installFixture(t)
	server := httptest.NewServer(registry.New(registry.Logger(log.New(io.Discard, "", 0))))
	t.Cleanup(server.Close)
	repository, err := name.NewRepository(strings.TrimPrefix(server.URL, "http://") + "/fixture/plugin")
	require.NoError(t, err)
	index, err := pluginoci.BuildIndex(directory, service.Descriptors)
	require.NoError(t, err)
	client := atmosoci.NewClient(nil)
	require.NoError(t, client.WriteIndex(t.Context(), repository.Tag("1.0.0"), index))
	service.Registry = client
	return service, "oci://" + repository.Name(), release.Digest
}

func TestBatchDownloadsOverlapWithinConfiguredLimit(t *testing.T) {
	for _, configured := range []int{0, 1, 2} {
		t.Run(fmt.Sprint(configured), func(t *testing.T) {
			service, source, digest := batchRegistry(t)
			service.Config.Plugins.MaxConcurrency = configured
			limit := configured
			if limit == 0 {
				limit = defaultMaxConcurrency
			}
			original := service.Registry
			registry := NewMockRegistry(gomock.NewController(t))
			service.Registry = registry
			entered := make(chan struct{}, limit*2+1)
			release := make(chan struct{})
			var once sync.Once
			t.Cleanup(func() { once.Do(func() { close(release) }) })
			var active, maximum atomic.Int64
			registry.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, ref name.Reference) (*remote.Descriptor, error) {
				current := active.Add(1)
				defer active.Add(-1)
				for old := maximum.Load(); current > old && !maximum.CompareAndSwap(old, current); old = maximum.Load() {
				}
				entered <- struct{}{}
				select {
				case <-release:
				case <-ctx.Done():
					return nil, ctx.Err()
				}
				return original.Get(ctx, ref)
			}).Times(limit*2 + 1)
			requests := make(map[string]*Request)
			for index := range limit*2 + 1 {
				requests[fmt.Sprint(index)] = &Request{Source: source, Constraint: "*", PinnedDigest: digest}
			}
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			done := make(chan error, 1)
			go func() {
				entries, err := service.prepareBatch(ctx, requests, func(batch.Event) {})
				if err == nil && len(entries) != len(requests) {
					err = errUtils.ErrTestsFailed
				}
				done <- err
			}()
			for range limit {
				select {
				case <-entered:
				case <-ctx.Done():
					t.Fatal("workers did not overlap", ctx.Err())
				}
			}
			select {
			case <-entered:
				t.Fatal("download exceeded concurrency bound")
			case <-time.After(25 * time.Millisecond):
			}
			once.Do(func() { close(release) })
			require.NoError(t, <-done)
			assert.Equal(t, int64(limit), maximum.Load())
		})
	}
}

func TestBatchFailureLetsSiblingsFinishAndDiscardsResults(t *testing.T) {
	service, source, digest := batchRegistry(t)
	service.Config.Plugins.MaxConcurrency = 2
	original := service.Registry
	registry := NewMockRegistry(gomock.NewController(t))
	service.Registry = registry
	var calls atomic.Int64
	registry.EXPECT().Get(gomock.Any(), gomock.Any()).DoAndReturn(func(ctx context.Context, ref name.Reference) (*remote.Descriptor, error) {
		calls.Add(1)
		if strings.Contains(ref.Name(), "/failing/") {
			return nil, os.ErrPermission
		}
		return original.Get(ctx, ref)
	}).Times(3)
	requests := map[string]*Request{
		"a-failure": {Source: strings.Replace(source, "/fixture/", "/failing/", 1), Constraint: "*", PinnedDigest: digest},
		"b-active":  {Source: source, Constraint: "*", PinnedDigest: digest},
		"c-queued":  {Source: source, Constraint: "*", PinnedDigest: digest},
	}
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	entries, err := service.prepareBatch(ctx, requests, func(batch.Event) {})
	require.ErrorIs(t, err, os.ErrPermission)
	assert.Nil(t, entries, "a failed batch discards every prepared entry")
	assert.Equal(t, int64(3), calls.Load(), "a failure must not skip or cancel sibling packages")
}

func TestBatchReportsRealPhasesAndCachedReuse(t *testing.T) {
	service, _, directory := installFixture(t)
	layout := filepath.Join(t.TempDir(), "layout")
	require.NoError(t, pluginoci.Pack(directory, layout, service.Descriptors))
	request := &Request{Source: "oci://example.com/plugin", Constraint: "*", Layout: layout}
	var events []batch.Event
	observer := func(event batch.Event) { events = append(events, event) }
	entries, err := service.prepareBatch(t.Context(), map[string]*Request{"fixture": request}, observer)
	require.NoError(t, err)
	require.Len(t, events, 4)
	assert.Equal(t, []string{"Resolving", "Downloading and verifying", "Validating"}, []string{events[0].Phase, events[1].Phase, events[2].Phase})
	assert.True(t, events[3].Done)
	assert.Equal(t, "installed", events[3].Outcome)
	assert.Equal(t, "1.0.0", events[3].Version)
	for _, event := range events {
		assert.Zero(t, event.Downloaded)
		assert.Zero(t, event.Total)
		assert.Zero(t, event.Fraction)
	}
	locked := entries["fixture"]
	request.Locked = &locked
	events = nil
	_, err = service.prepareBatch(t.Context(), map[string]*Request{"fixture": request}, observer)
	require.NoError(t, err)
	require.Len(t, events, 2)
	assert.Equal(t, "Cached", events[0].Phase)
	assert.Equal(t, "skipped", events[1].Outcome)
}

func TestBatchValidationCancellationAndEmptyInput(t *testing.T) {
	service, _, _ := installFixture(t)
	entries, err := service.PrepareBatch(t.Context(), nil)
	require.NoError(t, err)
	assert.Equal(t, map[string]pluginlock.Entry{}, entries)
	service.Config.Plugins.MaxConcurrency = -1
	_, err = service.PrepareBatch(t.Context(), map[string]*Request{"fixture": {}})
	require.ErrorIs(t, err, errUtils.ErrPluginInvalidConfig)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	_, err = service.PrepareBatch(ctx, map[string]*Request{"fixture": {}})
	require.ErrorIs(t, err, context.Canceled)
	_, err = service.Prepare(ctx, &Request{})
	require.ErrorIs(t, err, context.Canceled)
}
