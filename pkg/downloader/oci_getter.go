package downloader

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

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
	return oci.ProcessImage(ctx, withGHCREnvFallback(g.AtmosConfig), u.Host+u.Path, dst)
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

// prepareOCISource normalizes an "oci://" source before go-getter parses it, so callers can hand
// an OCI reference to the generic Fetch methods like any other protocol. The reference is
// normalized with name.ParseReference (host-less refs such as "ubuntu:latest" would otherwise
// fail URL parsing, with "latest" read as a port), any "//subdir" suffix is preserved, and
// "?archive=false" stops go-getter from treating a ref that ends in an archive suffix as an
// archive to decompress. Sources that are not oci:// are returned unchanged.
func prepareOCISource(src string) (string, error) {
	if !strings.HasPrefix(src, ociScheme) {
		return src, nil
	}
	base, subdir := getter.SourceDirSubdir(src)
	ref, err := name.ParseReference(strings.TrimPrefix(base, ociScheme))
	if err != nil {
		return "", errors.Join(errUtils.ErrInvalidImageReference, err)
	}
	prepared := ociScheme + ref.Name()
	if subdir != "" {
		prepared += "//" + subdir
	}
	return prepared + noArchiveQuery, nil
}
