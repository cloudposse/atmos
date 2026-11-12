package imports

import (
	"context"

	"github.com/cloudposse/atmos/pkg/downloader"
)

// fetch downloads src through the importer's downloader. When ctx carries a per-fetch environment
// (a named identity), the fetch runs with it; otherwise this is exactly the plain Fetch call.
func (r *RemoteImporter) fetch(ctx context.Context, src, dest string, mode downloader.ClientMode) error {
	if len(downloader.EnvironmentFromContext(ctx)) > 0 {
		if contextual, ok := r.downloader.(downloader.ContextFileDownloader); ok {
			_, err := contextual.FetchWithMetadataContext(ctx, src, dest, mode, defaultDownloadTimeout)
			return err
		}
	}
	return r.downloader.Fetch(src, dest, mode, defaultDownloadTimeout)
}
