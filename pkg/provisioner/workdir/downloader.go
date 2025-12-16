package workdir

import (
	"context"
	"sync"

	"github.com/hashicorp/go-getter"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DefaultDownloader is the default implementation of the Downloader interface.
// It uses go-getter to download sources from various protocols.
type DefaultDownloader struct {
	mu sync.Mutex
}

// NewDefaultDownloader creates a new default downloader implementation.
func NewDefaultDownloader() *DefaultDownloader {
	defer perf.Track(nil, "workdir.NewDefaultDownloader")()

	return &DefaultDownloader{}
}

// Download downloads the source from the given URI to the destination path.
func (d *DefaultDownloader) Download(ctx context.Context, uri, dest string) error {
	defer perf.Track(nil, "workdir.DefaultDownloader.Download")()

	d.mu.Lock()
	defer d.mu.Unlock()

	// Create go-getter client.
	client := &getter.Client{
		Ctx:             ctx,
		Src:             uri,
		Dst:             dest,
		Mode:            getter.ClientModeDir,
		DisableSymlinks: false,
		Getters: map[string]getter.Getter{
			"git":   &getter.GitGetter{},
			"file":  &getter.FileGetter{},
			"hg":    &getter.HgGetter{},
			"http":  &getter.HttpGetter{},
			"https": &getter.HttpGetter{},
			"s3":    &getter.S3Getter{},
			"gcs":   &getter.GCSGetter{},
		},
	}

	if err := client.Get(); err != nil {
		return errUtils.Build(errUtils.ErrSourceDownload).
			WithCause(err).
			WithExplanation("go-getter download failed").
			WithContext("uri", uri).
			WithContext("dest", dest).
			Err()
	}

	return nil
}
