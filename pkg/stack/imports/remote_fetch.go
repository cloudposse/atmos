package imports

import (
	"context"
	"net/url"

	"github.com/cloudposse/atmos/pkg/downloader"
)

// scopedKey qualifies a cache key with the identity the fetch runs as. What one identity may read,
// another may not, and two identities can rewrite the same URI to different repositories. So
// nothing fetched as one identity is ever served to another, or to a fetch that names none: the
// in-memory match and path caches, the file cache, and the shared source clones are all keyed by
// it. A fetch that names no identity keeps the plain key, so its cache entries are unchanged.
//
// The identity goes in front of the key because the file cache takes the file extension from the
// end of it. The import key reported to the caller (RemoteImportMatch.Key) is never scoped.
func scopedKey(ctx context.Context, key string) string {
	identity := downloader.IdentityFromContext(ctx)
	if identity == "" {
		return key
	}
	return "identity=" + url.QueryEscape(identity) + "|" + key
}

// fetch downloads src through the importer's downloader. When ctx names an identity (or carries a
// per-fetch environment), the fetch runs with it; otherwise this is exactly the plain Fetch call.
func (r *RemoteImporter) fetch(ctx context.Context, src, dest string, mode downloader.ClientMode) error {
	if downloader.FetchUsesIdentity(ctx) {
		if contextual, ok := r.downloader.(downloader.ContextFileDownloader); ok {
			_, err := contextual.FetchWithMetadataContext(ctx, src, dest, mode, defaultDownloadTimeout)
			return err
		}
	}
	return r.downloader.Fetch(src, dest, mode, defaultDownloadTimeout)
}
