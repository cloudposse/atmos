package downloader

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/hashicorp/go-getter"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	ociScheme      = "oci://"
	ociDirPerm     = 0o755
	noArchiveQuery = "?archive=false"
)

// OCIGetter is the go-getter Getter for oci:// sources. Upstream go-getter has no OCI scheme,
// so this registers one, backed by pkg/oci.ProcessImage -- the single primitive that component
// vendoring, scaffold sourcing, component source provisioning, and the !include YAML function
// all pull OCI artifacts through.
//
// An OCI artifact is a set of layers extracted into a directory, so only directory mode is
// supported. The "//subdir" selection built into go-getter can pick a sub-directory out of the
// result, but not a single file (it silently copies nothing), so single-file selection stays
// with the consumer that needs it.
type OCIGetter struct {
	AtmosConfig *schema.AtmosConfiguration
	client      *getter.Client
}

// Get pulls the OCI image referenced by u and extracts all of its layers into dst.
func (g *OCIGetter) Get(dst string, u *url.URL) error {
	defer perf.Track(g.AtmosConfig, "downloader.OCIGetter.Get")()

	ctx := context.Background()
	if g.client != nil && g.client.Ctx != nil {
		ctx = g.client.Ctx
	}
	if err := os.MkdirAll(dst, ociDirPerm); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrCreateTempDirectory, err)
	}
	return oci.ProcessImage(ctx, g.AtmosConfig, u.Host+u.Path, dst)
}

// GetFile is unsupported: an OCI artifact is never a single addressable file.
func (g *OCIGetter) GetFile(_ string, u *url.URL) error {
	return fmt.Errorf("%w: %s", errUtils.ErrOCIGetFileUnsupported, u.Redacted())
}

// ClientMode always reports directory mode.
func (g *OCIGetter) ClientMode(_ *url.URL) (getter.ClientMode, error) {
	return getter.ClientModeDir, nil
}

// SetClient captures the client so Get can honor its context.
func (g *OCIGetter) SetClient(c *getter.Client) {
	g.client = c
}

// DefaultOCIFetchTimeout bounds an OCI pull when the caller does not set one with WithOCITimeout.
const DefaultOCIFetchTimeout = 10 * time.Minute

type ociFetchConfig struct {
	timeout time.Duration
}

// OCIFetchOption configures FetchOCIContext.
type OCIFetchOption func(*ociFetchConfig)

// WithOCITimeout bounds the pull (registry auth, manifest, and layer download/extract).
func WithOCITimeout(timeout time.Duration) OCIFetchOption {
	return func(c *ociFetchConfig) { c.timeout = timeout }
}

// FetchOCIContext pulls the OCI image imageRef (no "oci://" prefix) into dst through the shared
// go-getter downloader, so every OCI consumer shares one fetch path. The reference is normalized
// first (host-less refs such as "ubuntu:latest" would otherwise fail URL parsing), and
// "?archive=false" stops go-getter from treating a ref that ends in an archive suffix as an
// archive to decompress.
func FetchOCIContext(ctx context.Context, atmosConfig *schema.AtmosConfiguration, imageRef, dst string, opts ...OCIFetchOption) error {
	defer perf.Track(atmosConfig, "downloader.FetchOCIContext")()

	cfg := &ociFetchConfig{timeout: DefaultOCIFetchTimeout}
	for _, opt := range opts {
		opt(cfg)
	}

	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return errors.Join(errUtils.ErrInvalidImageReference, err)
	}
	src := ociScheme + ref.Name() + noArchiveQuery
	dl, ok := NewGoGetterDownloader(atmosConfig).(ContextFileDownloader)
	if !ok {
		return fmt.Errorf("%w: downloader does not support contexts", errUtils.ErrCreateDownloadClient)
	}
	_, err = dl.FetchWithMetadataContext(ctx, src, dst, ClientModeDir, cfg.timeout)
	return err
}
