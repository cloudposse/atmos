package signature

import (
	"context"
	"net/http"
	"path/filepath"
	"time"

	"github.com/sigstore/sigstore-go/pkg/root"
	"github.com/sigstore/sigstore-go/pkg/tuf"
	"github.com/theupdateframework/go-tuf/v2/metadata/fetcher"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filelock"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/xdg"
)

const trustDirectoryMode = 0o700

// LoadPublicRoot returns the Sigstore public-good trusted root. Offline mode may use only
// unexpired, TUF-authenticated cached trust material. Never accept roots supplied by an artifact
// or signature bundle.
func LoadPublicRoot(ctx context.Context, offline bool) (root.TrustedMaterial, error) {
	defer perf.Track(nil, "signature.LoadPublicRoot")()

	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	directory, err := xdg.GetXDGDataDir("sigstore", trustDirectoryMode)
	if err != nil {
		return nil, err
	}
	var trusted *root.TrustedRoot
	err = filelock.New(filepath.Join(directory, "tuf.lock")).WithExclusive(ctx, func() error {
		options := tuf.DefaultOptions().WithCachePath(filepath.Join(directory, "tuf")).WithCacheValidity(1)
		downloads := fetcher.NewDefaultFetcher()
		downloads.SetHTTPClient(contextClient{ctx: ctx, client: &http.Client{Timeout: 30 * time.Second}})
		options.WithFetcher(downloads)
		if offline {
			options.WithForceCache().WithFetcher(offlineFetcher{})
		}
		var err error
		trusted, err = root.FetchTrustedRootWithOptions(options)
		return err
	})
	return trusted, err
}

type offlineFetcher struct{}

//nolint:lintroller // Pure fail-closed fetcher: never performs I/O.
func (offlineFetcher) DownloadFile(_ string, _ int64, _ time.Duration) ([]byte, error) {
	return nil, errUtils.ErrSignatureOffline
}

type contextClient struct {
	ctx    context.Context
	client *http.Client
}

func (c contextClient) Do(request *http.Request) (*http.Response, error) {
	defer perf.Track(nil, "signature.contextClient.Do")()

	return c.client.Do(request.Clone(c.ctx)) //nolint:gosec // URLs come only from the independently trusted Sigstore TUF configuration.
}
