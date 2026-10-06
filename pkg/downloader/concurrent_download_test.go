package downloader

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/go-getter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/github"
	"github.com/cloudposse/atmos/pkg/schema"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -destination=mock_progress_test.go -package=downloader github.com/hashicorp/go-getter ProgressTracker

func TestConcurrentClientsKeepPrivateDetectorsAndMetadata(t *testing.T) {
	const count = 12
	baseline := append([]getter.Detector(nil), getter.Detectors...)
	config := schema.AtmosConfiguration{}
	factory := &goGetterClientFactory{atmosConfig: &config, httpClient: &http.Client{}}
	clients := make([]*goGetterClient, count)
	var workers sync.WaitGroup
	for i := range count {
		workers.Add(1)
		go func() {
			defer workers.Done()
			client, err := factory.NewClient(context.Background(), fmt.Sprintf("source-%d", i), fmt.Sprintf("dest-%d", i), ClientModeFile)
			assert.NoError(t, err)
			clients[i] = client.(*goGetterClient)
		}()
	}
	workers.Wait()
	assert.Equal(t, baseline, getter.Detectors, "client construction must not mutate the process-global detector registry")
	require.NotEmpty(t, baseline)
	for i, client := range clients {
		detectors := client.client.Detectors
		require.Len(t, detectors, len(baseline)+1)
		detector, ok := detectors[0].(*CustomGitDetector)
		require.True(t, ok)
		assert.Equal(t, fmt.Sprintf("source-%d", i), detector.source)
		assert.Same(t, &config, detector.atmosConfig)
		assert.Equal(t, baseline, detectors[1:])
		if i > 0 {
			assert.NotSame(t, clients[0].client.Detectors[0], detector)
			assert.NotSame(t, clients[0].metadata, client.metadata)
		}
	}
	// A client's list mutation cannot leak into another client or into global defaults.
	clients[0].client.Detectors[1] = nil
	assert.Equal(t, baseline, getter.Detectors)
	assert.Equal(t, baseline, clients[1].client.Detectors[1:])
	// Conversely, a later global registry replacement cannot change an existing client.
	previous := getter.Detectors[0]
	getter.Detectors[0] = nil
	t.Cleanup(func() { getter.Detectors[0] = previous })
	assert.Equal(t, baseline, clients[1].client.Detectors[1:])
}

func TestWithProgressReportsActualHTTPTransfer(t *testing.T) {
	ctrl := gomock.NewController(t)
	progress := NewMockProgressTracker(ctrl)
	body := "source content from upstream"
	progress.EXPECT().TrackProgress(gomock.Any(), int64(0), int64(len(body)), gomock.Any()).DoAndReturn(
		func(_ string, _ int64, _ int64, stream io.ReadCloser) io.ReadCloser { return stream },
	).Times(1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", fmt.Sprint(len(body)))
		if r.Method != http.MethodHead {
			_, _ = io.WriteString(w, body)
		}
	}))
	defer server.Close()
	destination := filepath.Join(t.TempDir(), "download.txt")
	_, err := NewGoGetterDownloader(nil, WithHTTPClient(server.Client()), WithProgress(progress)).(ContextFileDownloader).FetchWithMetadataContext(context.Background(), server.URL+"/source", destination, ClientModeFile, time.Second*5)
	require.NoError(t, err)
	content, err := os.ReadFile(destination)
	require.NoError(t, err)
	assert.Equal(t, body, string(content))
}

func TestFetchWithMetadataContextCancelsActiveRequest(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case started <- struct{}{}:
		default:
		}
		<-r.Context().Done()
	}))
	defer server.Close()
	finished := make(chan error, 1)
	destination := filepath.Join(t.TempDir(), "download.txt")
	go func() {
		_, err := NewGoGetterDownloader(nil, WithHTTPClient(server.Client())).(ContextFileDownloader).FetchWithMetadataContext(ctx, server.URL+"/source", destination, ClientModeFile, time.Minute)
		finished <- err
	}()
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("download request never started")
	}
	cancel()
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(5 * time.Second):
		t.Fatal("download did not honor caller cancellation")
	}
}

func TestFetchWithMetadataContextAlreadyCanceledDoesNotCreateClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	factory := NewMockClientFactory(ctrl)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	metadata, err := NewFileDownloader(factory).(ContextFileDownloader).FetchWithMetadataContext(ctx, "local-source", "destination", ClientModeFile, time.Minute)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, metadata)
}

func TestFetchWithMetadataContextPreservesEarlierDeadline(t *testing.T) {
	ctrl := gomock.NewController(t)
	factory := NewMockClientFactory(ctrl)
	client := NewMockDownloadClient(ctrl)
	deadline := time.Now().Add(time.Second)
	ctx, cancel := context.WithDeadline(context.Background(), deadline)
	defer cancel()
	factory.EXPECT().NewClient(gomock.Any(), "source", "dest", ClientModeFile).DoAndReturn(
		func(actual context.Context, _ string, _ string, _ ClientMode) (DownloadClient, error) {
			actualDeadline, ok := actual.Deadline()
			assert.True(t, ok)
			assert.Equal(t, deadline, actualDeadline)
			return client, nil
		},
	)
	client.EXPECT().Get().Return(nil)
	_, err := NewFileDownloader(factory).(ContextFileDownloader).FetchWithMetadataContext(ctx, "source", "dest", ClientModeFile, time.Hour)
	require.NoError(t, err)
}

func TestFetchWithMetadataContextRateWaitCancellation(t *testing.T) {
	for _, interrupted := range []bool{false, true} {
		t.Run(fmt.Sprint(interrupted), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			previous := github.RateLimitWaiter
			github.RateLimitWaiter = func(context.Context, int) error {
				if interrupted {
					cancel()
					return ctx.Err()
				}
				return errors.New("rate status unavailable")
			}
			t.Cleanup(func() { github.RateLimitWaiter = previous })
			ctrl := gomock.NewController(t)
			factory := NewMockClientFactory(ctrl)
			if !interrupted {
				client := NewMockDownloadClient(ctrl)
				factory.EXPECT().NewClient(gomock.Any(), gomock.Any(), "dest", ClientModeFile).Return(client, nil)
				client.EXPECT().Get().Return(nil)
			}
			_, err := NewFileDownloader(factory).(ContextFileDownloader).FetchWithMetadataContext(ctx, "https://raw.githubusercontent.com/org/repo/main/file", "dest", ClientModeFile, time.Minute)
			if interrupted {
				require.ErrorIs(t, err, context.Canceled)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
