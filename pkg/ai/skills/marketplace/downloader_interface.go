package marketplace

import "context"

// DownloaderInterface defines the interface for downloading skills.
type DownloaderInterface interface {
	Download(ctx context.Context, source *SourceInfo) (string, error)
}
